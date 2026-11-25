package tui

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/msalexms/fin-cli/internal/config"
	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/locale"
	"github.com/msalexms/fin-cli/internal/theme"
)

type mode int

const (
	modeList mode = iota
	modeAdd
	modeConfirmDelete
	modeSettings
	modeHelp
)

// entry is everything known about one ticker.
type entry struct {
	quote    domain.Quote
	hasQuote bool
	candles  []domain.Candle
	err      error // last quote error
	loading  bool
}

type toastKind int

const (
	toastInfo toastKind = iota
	toastOK
	toastErr
)

type toast struct {
	text string
	kind toastKind
	id   int
}

// Model is the Bubble Tea model of the dashboard.
type Model struct {
	ctx     context.Context
	be      Backend
	st      theme.Styles
	printer locale.Printer
	ascii   bool

	width, height int

	tickers []domain.Ticker // watchlist order as stored on disk
	cursor  domain.Ticker   // selected ticker; survives re-sorting
	data    map[domain.Ticker]*entry
	loadErr error

	mode    mode
	input   textinput.Model
	busy    bool // add/remove in flight
	toast   toast
	spin    spinner.Model
	spinOn  bool
	updated time.Time // last completed refresh
	pollGen int       // invalidates scheduled polls when the interval changes

	set settingsState
}

// New builds the dashboard model.
func New(ctx context.Context, be Backend) *Model {
	st := theme.New(nil)
	p := be.Printer()

	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = "AAPL, SAN.MC, BTC-USD or an ISIN"
	in.CharLimit = 24
	in.PromptStyle = st.Accent
	in.TextStyle = st.Text
	in.PlaceholderStyle = st.Muted
	in.Cursor.Style = st.Brand

	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(st.Brand))

	m := &Model{
		ctx:     ctx,
		be:      be,
		st:      st,
		printer: p,
		ascii:   p.ASCIIOnly(),
		data:    make(map[domain.Ticker]*entry),
		input:   in,
		spin:    sp,
	}
	m.set = newSettings(st)
	return m
}

// Init loads the watchlist and starts the first fetch and the poll timer.
func (m *Model) Init() tea.Cmd {
	ts, err := m.be.LoadWatchlist()
	if err != nil {
		m.loadErr = err
		return nil
	}
	m.tickers = ts
	if len(ts) > 0 {
		m.cursor = m.order()[0]
	}
	return tea.Batch(m.refreshAll(false), m.schedulePoll())
}

func (m *Model) entry(t domain.Ticker) *entry {
	e, ok := m.data[t]
	if !ok {
		e = &entry{}
		m.data[t] = e
	}
	return e
}

func (m *Model) pollInterval() time.Duration {
	return m.be.Config().PollingInterval.Std()
}

func (m *Model) sortMode() string { return m.be.Config().UI.SortMode }

// order returns the tickers in display order for the current sort mode.
// Tickers without a quote yet sort last; ties keep the manual order.
func (m *Model) order() []domain.Ticker {
	out := slices.Clone(m.tickers)
	mode := m.sortMode()
	if mode == config.SortManual {
		return out
	}
	key := func(t domain.Ticker) (float64, bool) {
		e := m.data[t]
		if e == nil || !e.hasQuote {
			return 0, false
		}
		switch mode {
		case config.SortChangeDesc:
			return -e.quote.ChangePct, true
		case config.SortChangeAsc:
			return e.quote.ChangePct, true
		case config.SortVolume:
			v := e.quote.Volume
			if !v.Valid {
				v = e.quote.AvgVolume
			}
			return -float64(v.Value), v.Valid
		}
		return 0, true
	}
	slices.SortStableFunc(out, func(a, b domain.Ticker) int {
		if mode == config.SortAlpha {
			return cmp.Compare(a, b)
		}
		ka, oka := key(a)
		kb, okb := key(b)
		switch {
		case oka != okb:
			if oka {
				return -1
			}
			return 1
		default:
			return cmp.Compare(ka, kb)
		}
	})
	return out
}

// selectedIndex is the cursor position in display order (-1 if empty).
func (m *Model) selectedIndex(order []domain.Ticker) int {
	return slices.Index(order, m.cursor)
}

// moveCursor moves the selection by delta in display order, clamped.
func (m *Model) moveCursor(delta int) {
	order := m.order()
	if len(order) == 0 {
		return
	}
	i := max(m.selectedIndex(order), 0)
	m.cursor = order[min(max(i+delta, 0), len(order)-1)]
}

func (m *Model) anyLoading() bool {
	if m.busy {
		return true
	}
	for _, e := range m.data {
		if e.loading {
			return true
		}
	}
	return false
}
