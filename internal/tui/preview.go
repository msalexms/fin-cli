//go:build preview

package tui

import (
	"context"
	"time"

	"fin-cli/internal/cli"
	"fin-cli/internal/domain"
)

// NewPreviewModel builds a TUI Model pre-populated with the given quotes and
// ready to render via View(). Intended for previews/tests only.
func NewPreviewModel(app *cli.App, width, height int, quotes []domain.Quote) *Model {
	m := newModel(context.Background(), app)
	m.width = width
	m.height = height
	m.ready = true

	for _, q := range quotes {
		m.tickers = append(m.tickers, q.Symbol)
		m.quotes[q.Symbol] = q
	}
	if len(m.tickers) > 0 {
		m.selected = 0
	}
	// Seed a synthetic candle series for the selected ticker so the chart
	// pane is exercised.
	if len(m.tickers) > 0 {
		sym := m.tickers[0]
		now := time.Now().UTC()
		base := 100.0
		for i := 0; i < 22; i++ {
			base += float64(i%4-1) * 0.8
			m.candles[sym] = append(m.candles[sym], domain.Candle{
				Time:  now.AddDate(0, 0, -22+i),
				Close: base,
			})
		}
	}
	m.lastTick = time.Now()
	return m
}

// Render is a convenience wrapper over View() for previews/tests.
func (m *Model) Render() string { return m.View() }
