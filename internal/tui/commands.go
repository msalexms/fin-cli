package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"fin-cli/internal/cli"
	"fin-cli/internal/domain"
	"fin-cli/internal/quotes"
)

// ---- messages ----

type quoteFetchedMsg struct {
	Ticker  domain.Ticker
	Quote   domain.Quote
	Candles []domain.Candle
	Err     error
	Force   bool
}

type pollTickMsg struct{}

type addResultMsg struct {
	Ticker domain.Ticker
	Quote  domain.Quote
	Err    error
}

type deleteResultMsg struct {
	Ticker domain.Ticker
	Err    error
}

// ---- commands ----

// fetchQuoteCmd returns a tea.Cmd that fetches a quote and (best-effort) history.
// When force is true, the cache is bypassed (used by the polling loop and by "r").
func fetchQuoteCmd(ctx context.Context, svc *quotes.Service, t domain.Ticker, force bool) tea.Cmd {
	return func() tea.Msg {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		q, err := svc.Get(cctx, t, force)
		if err != nil {
			return quoteFetchedMsg{Ticker: t, Err: err, Force: force}
		}
		candles, herr := svc.History(cctx, t, domain.DefaultRange)
		if herr != nil {
			return quoteFetchedMsg{Ticker: t, Quote: q, Err: nil, Candles: nil, Force: force}
		}
		return quoteFetchedMsg{Ticker: t, Quote: q, Candles: candles, Force: force}
	}
}

// pollTickCmd emits a pollTickMsg after d.
func pollTickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return pollTickMsg{}
	})
}

// addTickerCmd resolves (if ISIN), validates against the provider, and persists.
func addTickerCmd(ctx context.Context, app *cli.App, raw string) tea.Cmd {
	return func() tea.Msg {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		raw = strings.ToUpper(strings.TrimSpace(raw))
		if raw == "" {
			return addResultMsg{Err: fmt.Errorf("%w: empty ticker", domain.ErrInvalidInput)}
		}

		t, err := app.ResolveInput(cctx, raw, false)
		if err != nil {
			return addResultMsg{Err: err}
		}
		if !app.HasFinnhubKey() {
			return addResultMsg{Ticker: t,
				Err: fmt.Errorf("%w: run `fin-cli config set finnhub.api_key <KEY>`", domain.ErrNoAPIKey)}
		}
		q, err := app.Quotes.Get(cctx, t, true)
		if err != nil {
			return addResultMsg{Ticker: t, Err: err}
		}
		if err := app.Watchlist.Add(t); err != nil {
			return addResultMsg{Ticker: t, Err: err}
		}
		return addResultMsg{Ticker: t, Quote: q}
	}
}

// deleteTickerCmd removes t from the watchlist.
func deleteTickerCmd(app *cli.App, t domain.Ticker) tea.Cmd {
	return func() tea.Msg {
		err := app.Watchlist.Remove(t)
		return deleteResultMsg{Ticker: t, Err: err}
	}
}
