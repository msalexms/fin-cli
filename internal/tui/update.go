package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/msalexms/fin-cli/internal/config"
	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/watchlist"
)

// Update is the Bubble Tea reducer.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		if is(msg, keys.Interrupt) {
			return m, tea.Quit
		}
		switch m.mode {
		case modeAdd:
			return m, m.onAddKey(msg)
		case modeConfirmDelete:
			return m, m.onConfirmKey(msg)
		case modeSettings:
			return m, m.onSettingsKey(msg)
		case modeHelp:
			m.mode = modeList // any key closes the help
			return m, nil
		default:
			return m, m.onListKey(msg)
		}

	case spinner.TickMsg:
		return m, m.onSpinner(msg)

	case fetchedMsg:
		e := m.entry(msg.ticker)
		e.loading = false
		if msg.err != nil {
			e.err = msg.err
		} else {
			e.err = nil
			e.quote, e.hasQuote = msg.quote, true
			if msg.candles != nil {
				e.candles = msg.candles
			}
		}
		if !m.anyLoading() {
			m.updated = time.Now()
		}
		return m, nil

	case pollMsg:
		if msg.gen != m.pollGen {
			return m, nil // superseded by a newer schedule
		}
		return m, tea.Batch(m.refreshAll(true), m.schedulePoll())

	case addedMsg:
		m.busy = false
		switch {
		case errors.Is(msg.err, watchlist.ErrAlreadyPresent):
			m.closeInput()
			m.cursor = msg.ticker
			cmd := m.notify(toastInfo, string(msg.ticker)+" is already in your watchlist")
			if !slices.Contains(m.tickers, msg.ticker) { // added from another process
				m.tickers = append(m.tickers, msg.ticker)
				cmd = tea.Batch(cmd, m.fetch(msg.ticker, false))
			}
			return m, cmd
		case msg.err != nil:
			// Stay in the input so the user can fix a typo.
			return m, m.notify(toastErr, explain(msg.err))
		}
		m.closeInput()
		m.tickers = append(m.tickers, msg.ticker)
		e := m.entry(msg.ticker)
		e.quote, e.hasQuote = msg.quote, true
		m.cursor = msg.ticker
		label := string(msg.ticker)
		if !strings.EqualFold(msg.input, label) {
			label += " (" + strings.ToUpper(msg.input) + ")"
		}
		return m, tea.Batch(m.notify(toastOK, "Added "+label), m.fetch(msg.ticker, false))

	case removedMsg:
		m.busy = false
		if msg.err != nil {
			return m, m.notify(toastErr, "Could not remove "+string(msg.ticker)+": "+msg.err.Error())
		}
		order := m.order()
		i := slices.Index(order, msg.ticker)
		m.tickers = slices.DeleteFunc(m.tickers, func(t domain.Ticker) bool { return t == msg.ticker })
		delete(m.data, msg.ticker)
		if order = m.order(); len(order) > 0 {
			m.cursor = order[min(max(i, 0), len(order)-1)]
		} else {
			m.cursor = ""
		}
		return m, m.notify(toastOK, "Removed "+string(msg.ticker))

	case movedMsg:
		if msg.err != nil {
			return m, m.notify(toastErr, "Could not save the new order: "+msg.err.Error())
		}
		return m, nil

	case toastExpiredMsg:
		if msg.id == m.toast.id {
			m.toast = toast{id: m.toast.id}
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) onListKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case is(msg, keys.Quit):
		return tea.Quit
	case is(msg, keys.Help):
		m.mode = modeHelp
	case is(msg, keys.Add):
		m.mode = modeAdd
		m.input.Reset()
		return m.input.Focus()
	case is(msg, keys.Settings):
		m.mode = modeSettings
		m.set.open()
	case len(m.tickers) == 0:
		return nil // the remaining keys act on the selection
	case is(msg, keys.Up):
		m.moveCursor(-1)
	case is(msg, keys.Down):
		m.moveCursor(1)
	case is(msg, keys.Top):
		m.moveCursor(-len(m.tickers))
	case is(msg, keys.Bottom):
		m.moveCursor(len(m.tickers))
	case is(msg, keys.MoveUp), is(msg, keys.MoveDown):
		return m.reorder(is(msg, keys.MoveUp))
	case is(msg, keys.Refresh):
		return m.fetch(m.cursor, true)
	case is(msg, keys.RefreshAll):
		return m.refreshAll(true)
	case is(msg, keys.Delete):
		if !m.busy {
			m.mode = modeConfirmDelete
		}
	case is(msg, keys.Sort):
		return m.cycleSort()
	}
	return nil
}

func (m *Model) reorder(up bool) tea.Cmd {
	if m.sortMode() != config.SortManual {
		return m.notify(toastInfo, "Reordering works in manual order · press s until it shows “manual”")
	}
	delta := 1
	if up {
		delta = -1
	}
	i := slices.Index(m.tickers, m.cursor)
	j := i + delta
	if i < 0 || j < 0 || j >= len(m.tickers) {
		return nil
	}
	m.tickers[i], m.tickers[j] = m.tickers[j], m.tickers[i]
	return m.moveTicker(m.cursor, delta)
}

func (m *Model) cycleSort() tea.Cmd {
	cfg := m.be.Config()
	i := slices.Index(config.SortModes, cfg.UI.SortMode)
	cfg.UI.SortMode = config.SortModes[(i+1)%len(config.SortModes)]
	if err := m.be.UpdateConfig(cfg); err != nil {
		return m.notify(toastErr, "Could not save the sort order: "+err.Error())
	}
	return m.notify(toastInfo, "Order: "+sortLabel(cfg.UI.SortMode))
}

func (m *Model) onAddKey(msg tea.KeyMsg) tea.Cmd {
	switch {
	case is(msg, keys.Cancel):
		m.closeInput()
		return nil
	case is(msg, keys.Confirm):
		raw := strings.TrimSpace(m.input.Value())
		if raw == "" || m.busy {
			return nil
		}
		m.busy = true
		return tea.Batch(m.startSpinner(), m.addTicker(raw))
	}
	if m.busy {
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

func (m *Model) closeInput() {
	m.mode = modeList
	m.input.Blur()
	m.input.Reset()
}

func (m *Model) onConfirmKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "y", "Y", "enter", "d", "x":
		m.mode = modeList
		m.busy = true
		return tea.Batch(m.startSpinner(), m.removeTicker(m.cursor))
	default:
		m.mode = modeList
		return nil
	}
}

// explain turns an error into a short, actionable sentence.
func explain(err error) string {
	switch {
	case errors.Is(err, domain.ErrNoAPIKey):
		return "Every provider in the chain needs an API key · press c to add one or add “yahoo” to the providers"
	case errors.Is(err, domain.ErrUnauthorized):
		return "A provider rejected its API key · check it in settings (c)"
	case errors.Is(err, domain.ErrRateLimited):
		return "Rate limited by the provider · fin-cli will retry on the next refresh"
	case errors.Is(err, domain.ErrNetwork):
		return "Network error · check your connection"
	case errors.Is(err, domain.ErrUnavailable):
		return "The provider is temporarily unavailable"
	case errors.Is(err, domain.ErrNotFound):
		return "Ticker not found · non-US markets need a suffix (SAN.MC, VOD.L, 7203.T)"
	case errors.Is(err, domain.ErrInvalidInput):
		_, after, ok := strings.Cut(err.Error(), "invalid input: ")
		if ok && after != "" {
			return strings.ToUpper(after[:1]) + after[1:]
		}
	}
	return fmt.Sprint(err)
}

func sortLabel(mode string) string {
	switch mode {
	case config.SortChangeDesc:
		return "% change ↓"
	case config.SortChangeAsc:
		return "% change ↑"
	case config.SortAlpha:
		return "symbol A–Z"
	case config.SortVolume:
		return "volume"
	default:
		return "manual"
	}
}
