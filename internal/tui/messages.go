package tui

// messages.go defines all tea.Msg types used by the TUI.

import "fin-cli/internal/domain"

// quoteFetchedMsg is sent when a quote (and optional history) fetch completes.
type quoteFetchedMsg struct {
	Ticker  domain.Ticker
	Quote   domain.Quote
	Candles []domain.Candle
	Err     error
	Force   bool
}

// pollTickMsg triggers a periodic refresh of all tickers.
type pollTickMsg struct{}

// addResultMsg is sent when an add-ticker operation completes.
type addResultMsg struct {
	Ticker domain.Ticker
	Quote  domain.Quote
	Err    error
}

// deleteResultMsg is sent when a delete-ticker operation completes.
type deleteResultMsg struct {
	Ticker domain.Ticker
	Err    error
}

// sparklineMsg is sent when a 5-day sparkline fetch completes.
type sparklineMsg struct {
	Ticker domain.Ticker
	Data   []float64
	Err    error
}
