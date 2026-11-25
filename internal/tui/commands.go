package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/msalexms/fin-cli/internal/domain"
)

const (
	fetchTimeout = 20 * time.Second
	toastTTL     = 4 * time.Second
)

type fetchedMsg struct {
	ticker  domain.Ticker
	quote   domain.Quote
	err     error
	candles []domain.Candle // nil when history failed: keep the previous chart
}

type pollMsg struct{ gen int }

type addedMsg struct {
	input  string
	ticker domain.Ticker
	quote  domain.Quote
	err    error
}

type removedMsg struct {
	ticker domain.Ticker
	err    error
}

type movedMsg struct{ err error }

type toastExpiredMsg struct{ id int }

// fetch loads the quote and, best effort, the chart history of t.
func (m *Model) fetch(t domain.Ticker, force bool) tea.Cmd {
	e := m.entry(t)
	if e.loading {
		return nil
	}
	e.loading = true
	ctx, be := m.ctx, m.be
	return tea.Batch(m.startSpinner(), func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
		defer cancel()
		q, err := be.Quote(ctx, t, force)
		if err != nil {
			return fetchedMsg{ticker: t, err: err}
		}
		candles, _ := be.History(ctx, t, domain.HistorySessions, force)
		return fetchedMsg{ticker: t, quote: q, candles: candles}
	})
}

func (m *Model) refreshAll(force bool) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.tickers))
	for _, t := range m.tickers {
		cmds = append(cmds, m.fetch(t, force))
	}
	return tea.Batch(cmds...)
}

func (m *Model) schedulePoll() tea.Cmd {
	m.pollGen++
	gen := m.pollGen
	return tea.Tick(m.pollInterval(), func(time.Time) tea.Msg { return pollMsg{gen: gen} })
}

func (m *Model) addTicker(raw string) tea.Cmd {
	ctx, be := m.ctx, m.be
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
		defer cancel()
		t, err := be.Resolve(ctx, raw)
		if err != nil {
			return addedMsg{input: raw, err: err}
		}
		q, err := be.Quote(ctx, t, false)
		if err == nil {
			err = be.AddTicker(t)
		}
		return addedMsg{input: raw, ticker: t, quote: q, err: err}
	}
}

func (m *Model) removeTicker(t domain.Ticker) tea.Cmd {
	be := m.be
	return func() tea.Msg { return removedMsg{ticker: t, err: be.RemoveTicker(t)} }
}

func (m *Model) moveTicker(t domain.Ticker, delta int) tea.Cmd {
	be := m.be
	return func() tea.Msg { return movedMsg{err: be.MoveTicker(t, delta)} }
}

// notify shows a transient message in the footer.
func (m *Model) notify(kind toastKind, text string) tea.Cmd {
	id := m.toast.id + 1
	m.toast = toast{text: text, kind: kind, id: id}
	return tea.Tick(toastTTL, func(time.Time) tea.Msg { return toastExpiredMsg{id: id} })
}

// startSpinner starts the spinner animation if it is not already running.
// It stops by itself once nothing is loading, so an idle dashboard does not
// redraw at all.
func (m *Model) startSpinner() tea.Cmd {
	if m.spinOn {
		return nil
	}
	m.spinOn = true
	return m.spin.Tick
}

func (m *Model) onSpinner(msg spinner.TickMsg) tea.Cmd {
	if !m.anyLoading() {
		m.spinOn = false
		return nil
	}
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return cmd
}
