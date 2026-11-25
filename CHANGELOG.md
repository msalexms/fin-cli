# Changelog

All notable changes to **fin-cli** are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and the project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.0] - 2026-11-25

### Added

- **AMSTech brand** — shared `theme` package with the AMSTech "noche" palette and
  the Thor mark, used by both the dashboard and `fin-cli quote`; new banners
  (EN/ES) and Thor Terminal sticker under `assets/`.
- **Dashboard** — help screen (`?`), confirmation before removing, manual
  reordering (`K`/`J`), refresh all (`R`), first/last (`g`/`G`), empty-state
  welcome, transient notifications, adaptive layout down to 44×12.
- **Settings panel** covers every option (keys, providers, interval, order),
  validates input inline, masks keys and shows env/flag overrides.
- **CLI** — `config list`, multi-ticker `add`/`remove`, `quote --no-chart`,
  `-f` shorthand for `--format`, documented exit codes for every command.
- Builds for macOS and Windows; CI on Linux, macOS and Windows; Makefile.
- `README.es.md` and a new demo GIF.

### Changed

- **Works without API keys**: `quote` and `add` no longer require a Finnhub key;
  Yahoo answers when no keyed provider is configured.
- Provider chain tries **every** provider before failing and reports the most
  useful error; quote and chart chains share one implementation.
- Settings, API keys and the refresh interval apply **immediately** (no restart).
- Sort order is persisted; the selection follows the ticker, not the row index.
- `config.toml` keeps its comments when saved; invalid values fall back to
  defaults instead of breaking startup.
- Finnhub uses a small REST client instead of the SDK (removes outdated
  protobuf/oauth2/appengine dependencies).
- Module path is now `github.com/msalexms/fin-cli` (`go install` works).
- Release archives have stable names (`fin-cli_linux_amd64.tar.gz`).
- Debug log moved to `$XDG_STATE_HOME/fin-cli/`.
- The dashboard no longer paints a black background or redraws while idle.

### Fixed

- Yahoo daily change was computed against the close **five sessions ago**.
- A "not found" from Finnhub stopped the chain, so international tickers
  failed whenever a Finnhub key was set.
- Alpha Vantage's daily-limit notice was taken as an invalid key and stopped
  the chain.
- Twelve Data's 52-week range was never read.
- Finnhub's 10-day average volume was shown as today's volume.
- With a non-manual sort, the highlighted row, the detail pane and the
  refresh/delete actions could refer to different tickers.
- Status messages never disappeared from the footer.
- Settings reported "saved" for invalid intervals or unknown providers.
- `Ctrl+C` in the settings panel did not quit.
- API keys could leak into error messages and logs through request URLs.
- Errors were printed twice and `os.Exit` skipped cleanup in subcommands.
- `export` wrote an empty file for an invalid format and could lose the last
  CSV rows.
- Watchlist add/remove could lose concurrent changes from another process.
- Fresh cached quotes were labelled as stale.
- ISIN detection accepted invalid check digits and lowercase input failed.
- Sub-unit prices (crypto, FX, penny stocks) were rounded to `0.00`.
- Unicode was disabled when `LANG` was unset (Windows and macOS terminals).
- Charts could overflow the terminal width, and FX axis labels repeated.
- FX and sub-10 prices were rounded to two decimals (EUR/USD showed `1.09`).
- A just-added ticker said "no price history" while its chart was loading.
- Initial load was throttled to one request per second (≈7 s for 6 tickers).
- The project did not compile outside Linux.

### Removed

- `scripts/` preview programs (covered by the TUI tests) and the unused
  `throttle` package and legacy single-provider service fields.

## [0.1.0] - 2026-05-21

- Initial release.

[Unreleased]: https://github.com/msalexms/fin-cli/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/msalexms/fin-cli/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/msalexms/fin-cli/releases/tag/v0.1.0
