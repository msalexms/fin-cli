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

type sparklineMsg struct {
	Ticker domain.Ticker
	Data   []float64
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

// fetchSparklineCmd fetches 5-day history for a ticker's sidebar sparkline.
func fetchSparklineCmd(ctx context.Context, svc *quotes.Service, t domain.Ticker) tea.Cmd {
	return func() tea.Msg {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		candles, err := svc.History(cctx, t, domain.Range5D)
		if err != nil {
			return sparklineMsg{Ticker: t, Err: err}
		}
		data := make([]float64, 0, len(candles))
		for _, c := range candles {
			data = append(data, c.Close)
		}
		return sparklineMsg{Ticker: t, Data: data}
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
