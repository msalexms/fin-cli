// Package tui is fin-cli's interactive dashboard (Bubble Tea): a watchlist
// with sparklines on the left, the selected instrument with stats and a
// one-month chart on the right, plus settings, help and add/remove flows.
package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/msalexms/fin-cli/internal/app"
	"github.com/msalexms/fin-cli/internal/config"
	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/locale"
)

// Backend is everything the dashboard needs from the application. It is an
// interface so the model can be tested without network or disk.
type Backend interface {
	Quote(ctx context.Context, t domain.Ticker, force bool) (domain.Quote, error)
	History(ctx context.Context, t domain.Ticker, sessions int, force bool) ([]domain.Candle, error)
	Resolve(ctx context.Context, raw string) (domain.Ticker, error)

	LoadWatchlist() ([]domain.Ticker, error)
	AddTicker(t domain.Ticker) error
	RemoveTicker(t domain.Ticker) error
	MoveTicker(t domain.Ticker, delta int) error

	Config() config.Config
	UpdateConfig(cfg config.Config) error
	Override(f config.Field) string
	ConfigPath() string
	Printer() locale.Printer
}

// Run starts the dashboard and blocks until the user quits.
func Run(ctx context.Context, a *app.App) error {
	m := New(ctx, appBackend{a})
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	if ctx.Err() != nil {
		return nil // interrupted by a signal: not an error
	}
	return err
}

// appBackend adapts *app.App to Backend.
type appBackend struct{ a *app.App }

func (b appBackend) Quote(ctx context.Context, t domain.Ticker, force bool) (domain.Quote, error) {
	return b.a.Quotes.Get(ctx, t, force)
}

func (b appBackend) History(ctx context.Context, t domain.Ticker, n int, force bool) ([]domain.Candle, error) {
	return b.a.Quotes.History(ctx, t, n, force)
}

func (b appBackend) Resolve(ctx context.Context, raw string) (domain.Ticker, error) {
	return b.a.ResolveInput(ctx, raw, false)
}

func (b appBackend) LoadWatchlist() ([]domain.Ticker, error)     { return b.a.Watchlist.Load() }
func (b appBackend) AddTicker(t domain.Ticker) error             { return b.a.Watchlist.Add(t) }
func (b appBackend) RemoveTicker(t domain.Ticker) error          { return b.a.Watchlist.Remove(t) }
func (b appBackend) MoveTicker(t domain.Ticker, delta int) error { return b.a.Watchlist.Move(t, delta) }
func (b appBackend) Config() config.Config                       { return b.a.Config() }
func (b appBackend) UpdateConfig(cfg config.Config) error        { return b.a.UpdateConfig(cfg) }
func (b appBackend) Override(f config.Field) string              { return b.a.Override(f) }
func (b appBackend) ConfigPath() string                          { return b.a.Paths.ConfigFile }
func (b appBackend) Printer() locale.Printer                     { return b.a.Printer }
