package tui

import (
	"context"

	"fin-cli/internal/domain"
	"fin-cli/internal/locale"
)

// Deps abstracts the application services the TUI needs.
// This decouples the TUI from the concrete cli.App type, enabling
// isolated testing and cleaner dependency boundaries.
type Deps struct {
	// Quotes fetches and caches market data.
	Quotes QuotesFetcher
	// Watchlist manages the persisted ticker list.
	Watchlist WatchlistStore
	// Resolver maps raw user input (ticker or ISIN) to a Ticker.
	Resolver InputResolver
	// Printer provides locale-aware number formatting.
	Printer locale.Printer
	// ASCIIOnly disables Unicode glyphs when true.
	ASCIIOnly bool
	// PollingInterval as seconds; 0 means default (300s).
	PollSeconds int
}

// QuotesFetcher is the subset of quotes.Service the TUI uses.
type QuotesFetcher interface {
	Get(ctx context.Context, sym domain.Ticker, force bool) (domain.Quote, error)
	History(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error)
}

// WatchlistStore is the subset of watchlist.Store the TUI uses.
type WatchlistStore interface {
	Load() ([]domain.Ticker, error)
	Add(t domain.Ticker) error
	Remove(t domain.Ticker) error
}

// InputResolver resolves raw user input to a domain.Ticker.
type InputResolver interface {
	ResolveInput(ctx context.Context, raw string, forceISIN bool) (domain.Ticker, error)
}
