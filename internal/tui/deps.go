package tui

import (
	"context"

	"fin-cli/internal/config"
	"fin-cli/internal/domain"
	"fin-cli/internal/locale"
)

// Deps abstracts the application services the TUI needs.
type Deps struct {
	Quotes      QuotesFetcher
	Watchlist   WatchlistStore
	Resolver    InputResolver
	Config      ConfigStore
	Printer     locale.Printer
	ASCIIOnly   bool
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

// ConfigStore provides read/write access to the persistent configuration.
type ConfigStore interface {
	GetConfig() config.Config
	SetConfig(cfg config.Config) error
}
