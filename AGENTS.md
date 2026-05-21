# AGENTS.md — fin-cli project context

Self-contained brief for AI agents picking up work on **fin-cli**. Read this before touching code. It captures what the project is, how it is organized, which decisions were made and why, and what the current state and open questions are.

---

## 1. What fin-cli is

A Linux terminal financial monitor written in Go. Two modes:

- **Interactive TUI** (`fin-cli`, no args): Bubbletea dashboard with a watchlist on the left, rich instrument detail and a 30-session chart on the right. Polling refresh every 5 min by default. Add/remove tickers from inside the TUI.
- **One-shot CLI** (`fin-cli quote AAPL`, etc.): prints a "neofetch-like" snapshot to stdout and exits.

The target audience is a developer who wants to keep an eye on prices from a tiling WM without leaving the terminal.

### Scope constraints (agreed with the user)

- **Linux only.** No Windows/macOS-specific branches required, though nothing intentionally blocks them.
- **No CGO.** Builds must be `CGO_ENABLED=0` and produce a static binary.
- **No emojis anywhere.** Only Unicode glyphs from the BMP: `▲ ▼ ▌ │ ─ • ▁▂▃▄▅▆▇█` and ASCII punctuation.
- **Strict minimalist aesthetic** ("OpenCode"): absolute black background, a single grey scale, green/red accents only for price direction.
- **Optimize for:** 3+ year maintainability, simplicity, robustness, stable compilation, idiomatic Go. Not for feature velocity.

---

## 2. Tech stack

| Concern | Choice | Notes |
|---|---|---|
| Language | **Go 1.23** in `go.mod` (toolchain auto-bumps to 1.25+ if available) | `CGO_ENABLED=0` required |
| CLI framework | `github.com/spf13/cobra` v1.10 | Subcommands: `quote`, `add`, `remove`, `list`, `config`, `purge` |
| TUI | `github.com/charmbracelet/bubbletea` v1.3 | Alt-screen, contextual run |
| TUI widgets | `github.com/charmbracelet/bubbles` (spinner, textinput, key) | |
| TUI styling | `github.com/charmbracelet/lipgloss` v1.1 | Rounded borders, palette |
| Config/persistence format | **TOML** (`github.com/pelletier/go-toml/v2`) | Chosen over YAML for Go idiom and no-indent-trap |
| Finnhub integration | **Official SDK** `github.com/Finnhub-Stock-API/finnhub-go/v2` | Quote, Profile2, BasicFinancials, (Candles) |
| Yahoo (quotes + history) | Raw HTTP against `query1.finance.yahoo.com/v8/finance/chart` | Unofficial, best-effort, no key; keyless fallback quote source |
| Twelve Data | Raw HTTP against `api.twelvedata.com` | Quote + History; 800 req/day, 8 req/min; requires free API key |
| Alpha Vantage | Raw HTTP against `alphavantage.co/query` | Quote + History; 25 req/day, 5 req/min; last-resort fallback |
| ISIN resolver | OpenFIGI (`api.openfigi.com/v3/mapping`) | Raw HTTP; key optional |
| Rate limiting | `golang.org/x/time/rate` (token bucket per provider) | Finnhub 60/min, TwelveData 8/min, AlphaVantage 5/min, Yahoo unlimited |
| Singleflight | `golang.org/x/sync/singleflight` | Dedupe concurrent quote refreshes per ticker |
| Locale | `golang.org/x/text/message` | Autodetect from `LC_*`/`LANG`; ASCII-only fallback on `LANG=C` |
| Logging | stdlib `log/slog` | Text handler, API keys redacted by middleware |
| Atomic writes | stdlib + `golang.org/x/sys/unix` (flock advisory) | tmp + rename + 0600 perms |

**Binary size**: ~9 MB stripped (`-s -w -trimpath`).

---

## 3. Architecture

Hexagonal-lite. **Domain** defines types and ports. **Adapters** implement ports. **Services** orchestrate. **Presentation** consumes services.

```
┌──────────────────────────────────────────────────────────┐
│ Presentation                                             │
│  cmd/fin-cli          Cobra root + signal handling       │
│  internal/tui         Bubbletea app (Model/Update/View)  │
│  internal/cli         Subcommands + one-shot renderer    │
│  internal/chart       Renderers (blocks, ascii)          │
├──────────────────────────────────────────────────────────┤
│ Application / services                                   │
│  internal/quotes      QuoteService (cache+chain+sf)      │
│  internal/isin        IsinResolver service               │
│  internal/watchlist   Watchlist store                    │
│  internal/format      Shared formatting helpers          │
├──────────────────────────────────────────────────────────┤
│ Domain                                                   │
│  internal/domain      types, errors, ports               │
├──────────────────────────────────────────────────────────┤
│ Infrastructure / adapters                                │
│  internal/providers/finnhub   Quote + fundamentals       │
│  internal/providers/yahoo     History only (free)        │
│  internal/providers/openfigi  ISIN → ticker              │
│  internal/providers/twelvedata  Quote + History          │
│  internal/providers/alphavantage  Quote + History        │
│  internal/cache       Disk cache (file-per-key, schema)  │
│  internal/config      XDG paths, TOML loader, atomic IO  │
│  internal/httpx       Shared HTTP client (timeouts+retry)│
│  internal/throttle    Rate limiter wrapper               │
│  internal/logging     slog setup + redactor              │
│  internal/locale      Locale autodetect                  │
│  internal/version     Build-time metadata                │
└──────────────────────────────────────────────────────────┘
```

### Key flow: `QuoteService.Get(ctx, sym, force)`

1. `singleflight` key = `Q_<SYM>` deduplicates concurrent calls.
2. If `!force`, try disk cache; if fresh (`FetchedAt` within TTL), return with `Source=cache`.
3. Otherwise wait on the rate limiter (ctx-aware).
4. Call `ProviderChain.Quote(ctx, sym)` — tries each configured provider in order. Terminal errors (ErrNotFound, ErrUnauthorized) stop the chain; transient errors (ErrNetwork, ErrRateLimited, ErrUnavailable) advance to the next provider; ErrNoAPIKey skips silently.
5. On transient error and a cached entry exists (even stale), return the cached value. Otherwise propagate the error.
6. Persist fresh value to disk. Return.

### History flow

`QuoteService.History` delegates to the `HistoryChain` (configured via `history_providers` in config). The chain tries providers in order with the same fallback semantics as the quote chain, except `ErrUnauthorized` is non-terminal (Finnhub free tier returns 403 for candles). No caching — history is only called on demand from the TUI on selection / poll, and from `quote` CLI.

### TUI polling vs cache TTL

They are equal (5 min default). The polling loop calls `Get(force=true)`, bypassing the cache. The cache only serves fast starts and graceful fallback on transient errors. This is intentional (no overlap).

---

## 4. Directory layout

```
cmd/fin-cli/main.go                     # Cobra root
internal/
  domain/        types.go errors.go ports.go
  version/       version.go              # ldflags injection
  logging/       slog.go                  # slog + redactor
  locale/        detect.go                # locale → Printer
  httpx/         client.go                # timeouts + retries
  throttle/      limiter.go               # token bucket
  config/        paths.go loader.go migrate.go  # TOML + flock
  cache/         disk.go                  # file-per-key JSON
  watchlist/     store.go                 # TOML + flock
  format/        format.go format_test.go # shared formatting helpers
  providers/
    finnhub/     client.go errors.go
    openfigi/    client.go
    yahoo/       client.go client_test.go
    twelvedata/  client.go client_test.go
    alphavantage/ client.go client_test.go
  quotes/        service.go chain.go chain_test.go  # cache + sf + chain
  isin/          resolver.go              # cache (30d) + openfigi
  chart/         chart.go blocks.go ascii.go
  cli/           app.go render.go         # DI + neofetch render
                 quote.go addremove.go list.go config.go purge.go export.go
  tui/           model.go update.go view.go
                 styles.go keys.go commands.go
                 deps.go messages.go       # extracted interfaces + tea.Msg types
                 header.go footer.go       # extracted chrome renderers
                 sidebar.go detail.go      # extracted pane renderers
                 settings.go               # interactive config panel
                 preview.go               # build-tag "preview"
scripts/
  preview/main.go                         # build-tag "preview"
  preview-tui/main.go                     # build-tag "preview"
go.mod
```

The two `preview` files and `scripts/preview*` are dev-only, gated by `//go:build preview`, and never compiled into production.

---

## 5. XDG layout and persistence

| Path | Purpose |
|---|---|
| `~/.config/fin-cli/config.toml` | API keys, polling interval. Perms `0600`. Template with placeholders written on first run. |
| `~/.config/fin-cli/watchlist.toml` | Array of tickers in insertion order. Perms `0600`. |
| `~/.cache/fin-cli/quotes/<TICKER>.json` | Per-ticker quote cache. Mtime = `FetchedAt`. Schema-versioned. |
| `~/.cache/fin-cli/isin/<ISIN>.json` | ISIN → ticker resolution cache. 30 days TTL. |
| `~/.local/share/fin-cli/fin-cli.log` | Only written with `--debug`. Truncated on startup if > 5 MiB. |

All writes are **atomic**: `os.CreateTemp` in same dir + rename. All TOML writes use advisory `flock` on the target path via `<path>.lock` to prevent two `fin-cli` instances from stepping on each other.

---

## 6. API key precedence

For each key (`finnhub`, `openfigi`, `twelvedata`, `alphavantage`):

1. CLI flag: `--finnhub-key`, `--openfigi-key`, `--twelvedata-key`, `--alphavantage-key`.
2. Env var: `FIN_CLI_FINNHUB_KEY`, `FIN_CLI_OPENFIGI_KEY`, `FIN_CLI_TWELVEDATA_KEY`, `FIN_CLI_ALPHAVANTAGE_KEY`.
3. Config file value.

Keys are **redacted** in logs (`X-Finnhub-Token`, `Bearer ...`, `X-OPENFIGI-APIKEY`) via a slog handler wrapper, and in `fin-cli config get` output (shows `****<last4>`).

---

## 7. Providers — gotchas

### Finnhub
- Free tier supports `/quote`, `/stock/profile2`, `/stock/metric`. All three are called per quote.
- **Free tier does NOT return stock candles** (`/stock/candle`) for US equities since ~mid-2024. The provider's `History` will return 403/`ErrUnauthorized`. We work around this via Yahoo.
- `/quote` returns all-zero fields for unknown symbols instead of 404. We detect this in `providers/finnhub/client.go` → returns `ErrNotFound`.
- Free tier has weak coverage for European tickers (`.MC`, `.DE`, etc.).

### Yahoo (history only)
- Endpoint: `https://query1.finance.yahoo.com/v8/finance/chart/{SYM}?interval=1d&range=1mo`.
- Requires a browser-like `User-Agent`; generic UAs get 429/403.
- Unofficial, undocumented. Treat as best-effort. If it breaks, the chart shows `(no historical data available)` and the rest of the UI keeps working.
- Also supports `Quote()` by parsing `meta` from the same chart endpoint (Partial=true, no fundamentals).
- Plan B if it breaks permanently: Stooq CSV (`https://stooq.com/q/d/l/?s=aapl.us&i=d`). Would be ~50 lines behind the same `domain.HistoryProvider` interface.

### Twelve Data
- Endpoint: `https://api.twelvedata.com/quote` (real-time) + `/time_series` (history).
- Free tier: 800 req/day, 8 req/min. Requires free API key.
- Returns `status: "error"` with a `code` and `message` on failures; the client classifies to domain sentinels.
- Marks quotes as `Partial=true` (no P/E, EPS, Beta from `/quote`; only price + OHLC + volume + 52w range).
- Returns time series newest-first; client reverses for chronological order.

### Alpha Vantage
- Endpoint: `https://www.alphavantage.co/query?function=GLOBAL_QUOTE` + `TIME_SERIES_DAILY`.
- Free tier: 25 req/day, 5 req/min. **Last-resort fallback** due to harsh daily limit.
- Error signaling via `Note` field (rate limit) or `Information` field (invalid key / premium).
- Returns empty `Global Quote` for unknown symbols (no HTTP error); client detects and returns ErrNotFound.
- Marks quotes as `Partial=true` (no fundamentals from GLOBAL_QUOTE).

### OpenFIGI
- No key needed (25 req/min); with a key 250 req/min.
- Used for ISIN → ticker resolution when the user passes an ISIN (either via `--isin` or autodetected by regex: `^[A-Z]{2}[A-Z0-9]{9}\d$`).
- Prefer US exchange listing when multiple candidates are returned.

---

## 8. TUI design principles

### Visual language

| Element | Style |
|---|---|
| Background (everywhere) | `#000000` absolute black |
| Primary text | `#E0E0E0` grey |
| Labels / secondary | `#808080` grey |
| Borders / dividers | `#303030` subtle grey |
| Positive delta | `#4E9A06` green |
| Negative delta | `#CC0000` red |
| Arrows | `▲` up, `▼` down, `•` flat. ASCII fallback: `^ v .` on `LANG=C`. |
| Pane border | `lipgloss.RoundedBorder()` in subtle grey |
| Section title prefix | `▌ ` (half-block) |

### Layout

- **Header bar** (1 line + bottom border): `▌ fin-cli`, `N tickers · next poll Xm`, clock.
- **Body** (flex): 25% left sidebar pane (bordered), 75% detail pane (bordered). Panes are flush (no gap) so no unstyled background bleeds between them. Under 50 cols, collapse to single pane with a counter hint.
- **Footer bar** (1 line + top border): dynamic — keybindings, or input prompt, or transient status.

### Detail sections (top to bottom)

1. Title: `▌ <SYM>  <Name>         [badge] via <provider>`
2. Subtle separator `─`
3. Big price + change (green ▲ / red ▼) + session tag (`regular`, `pre-market`, `after-hours`, `closed`)
4. Two-column stats grid: Prev Close / Open / Day Range / 52w Range / Volume / Market Cap / P/E / EPS / Beta / Div Yield
5. Subtle separator
6. Meta line: `Exchange · Industry · Country · IPO date`
7. Chart (asciigraph, height-adaptive, coloured by trend)

### Badges (provenance)

- `[*]` fresh from provider, full data (green).
- `[!]` fresh but partial — some fields missing (red).
- `[~]` served from cache / stale (grey).

### Backgrounds — critical detail

Lipgloss's `JoinVertical`/`JoinHorizontal` pads shorter lines with **unstyled spaces**, which in a terminal render as the user's default background (often grey). All fixes applied:

1. Every line that may be shorter than the pane is rendered with an explicit `Width(inner)` so lipgloss pads it with the style's background.
2. Raw `strings.Repeat(" ", n)` that sits between two styled segments is always wrapped in `st.Base.Render(...)`.
3. Panes are placed flush (`JoinHorizontal(sidebar, detail)`) with no 1-col gap. Their rounded borders are the visual separator.
4. Pane width math: in lipgloss, `Style.Width(n)` is the content width (**including** horizontal padding). Borders add 2 extra cols. For a pane of total width `w`, use `Width(w - 2)` and produce content of width `w - 4` (after 1+1 padding).
5. The root `View()` wraps the whole composed output in `lipgloss.NewStyle().Background(Bg).Width(w).Height(h).Render(...)` as a final safety net.
6. The chart width passed to asciigraph is `w - 10` to account for y-axis labels.

If a new section is added, remember to render it with explicit `Width` or pad with `st.Base.Render(...)`.

### Interaction

| Key | Mode | Action |
|---|---|---|
| `↑`/`k` | list | previous ticker |
| `↓`/`j` | list | next ticker |
| `r` | list | force-refresh selected |
| `a` | list | open add-input in footer |
| `d` | list | delete selected |
| `s` | list | cycle sort mode (Manual/%%Desc/%%Asc/Alpha/Volume) |
| `q`/`ctrl+c` | list | quit |
| `Enter` | add | validate + persist + fetch |
| `Esc` | add | cancel input |

Add flow: text input in footer → `addTickerCmd` (async) → `app.ResolveInput` (ISIN autodetect via OpenFIGI) → `app.Quotes.Get(force=true)` to validate → `app.Watchlist.Add` → success status in footer + history fetch for the new ticker.

Delete flow: confirm-free, single keypress. Removes from in-memory maps and persists.

Known caveat: if the user presses `Esc` while an add is validating, the add may still complete in the background and show a success status. Acceptable for v1.

---

## 9. Error model

Domain-level sentinels in `internal/domain/errors.go`:

- `ErrNotFound` — ticker/ISIN unknown
- `ErrRateLimited` — provider quota
- `ErrUnauthorized` — bad/missing key
- `ErrUnavailable` — 5xx / maintenance
- `ErrNetwork` — timeout, DNS, TLS
- `ErrPartialData` — 2xx but missing fields
- `ErrNoAPIKey` — operation requires a key
- `ErrInvalidInput` — user input validation
- `ErrCacheMiss` — cache lookup miss

Providers classify HTTP/SDK errors to these sentinels. UI matches via `errors.Is` in `tui/view.go:explainError` and `cli/render.go:ExitCodeFor`.

### CLI exit codes (one-shot mode)

| Code | Meaning |
|---|---|
| 0 | OK |
| 1 | generic |
| 2 | usage / invalid input |
| 3 | network / provider unavailable |
| 4 | config / auth |
| 5 | ticker not found |

---

## 10. Build and test

### Production build

```bash
CGO_ENABLED=0 go build -trimpath \
  -ldflags='-s -w -X fin-cli/internal/version.Version=X.Y.Z' \
  -o fin-cli ./cmd/fin-cli
```

### Dev previews

```bash
go run -tags preview ./scripts/preview         # CLI one-shot render (synthetic quote)
go run -tags preview ./scripts/preview-tui     # TUI snapshot (synthetic quotes)
```

Both require `-tags preview` because the preview helpers live in files guarded with `//go:build preview`.

### Tests

```bash
go test ./...                       # run all tests
```

Test coverage:
- `internal/format/format_test.go` — table-driven unit tests for all shared formatting helpers.
- `internal/providers/twelvedata/client_test.go` — httptest-based, covers Quote/History success, error classification (404, 429, 401, 5xx), and ErrNoAPIKey.
- `internal/providers/alphavantage/client_test.go` — httptest-based, covers Quote/History success, rate-limit (`Note`), unauthorized (`Information`), empty response, and HTTP error codes.
- `internal/quotes/chain_test.go` — mock providers testing ProviderChain and HistoryChain fallback logic (transient → next, terminal → stop, ErrNoAPIKey → skip, ErrUnauthorized non-terminal for history).
- `internal/providers/yahoo/client_test.go` — integration test (network-skippable).

No online calls in CI. All httptest-based tests use inline JSON fixtures.

### Inspection

```bash
go vet ./...                        # should be clean
go build ./...                      # should be clean
find . -name '*.go' -not -path './scripts/*' | xargs wc -l   # ~4.2k SLOC
```

---

## 11. Configuration reference

`~/.config/fin-cli/config.toml`:

```toml
schema_version = 2
polling_interval = "5m"     # any Go duration string
providers = ["finnhub", "yahoo"]          # quote provider chain order
history_providers = ["yahoo"]             # history provider chain order

[finnhub]
api_key = ""                # env: FIN_CLI_FINNHUB_KEY

[openfigi]
api_key = ""                # env: FIN_CLI_OPENFIGI_KEY

[twelvedata]
api_key = ""                # env: FIN_CLI_TWELVEDATA_KEY

[alphavantage]
api_key = ""                # env: FIN_CLI_ALPHAVANTAGE_KEY

[ui]
sort_mode = ""              # "manual" (default) | "%desc" | "%asc" | "alpha" | "volume"
```

Manage via `fin-cli config {get,set,unset,edit,path}`.

---

## 12. Subcommand summary

```
fin-cli                           TUI
fin-cli quote AAPL                one-shot snapshot
fin-cli quote US0378331005        ISIN autodetected
fin-cli quote --isin <id>         explicit ISIN
fin-cli add AAPL                  validate + persist
fin-cli add AAPL --no-validate    skip online check (offline use)
fin-cli remove AAPL               drop from watchlist
fin-cli list [--format=json]      print watchlist
fin-cli export [--format=csv|json] [--output=file]  export watchlist quotes
fin-cli config set <key> <value>  set + persist
fin-cli config get <key>          print (secrets redacted)
fin-cli config edit               open $EDITOR on config.toml
fin-cli config path               print config path
fin-cli purge                     clear disk caches
fin-cli --debug ...               enable slog to file
```

---

## 13. Decisions log

Agreed with the user, documented so future agents don't re-litigate:

- **TOML over YAML.** Go idiom; no indent traps; reliable stdlib-adjacent parser.
- **Finnhub only for quotes.** No Yahoo fallback for quote data — Yahoo is fragile and our dual-provider logic was considered too much machinery. Yahoo is used **only** for historical candles, where its fragility only degrades the chart.
- **Single disk cache with singleflight.** Previously the spec asked for memory+disk caches with separate TTLs. Collapsed to disk + singleflight: simpler, same behavior for a single-process CLI, no duplicate invalidation logic.
- **Unified chart package.** One interface (`chart.Renderer`), two backends (`Blocks`, `ASCII`). Blocks for tiny/sparkline contexts, ASCII (asciigraph) for everything else.
- **Exit codes are granular.** Cobra's default is 0/1; we override to the 0..5 convention above so scripts can distinguish failure modes.
- **Locale autodetection only.** No `--locale` flag for now.
- **No forex/crypto.** Only equities. Cross-currency normalization also out of scope.
- **`add` validates online by default.** Use `--no-validate` to bypass when offline.
- **First run writes a commented config template.** Users shouldn't have to read docs to know what is configurable.
- **Go module path is `fin-cli`** (no domain). Module is not published. If published later, rename.
- **Multi-provider configurable via `config.toml` array.** User controls priority order of the quote chain. One rate limiter per provider (not global).
- **Yahoo as keyless fallback quote provider.** Reuses existing chart endpoint, parses `meta`. Global coverage but partial data.
- **ProviderChain skips ErrNoAPIKey silently.** Stops on ErrNotFound/ErrUnauthorized (terminal). For history chain, ErrUnauthorized is non-terminal (Finnhub free tier returns 403 for candles).
- **Config migration v1 to v2 auto-adds defaults.** `providers = ["finnhub","yahoo"]` and `history_providers = ["yahoo"]`.
- **Sort is visual only.** Cycling sort modes does not reorder the watchlist file.
- **Sparkline sidebar uses 5-day close data.** Fetched via `domain.Range5D` on selection/add.
- **Shared formatting in `internal/format`** eliminates duplication between TUI and CLI renderers.

---

## 14. Open items / known limitations

- **Automatic tests are sparse.** No unit tests for finnhub or openfigi clients. Integration test for Yahoo only, network-skippable.
- **No mass-refresh throttling UI.** If the watchlist is >60 entries, startup hits the Finnhub limit. We wait, but the UI only shows spinners; no explicit "rate limited" feedback during this.
- **Polling config key `polling_interval` is documented in the template** but has no validation beyond being a Go duration string.
- **TUI-add cancel is fire-and-forget.** Esc during validation may still complete the add.
- **No delete confirmation.** `d` removes instantly.
- **Market cap display assumes provider units in millions of the reporting currency.** True for Finnhub; if we add a provider with different units we need to normalize.
- **Session inference is US-market-biased.** It uses `America/New_York` and NYSE hours regardless of the symbol's actual exchange.
- **No NO_COLOR handling in the TUI.** The one-shot CLI respects `NO_COLOR`/`FORCE_COLOR`; bubbletea honors `NO_COLOR` via termenv but the TUI assumes a truecolor-ish terminal for the OpenCode look.
- **No i18n / translations.** Messages are English only.

---

## 15. What to do / not do when extending

- **Do** put new sentinels in `domain/errors.go` and handle them in both `cli/render.go:ExitCodeFor` and `tui/view.go:explainError`.
- **Do** put new providers behind `domain.MarketProvider` or `domain.HistoryProvider`. Do not import provider packages from `domain`.
- **Do** wrap every shortfill / separator / padding in `st.Base.Render(...)` or use `Width(n)` to avoid terminal default-bg bleed.
- **Do** make network-bound paths take `context.Context` and respect timeouts.
- **Do** add atomic + flock-safe persistence for any new on-disk format.
- **Do not** import `lipgloss` or `bubbletea` from anything outside `internal/tui`.
- **Do not** import `cobra` from anything outside `internal/cli` + `cmd/fin-cli`.
- **Do not** add CGO dependencies.
- **Do not** echo secrets in logs, errors, or help text. Use the redactor.
- **Do not** introduce new file formats; use TOML for config, JSON for cache.

---

## 16. Quick orientation for the next agent

If you need to:

- **Show more data** → add fields to `domain.Quote`, fill them in `providers/finnhub/client.go:Quote`, render them in `cli/render.go:statsRows` and `tui/view.go:detailStatsGrid`.
- **Add a provider** → create `internal/providers/<name>/client.go` implementing `domain.MarketProvider` or `domain.HistoryProvider`. Wire in `cli/app.go:NewApp`.
- **Change the layout** → edit `tui/view.go`. Start at `renderBody` for structure, `detailBody` for the detail pane content.
- **Change a subcommand** → edit the matching file under `internal/cli/`. The `appFactory` pattern lets tests and main share construction.
- **Add a key binding** → extend `tui/keys.go`, handle in `tui/update.go:onKey`, document in `tui/view.go:renderFooter`.
- **Tune rendering on narrow terminals** → see `renderCollapsed` and the threshold in `renderBody` (`w < 50`).
- **Add shared formatting** → `internal/format/format.go`. Used by both TUI (`tui/view.go`) and CLI (`cli/render.go`).

That's it. Keep it boring, reversible, and well-scoped. The user optimizes for maintainability over velocity.
