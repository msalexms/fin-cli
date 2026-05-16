# PLAN.md — fin-cli v2 Implementation Tracker

## Objective

Transform fin-cli from a US-only (Finnhub free tier) tool into a **globally-capable** financial monitor with configurable multi-provider support, plus UX improvements (sparkline sidebar, sorting, export).

---

## Implementation Steps

### Phase 1: Foundation (no breaking changes)

- [ ] **Step 1**: `internal/format/` — Shared formatting package
  - Extract `HumanizeInt`, `HumanizeFloat`, `FormatVolume`, `FormatMarketCap`, `FormatRange`, `FormatChange`, `PadRight` from tui/view.go and cli/render.go
  - Both renderers will import this package (Step 12)

- [ ] **Step 2**: Domain — Add `Range5D` constant to `domain/types.go`
  - Needed for sparkline (5-day history)

- [ ] **Step 3**: Config schema v2 + migration
  - Add fields: `Providers []string`, `HistoryProviders []string`, `TwelveData`, `AlphaVantage` sections
  - Migration v1→v2: inject `providers = ["finnhub"]`, `history_providers = ["yahoo"]`
  - Update config template with comments for new providers
  - Bump `CurrentSchemaVersion` to 2

### Phase 2: New providers

- [ ] **Step 4**: Expand Yahoo provider with `Quote()` support
  - Parse `meta` from existing chart endpoint for price data
  - Implement `domain.MarketProvider` interface fully (previously only HistoryProvider)
  - Limitation: no fundamentals (P/E, EPS, Beta) → returns partial data

- [ ] **Step 5**: New `internal/providers/twelvedata/` provider
  - Endpoints: `/quote` + `/time_series`
  - Rate limiter: 8 req/min
  - Good global coverage, requires free API key (800 req/day)

- [ ] **Step 6**: New `internal/providers/alphavantage/` provider
  - Endpoints: `GLOBAL_QUOTE` + `TIME_SERIES_DAILY` + optional `OVERVIEW`
  - Rate limiter: 5 req/min, 25 req/day
  - Last-resort provider due to harsh daily limit

### Phase 3: Orchestration

- [ ] **Step 7**: `quotes/chain.go` — ProviderChain + HistoryChain
  - Try providers in configured order
  - Terminal errors (ErrNotFound, ErrUnauthorized) stop the chain
  - Transient errors (ErrNetwork, ErrRateLimited, ErrUnavailable) advance to next
  - ErrNoAPIKey skips provider silently

- [ ] **Step 8**: Refactor `quotes/service.go` to use chains
  - Replace single `Provider` field with `Chain *ProviderChain`
  - Replace `HistoryProv` with `HistoryChain *HistoryChain`
  - Keep singleflight + cache + graceful degradation logic

- [ ] **Step 9**: CLI wiring (`cli/app.go`)
  - Read `cfg.Providers` and `cfg.HistoryProviders`
  - Instantiate only providers with keys configured (or keyless ones like Yahoo)
  - Build chains in config order
  - Per-provider rate limiters

### Phase 4: UX Features

- [ ] **Step 10**: TUI — Sparkline in sidebar + Sorting
  - Sparkline: 5-day mini-chart (Unicode blocks) next to each ticker
  - Fetch history for visible tickers on startup + each poll
  - Sorting: `s` cycles Manual > %Desc > %Asc > Alpha > Volume
  - Show current sort mode in footer

- [ ] **Step 11**: `fin-cli export` subcommand
  - Flags: `--format=csv|json` (default csv), `--output=<file>` (default stdout)
  - Reads watchlist, gets cached quotes, formats output
  - CSV columns: Symbol, Name, Price, Change, Change%, Volume, MarketCap, Currency, Exchange, FetchedAt

- [ ] **Step 12**: Refactor renderers to use `internal/format`
  - Replace duplicated helpers in `tui/view.go` and `cli/render.go`
  - Both import `internal/format` instead

### Phase 5: Quality

- [ ] **Step 13**: Tests
  - `internal/format/format_test.go` — table tests
  - `internal/providers/finnhub/client_test.go` — httptest + testdata fixtures
  - `internal/providers/yahoo/client_test.go` — expand with httptest
  - `internal/providers/twelvedata/client_test.go` — httptest + testdata
  - `internal/providers/alphavantage/client_test.go` — httptest + testdata
  - `internal/quotes/service_test.go` — mock providers, chain behavior, cache
  - `internal/cache/disk_test.go` — write/read/TTL/expiry

- [ ] **Step 14**: Update documentation
  - `AGENTS.md` — reflect new architecture, providers, config schema
  - Config template — add new provider sections with comments

- [ ] **Final**: Verify clean build
  - `CGO_ENABLED=0 go build ./...`
  - `go vet ./...`
  - `go test ./...`

---

## Architecture Diagram (after v2)

```
QuoteService.Get(ctx, sym, force)
    │
    ├─ singleflight dedup
    ├─ disk cache check (if !force)
    │
    ▼
ProviderChain.Quote(ctx, sym)
    │
    ├─ providers[0]: e.g. Finnhub  ──► success? return
    │       │ transient error? ▼
    ├─ providers[1]: e.g. Yahoo    ──► success? return
    │       │ transient error? ▼
    ├─ providers[2]: e.g. TwelveData ─► success? return
    │       │ transient error? ▼
    └─ all failed → fallback to stale cache or propagate error
```

---

## Config Schema v2

```toml
schema_version = 2
polling_interval = "5m"

# Provider chain for quotes (tried in order)
providers = ["finnhub", "yahoo"]

# Provider chain for historical data (tried in order)
history_providers = ["yahoo", "twelvedata", "alphavantage"]

[finnhub]
api_key = ""

[openfigi]
api_key = ""

[twelvedata]
api_key = ""

[alphavantage]
api_key = ""

[ui]
# sort_mode = "manual"
```

---

## Risk Register

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Yahoo quotes break (unofficial API) | Medium | Chain advances to next provider; badge shows source |
| Alpha Vantage 25 req/day exhausted | Low | Only used as last resort; daily counter prevents waste |
| Twelve Data 8 req/min hit | Medium | Dedicated rate limiter; chain skips on ErrRateLimited |
| Sparkline requests overload | Medium | 15min cache; singleflight; only visible tickers |
| Config migration breaks existing users | High | Auto-backup + safe defaults preserve v1 behavior |

---

## Decision Log (v2 additions)

- **Multi-provider configurable**: User controls provider priority in config.toml
- **Rate limiter per provider**: Each has independent limits matching their free tier
- **Yahoo for quotes**: Reuses existing chart endpoint, parses `meta` for price data
- **Sparkline 5 days**: One business week, minimizes history requests
- **Sort cycles with 's'**: No menu, direct cycling — faster for keyboard-driven workflow
- **Export to stdout by default**: Unix-friendly, pipeable
