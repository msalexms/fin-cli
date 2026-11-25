package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/msalexms/fin-cli/internal/config"
	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/locale"
	"github.com/msalexms/fin-cli/internal/watchlist"
)

type fakeBackend struct {
	tickers []domain.Ticker
	quotes  map[domain.Ticker]domain.Quote
	cfg     config.Config
	removed []domain.Ticker
	saves   int
}

func newFake(pcts map[domain.Ticker]float64, order ...domain.Ticker) *fakeBackend {
	f := &fakeBackend{tickers: order, quotes: map[domain.Ticker]domain.Quote{}, cfg: config.Default()}
	for t, pct := range pcts {
		f.quotes[t] = domain.Quote{Symbol: t, Name: string(t) + " Corp", Price: 100, Change: pct, ChangePct: pct,
			Currency: "USD", Source: "yahoo", AsOf: time.Now(), FetchedAt: time.Now()}
	}
	return f
}

func (f *fakeBackend) Quote(_ context.Context, t domain.Ticker, _ bool) (domain.Quote, error) {
	q, ok := f.quotes[t]
	if !ok {
		return q, fmt.Errorf("yahoo: %w", domain.ErrNotFound)
	}
	return q, nil
}
func (f *fakeBackend) History(context.Context, domain.Ticker, int, bool) ([]domain.Candle, error) {
	var cs []domain.Candle
	for i := range 22 {
		cs = append(cs, domain.Candle{Time: time.Now().AddDate(0, 0, i-22), Close: 100 + float64(i%5)})
	}
	return cs, nil
}
func (f *fakeBackend) Resolve(_ context.Context, raw string) (domain.Ticker, error) {
	return domain.ParseTicker(raw)
}
func (f *fakeBackend) LoadWatchlist() ([]domain.Ticker, error) { return slices.Clone(f.tickers), nil }
func (f *fakeBackend) AddTicker(t domain.Ticker) error {
	if slices.Contains(f.tickers, t) {
		return watchlist.ErrAlreadyPresent
	}
	f.tickers = append(f.tickers, t)
	return nil
}
func (f *fakeBackend) RemoveTicker(t domain.Ticker) error {
	f.removed = append(f.removed, t)
	f.tickers = slices.DeleteFunc(f.tickers, func(x domain.Ticker) bool { return x == t })
	return nil
}
func (f *fakeBackend) MoveTicker(domain.Ticker, int) error { return nil }
func (f *fakeBackend) Config() config.Config               { return f.cfg }
func (f *fakeBackend) UpdateConfig(c config.Config) error  { f.cfg = c; f.saves++; return nil }
func (f *fakeBackend) Override(config.Field) string        { return "" }
func (f *fakeBackend) ConfigPath() string                  { return "/tmp/config.toml" }
func (f *fakeBackend) Printer() locale.Printer             { return locale.Printer{} }

// run feeds msg to the model and executes the resulting commands
// synchronously (ticks are skipped), like the Bubble Tea runtime would.
func run(t *testing.T, m *Model, msg tea.Msg) {
	t.Helper()
	_, cmd := m.Update(msg)
	drain(t, m, cmd, 0)
}

// drainWait bounds how long a command may run before it is treated as a timer.
var drainWait = 50 * time.Millisecond

func drain(t *testing.T, m *Model, cmd tea.Cmd, depth int) {
	if cmd == nil || depth > 4 {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(drainWait):
		return // a timer (poll, toast, spinner): not needed for these tests
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			drain(t, m, c, depth+1)
		}
		return
	}
	if msg == nil {
		return
	}
	_, next := m.Update(msg)
	drain(t, m, next, depth+1)
}

func press(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func start(t *testing.T, be *fakeBackend) *Model {
	t.Helper()
	m := New(context.Background(), be)
	drain(t, m, m.Init(), 0)
	run(t, m, tea.WindowSizeMsg{Width: 120, Height: 32})
	return m
}

func TestSelectionFollowsDisplayOrder(t *testing.T) {
	be := newFake(map[domain.Ticker]float64{"AAA": -1, "BBB": 3, "CCC": 1}, "AAA", "BBB", "CCC")
	m := start(t, be)
	if m.cursor != "AAA" {
		t.Fatalf("initial cursor = %s", m.cursor)
	}

	run(t, m, press("s")) // sort by % change, descending: BBB, CCC, AAA
	if be.cfg.UI.SortMode != config.SortChangeDesc {
		t.Fatalf("sort mode not persisted: %q", be.cfg.UI.SortMode)
	}
	if got := m.order(); !slices.Equal(got, []domain.Ticker{"BBB", "CCC", "AAA"}) {
		t.Fatalf("order = %v", got)
	}
	if m.cursor != "AAA" {
		t.Fatalf("sorting must keep the selected ticker, got %s", m.cursor)
	}

	run(t, m, press("g")) // first row on screen
	if m.cursor != "BBB" {
		t.Fatalf("g selected %s, want BBB (first displayed)", m.cursor)
	}
	run(t, m, press("down"))
	if m.cursor != "CCC" {
		t.Fatalf("down selected %s, want CCC", m.cursor)
	}

	// The highlighted row must be the selected ticker.
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "▸") && !strings.Contains(line, "CCC") {
			t.Fatalf("highlighted row is not CCC: %q", line)
		}
	}

	run(t, m, press("d"))
	run(t, m, press("y"))
	if !slices.Equal(be.removed, []domain.Ticker{"CCC"}) {
		t.Fatalf("removed %v, want [CCC]", be.removed)
	}
	if m.cursor != "AAA" {
		t.Fatalf("after delete cursor = %s, want the next displayed row (AAA)", m.cursor)
	}
}

func TestAddTicker(t *testing.T) {
	be := newFake(map[domain.Ticker]float64{"AAPL": 1, "MSFT": 2}, "AAPL")
	m := start(t, be)

	run(t, m, press("a"))
	for _, r := range "msft" {
		run(t, m, press(string(r)))
	}
	run(t, m, press("enter"))
	if m.mode != modeList || !slices.Equal(m.tickers, []domain.Ticker{"AAPL", "MSFT"}) || m.cursor != "MSFT" {
		t.Fatalf("after add: mode=%v tickers=%v cursor=%s", m.mode, m.tickers, m.cursor)
	}

	// Unknown tickers keep the input open with an explanation.
	run(t, m, press("a"))
	for _, r := range "nope" {
		run(t, m, press(string(r)))
	}
	run(t, m, press("enter"))
	if m.mode != modeAdd || !strings.Contains(m.toast.text, "not found") {
		t.Fatalf("mode=%v toast=%q", m.mode, m.toast.text)
	}
	run(t, m, press("esc"))
	if m.mode != modeList {
		t.Fatal("esc should close the input")
	}
}

func TestSettingsValidateAndApply(t *testing.T) {
	be := newFake(nil)
	m := start(t, be)
	run(t, m, press("c"))
	idx := slices.IndexFunc(config.Fields, func(f config.Field) bool { return f.Key == "polling_interval" })
	for range idx {
		run(t, m, press("down"))
	}
	run(t, m, press("enter"))
	m.set.input.SetValue("5s")
	run(t, m, press("enter"))
	if m.set.err == "" || be.saves != 0 {
		t.Fatalf("invalid interval accepted: err=%q saves=%d", m.set.err, be.saves)
	}
	m.set.input.SetValue("2m")
	run(t, m, press("enter"))
	if be.cfg.PollingInterval.Std() != 2*time.Minute || be.saves != 1 || m.set.editing {
		t.Fatalf("interval not saved: %v saves=%d", be.cfg.PollingInterval, be.saves)
	}
	run(t, m, press("esc"))
	if m.mode != modeList {
		t.Fatal("esc should leave settings")
	}
}

func TestViewFitsTerminal(t *testing.T) {
	be := newFake(map[domain.Ticker]float64{"AAPL": 1.5, "SAN.MC": -0.7, "BTC-USD": 0}, "AAPL", "SAN.MC", "BTC-USD", "LOADING")
	m := start(t, be)
	m.data["LOADING"] = &entry{err: errors.New("boom")}
	sizes := [][2]int{{120, 32}, {80, 24}, {60, 20}, {44, 12}, {30, 8}}
	for _, mode := range []mode{modeList, modeHelp, modeSettings, modeAdd} {
		m.mode = mode
		for _, sz := range sizes {
			run(t, m, tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			lines := strings.Split(m.View(), "\n")
			if len(lines) != sz[1] {
				t.Errorf("mode %d %dx%d: %d lines", mode, sz[0], sz[1], len(lines))
			}
			for _, l := range lines {
				if w := ansi.StringWidth(l); w > sz[0] {
					t.Errorf("mode %d %dx%d: line too wide (%d): %q", mode, sz[0], sz[1], w, ansi.Strip(l))
				}
			}
		}
	}
}

func TestEmptyWatchlist(t *testing.T) {
	m := start(t, newFake(nil))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Your watchlist is empty") {
		t.Fatalf("empty state missing:\n%s", v)
	}
	run(t, m, press("d")) // no selection: must not panic
	run(t, m, press("r"))
}
