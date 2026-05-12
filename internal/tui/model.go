package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"

	"fin-cli/internal/cli"
	"fin-cli/internal/domain"
)

// mode enumerates the TUI interaction modes.
type mode int

const (
	modeList    mode = iota // browsing the watchlist
	modeAdd                 // text input to add a ticker/ISIN
	modeConfirm             // confirm delete
)

// Model is the Bubbletea model of the interactive dashboard.
type Model struct {
	ctx context.Context
	app *cli.App

	styles Styles
	keys   KeyMap
	sp     spinner.Model

	width, height int
	ready         bool

	tickers  []domain.Ticker
	selected int

	quotes  map[domain.Ticker]domain.Quote
	candles map[domain.Ticker][]domain.Candle
	loading map[domain.Ticker]bool
	errs    map[domain.Ticker]error

	lastTick  time.Time
	globalErr error

	// Interaction state
	mode     mode
	input    textinput.Model
	status   string // transient message shown in the footer
	statusOK bool   // green if true, red otherwise
	busy     bool   // validating an add/delete
}

func newModel(ctx context.Context, app *cli.App) *Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	st := NewStyles(DefaultPalette)
	s.Style = st.Label

	in := textinput.New()
	in.Placeholder = "AAPL or US0378331005"
	in.Prompt = "› "
	in.CharLimit = 16
	in.Width = 24
	in.PromptStyle = st.Title
	in.TextStyle = st.Base
	in.PlaceholderStyle = st.Subtle

	return &Model{
		ctx:      ctx,
		app:      app,
		styles:   st,
		keys:     DefaultKeyMap(),
		sp:       s,
		input:    in,
		quotes:   make(map[domain.Ticker]domain.Quote),
		candles:  make(map[domain.Ticker][]domain.Candle),
		loading:  make(map[domain.Ticker]bool),
		errs:     make(map[domain.Ticker]error),
		lastTick: time.Now(),
	}
}
