package domain

import "context"

// Provider is a market data source. Implementations must be safe for
// concurrent use and wrap failures with the sentinels in errors.go.
type Provider interface {
	// Name identifies the provider in config, logs and the UI.
	Name() string
	// Quote fetches the current snapshot of sym.
	Quote(ctx context.Context, sym Ticker) (Quote, error)
	// History fetches up to sessions daily candles, oldest first.
	History(ctx context.Context, sym Ticker, sessions int) ([]Candle, error)
}

// ISINResolver maps an ISIN to a ticker symbol.
type ISINResolver interface {
	Resolve(ctx context.Context, isin ISIN) (Ticker, error)
}
