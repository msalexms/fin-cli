package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"

	"fin-cli/internal/domain"
)

// mode enumerates the TUI interaction modes.
type mode int

const (
	modeList mode = iota // browsing the watchlist
	modeAdd              // text input to add a ticker/ISIN
)

// SortMode controls how the watchlist sidebar is sorted.
type SortMode int

const (
	SortManual     SortMode = iota // insertion order (original)
	SortChangeDesc                 // change % descending (best first)
	SortChangeAsc                  // change % ascending (worst first)
	SortAlpha                      // alphabetical A-Z
	SortVolume                     // volume descending
	sortModeCount                  // sentinel for cycling
)

// String returns a short label for the sort mode.
func (s SortMode) String() string {
	switch s {
	case SortChangeDesc:
		return "%desc"
	case SortChangeAsc:
		return "%asc"
	case SortAlpha:
		return "alpha"
	case SortVolume:
		return "volume"
	default:
		return "manual"
	}
}

// Model is the Bubbletea model of the interactive dashboard.
type Model struct {
	ctx  context.Context
	deps Deps

	styles Styles
	keys   KeyMap
	sp     spinner.Model

	width, height int
	ready         bool

	tickers  []domain.Ticker
	selected int

	quotes     map[domain.Ticker]domain.Quote
	candles    map[domain.Ticker][]domain.Candle
	sparklines map[domain.Ticker][]float64 // 5-day close prices for sidebar sparkline
	loading    map[domain.Ticker]bool
	errs       map[domain.Ticker]error

	lastTick  time.Time
	globalErr error

	// Sort
	sortMode SortMode

	// Interaction state
	mode     mode
	input    textinput.Model
	status   string // transient message shown in the footer
	statusOK bool   // green if true, red otherwise
	busy     bool   // validating an add/delete
}

func newModel(ctx context.Context, deps Deps, initialSort string) *Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	st := NewStyles(DefaultPalette)
	s.Style = st.Label

	in := textinput.New()
	in.Placeholder = "AAPL or US0378331005"
	in.Prompt = "\u203A "
	in.CharLimit = 16
	in.Width = 24
	in.PromptStyle = st.Title
	in.TextStyle = st.Base
	in.PlaceholderStyle = st.Subtle

	return &Model{
		ctx:        ctx,
		deps:       deps,
		styles:     st,
		keys:       DefaultKeyMap(),
		sp:         s,
		input:      in,
		quotes:     make(map[domain.Ticker]domain.Quote),
		candles:    make(map[domain.Ticker][]domain.Candle),
		sparklines: make(map[domain.Ticker][]float64),
		loading:    make(map[domain.Ticker]bool),
		errs:       make(map[domain.Ticker]error),
		lastTick:   time.Now(),
		sortMode:   parseSortMode(initialSort),
	}
}

func parseSortMode(s string) SortMode {
	switch s {
	case "change_desc", "%desc":
		return SortChangeDesc
	case "change_asc", "%asc":
		return SortChangeAsc
	case "alpha":
		return SortAlpha
	case "volume":
		return SortVolume
	default:
		return SortManual
	}
}
