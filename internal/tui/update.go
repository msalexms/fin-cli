package tui

import (
	"context"
	"errors"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"fin-cli/internal/cli"
	"fin-cli/internal/domain"
	"fin-cli/internal/watchlist"
)

// Run starts the Bubbletea program. It returns after the user quits.
func Run(ctx context.Context, app *cli.App) error {
	deps := Deps{
		Quotes:      app.Quotes,
		Watchlist:   app.Watchlist,
		Resolver:    app,
		Printer:     app.Printer,
		ASCIIOnly:   app.ASCIIOnly,
		PollSeconds: int(app.Config.PollingInterval.Std().Seconds()),
	}
	m := newModel(ctx, deps, app.Config.UI.SortMode)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}

// Init loads the watchlist, kicks off fetches, and schedules the polling tick.
func (m *Model) Init() tea.Cmd {
	ts, err := m.deps.Watchlist.Load()
	if err != nil {
		m.globalErr = err
		return m.sp.Tick
	}
	m.tickers = ts

	cmds := []tea.Cmd{m.sp.Tick}
	for _, t := range m.tickers {
		m.loading[t] = true
		cmds = append(cmds, fetchQuoteCmd(m.ctx, m.deps.Quotes, t, false))
		cmds = append(cmds, fetchSparklineCmd(m.ctx, m.deps.Quotes, t))
	}
	cmds = append(cmds, pollTickCmd(m.pollInterval()))
	return tea.Batch(cmds...)
}

func (m *Model) pollInterval() time.Duration {
	if m.deps.PollSeconds > 0 {
		return time.Duration(m.deps.PollSeconds) * time.Second
	}
	return 5 * time.Minute
}

// Update is the Bubbletea reducer.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil

	case tea.KeyMsg:
		return m.onKey(msg)

	case quoteFetchedMsg:
		m.loading[msg.Ticker] = false
		if msg.Err != nil {
			m.errs[msg.Ticker] = msg.Err
			return m, nil
		}
		delete(m.errs, msg.Ticker)
		m.quotes[msg.Ticker] = msg.Quote
		if msg.Candles != nil {
			m.candles[msg.Ticker] = msg.Candles
		}
		return m, nil

	case sparklineMsg:
		if msg.Err == nil && len(msg.Data) > 0 {
			m.sparklines[msg.Ticker] = msg.Data
		}
		return m, nil

	case addResultMsg:
		m.busy = false
		if msg.Err != nil {
			if errors.Is(msg.Err, watchlist.ErrAlreadyPresent) {
				m.setStatus(false, string(msg.Ticker)+" already in watchlist")
			} else {
				m.setStatus(false, "add failed: "+explainError(msg.Err))
			}
			return m, nil
		}
		m.tickers = append(m.tickers, msg.Ticker)
		m.quotes[msg.Ticker] = msg.Quote
		m.selected = len(m.tickers) - 1
		m.exitInput()
		m.setStatus(true, "added "+string(msg.Ticker))
		return m, tea.Batch(
			fetchQuoteCmd(m.ctx, m.deps.Quotes, msg.Ticker, false),
			fetchSparklineCmd(m.ctx, m.deps.Quotes, msg.Ticker),
		)

	case deleteResultMsg:
		m.busy = false
		if msg.Err != nil {
			m.setStatus(false, "remove failed: "+msg.Err.Error())
			return m, nil
		}
		m.removeTickerFromModel(msg.Ticker)
		m.setStatus(true, "removed "+string(msg.Ticker))
		return m, nil

	case pollTickMsg:
		var cmds []tea.Cmd
		for _, t := range m.tickers {
			if m.loading[t] {
				continue
			}
			m.loading[t] = true
			cmds = append(cmds, fetchQuoteCmd(m.ctx, m.deps.Quotes, t, true))
			cmds = append(cmds, fetchSparklineCmd(m.ctx, m.deps.Quotes, t))
		}
		cmds = append(cmds, pollTickCmd(m.pollInterval()))
		m.lastTick = time.Now()
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.sp, cmd = m.sp.Update(msg)
	return m, cmd
}

func (m *Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeAdd {
		switch {
		case keyMatches(m.keys.Cancel, msg):
			m.exitInput()
			return m, nil
		case keyMatches(m.keys.Submit, msg):
			if m.busy {
				return m, nil
			}
			m.busy = true
			m.setStatus(true, "")
			return m, addTickerCmd(m.ctx, m.deps, m.input.Value())
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch {
	case keyMatches(m.keys.Quit, msg):
		return m, tea.Quit
	case keyMatches(m.keys.Up, msg):
		if len(m.tickers) > 0 {
			m.selected = (m.selected - 1 + len(m.tickers)) % len(m.tickers)
		}
		return m, nil
	case keyMatches(m.keys.Down, msg):
		if len(m.tickers) > 0 {
			m.selected = (m.selected + 1) % len(m.tickers)
		}
		return m, nil
	case keyMatches(m.keys.Refresh, msg):
		if len(m.tickers) == 0 {
			return m, nil
		}
		t := m.tickers[m.selected]
		if m.loading[t] {
			return m, nil
		}
		m.loading[t] = true
		return m, fetchQuoteCmd(m.ctx, m.deps.Quotes, t, true)
	case keyMatches(m.keys.Add, msg):
		m.enterInput()
		return m, nil
	case keyMatches(m.keys.Delete, msg):
		if len(m.tickers) == 0 || m.busy {
			return m, nil
		}
		t := m.tickers[m.selected]
		m.busy = true
		return m, deleteTickerCmd(m.deps, t)
	case keyMatches(m.keys.Sort, msg):
		m.sortMode = (m.sortMode + 1) % SortMode(sortModeCount)
		m.setStatus(true, "sort: "+m.sortMode.String())
		return m, nil
	}
	return m, nil
}

// sortedTickers returns the tickers list sorted according to m.sortMode.
func (m *Model) sortedTickers() []domain.Ticker {
	if m.sortMode == SortManual || len(m.tickers) <= 1 {
		return m.tickers
	}
	sorted := make([]domain.Ticker, len(m.tickers))
	copy(sorted, m.tickers)

	switch m.sortMode {
	case SortChangeDesc:
		sort.Slice(sorted, func(i, j int) bool {
			return m.quotes[sorted[i]].ChangePct > m.quotes[sorted[j]].ChangePct
		})
	case SortChangeAsc:
		sort.Slice(sorted, func(i, j int) bool {
			return m.quotes[sorted[i]].ChangePct < m.quotes[sorted[j]].ChangePct
		})
	case SortAlpha:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i] < sorted[j]
		})
	case SortVolume:
		sort.Slice(sorted, func(i, j int) bool {
			vi, vj := int64(0), int64(0)
			if q, ok := m.quotes[sorted[i]]; ok && q.Volume.Valid {
				vi = q.Volume.Value
			}
			if q, ok := m.quotes[sorted[j]]; ok && q.Volume.Valid {
				vj = q.Volume.Value
			}
			return vi > vj
		})
	}
	return sorted
}

// --- helpers ---

func keyMatches(b interface{ Keys() []string }, msg tea.KeyMsg) bool {
	for _, k := range b.Keys() {
		if msg.String() == k {
			return true
		}
	}
	return false
}

func (m *Model) enterInput() {
	m.mode = modeAdd
	m.input.Reset()
	m.input.Focus()
	m.setStatus(true, "")
}

func (m *Model) exitInput() {
	m.mode = modeList
	m.input.Blur()
	m.input.Reset()
}

func (m *Model) setStatus(ok bool, s string) {
	m.status = s
	m.statusOK = ok
}

func (m *Model) removeTickerFromModel(t domain.Ticker) {
	idx := -1
	for i, x := range m.tickers {
		if x == t {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	m.tickers = append(m.tickers[:idx], m.tickers[idx+1:]...)
	delete(m.quotes, t)
	delete(m.candles, t)
	delete(m.sparklines, t)
	delete(m.loading, t)
	delete(m.errs, t)
	if m.selected >= len(m.tickers) {
		m.selected = len(m.tickers) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}
