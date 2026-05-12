package domain

import "context"

// MarketProvider provides real-time quotes and historical candles.
// Implementations must be safe for concurrent use.
type MarketProvider interface {
	// Name identifies the provider for logging/UI.
	Name() string

	// Quote fetches the current snapshot of sym.
	// Must return ErrNotFound, ErrUnauthorized, ErrRateLimited, ErrUnavailable,
	// or ErrNetwork wrapped via fmt.Errorf("%w: ...", ...) for caller matching.
	Quote(ctx context.Context, sym Ticker) (Quote, error)

	// History fetches OHLCV candles for sym over r.
	History(ctx context.Context, sym Ticker, r Range) ([]Candle, error)
}

// HistoryProvider is a narrower interface for sources that only expose
// historical candles (e.g. free chart providers decoupled from the quote API).
type HistoryProvider interface {
	Name() string
	History(ctx context.Context, sym Ticker, r Range) ([]Candle, error)
}

// IsinResolver maps an ISIN to a ticker symbol.
type IsinResolver interface {
	Resolve(ctx context.Context, isin ISIN) (Ticker, error)
}

// QuoteService is the application-level orchestrator used by CLI and TUI.
type QuoteService interface {
	// Get returns a quote, possibly from cache. Set force=true to bypass cache.
	Get(ctx context.Context, sym Ticker, force bool) (Quote, error)
	// History returns candles (no cache layer in v1, called only on demand).
	History(ctx context.Context, sym Ticker, r Range) ([]Candle, error)
}
