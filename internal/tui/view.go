package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/msalexms/fin-cli/internal/config"
	"github.com/msalexms/fin-cli/internal/format"
	"github.com/msalexms/fin-cli/internal/theme"
)

const (
	minWidth  = 44
	minHeight = 12
	// Below this width the list is hidden and the detail pane takes over.
	splitWidth = 76
)

// View renders the whole screen.
func (m *Model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		msg := fmt.Sprintf("Terminal too small (%d×%d)\nenlarge it to at least %d×%d", m.width, m.height, minWidth, minHeight)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.st.Muted.Render(msg))
	}
	header := m.viewHeader()
	footer := m.viewFooter()
	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	return lipgloss.JoinVertical(lipgloss.Left, header, m.viewBody(bodyH), footer)
}

func (m *Model) viewHeader() string {
	st := m.st
	brand := st.Accent.Render(m.glyph("▌", "|")+" fin-cli") + st.Line.Render("  │  ") + st.Muted.Render("amstech")
	var info []string
	if n := len(m.tickers); n > 0 {
		info = append(info, fmt.Sprintf("%d %s", n, plural(n, "ticker", "tickers")))
	}
	if m.anyLoading() {
		info = append(info, m.spin.View()+" updating")
	} else if !m.updated.IsZero() {
		info = append(info, "updated "+m.updated.Format("15:04"))
	}
	info = append(info, "every "+config.Duration(m.pollInterval()).String())
	right := st.Muted.Render(strings.Join(info, " · "))
	line := spread(" "+brand, right+" ", m.width)
	return line + "\n" + st.Line.Render(strings.Repeat(m.glyph("─", "-"), m.width))
}

func (m *Model) viewFooter() string {
	st := m.st
	rule := st.Line.Render(strings.Repeat(m.glyph("─", "-"), m.width))
	var line string
	switch {
	case m.mode == modeAdd:
		m.input.Width = max(m.width-40, 16)
		hint := hints(st, "enter", "add", "esc", "cancel")
		if m.busy {
			hint = m.spin.View() + st.Muted.Render(" checking…")
		}
		line = st.Label.Render("ADD ") + m.input.View()
		if m.toast.text != "" && m.toast.kind == toastErr {
			hint = m.toastView()
		}
		line = spread(line, hint, m.width-2)
	case m.mode == modeConfirmDelete:
		line = st.Warn.Render("Remove "+string(m.cursor)+" from the watchlist?") + "  " + hints(st, "y", "yes", "n", "no")
	case m.toast.text != "":
		line = m.toastView()
	case m.loadErr != nil:
		line = st.Down.Render("Could not read the watchlist: " + m.loadErr.Error())
	case m.mode == modeSettings && m.set.editing:
		line = hints(st, "enter", "save", "esc", "cancel", "ctrl+u", "clear")
	case m.mode == modeSettings:
		line = hints(st, "↑↓", "move", "enter", "edit", "esc", "back")
	case m.mode == modeHelp:
		line = st.Muted.Render("press any key to close")
	case len(m.tickers) == 0:
		line = hints(st, "a", "add ticker", "c", "settings", "?", "help", "q", "quit")
	default:
		line = hints(st, "↑↓", "move", "a", "add", "d", "remove", "r", "refresh", "s", "sort", "c", "settings", "?", "help", "q", "quit")
	}
	return rule + "\n " + ansi.Truncate(line, m.width-2, "…")
}

func (m *Model) toastView() string {
	switch m.toast.kind {
	case toastOK:
		return m.st.Up.Render(m.glyph("✓ ", "") + m.toast.text)
	case toastErr:
		return m.st.Down.Render(m.glyph("✗ ", "! ") + m.toast.text)
	default:
		return m.st.Brand.Render(m.toast.text)
	}
}

func (m *Model) viewBody(h int) string {
	w := m.width
	pane := func(content string) string {
		return m.st.Pane.Width(w - 2).Height(h - 2).Render(fit(content, w-4, h-2))
	}
	switch {
	case m.mode == modeSettings:
		return m.st.PaneFocused.Width(w - 2).Height(h - 2).Render(m.viewSettings(w-4, h-2))
	case m.mode == modeHelp:
		return pane(m.viewHelp(w-4, h-2))
	case len(m.tickers) == 0:
		return pane(m.viewEmpty(w-4, h-2))
	case w < splitWidth:
		return pane(m.viewDetail(w-4, h-2, true))
	}
	listW := min(max(w/3, 26), 46)
	list := m.st.PaneFocused.Width(listW - 2).Height(h - 2).Render(m.viewList(listW-4, h-2))
	detail := m.st.Pane.Width(w - listW - 2).Height(h - 2).Render(m.viewDetail(w-listW-4, h-2, false))
	return lipgloss.JoinHorizontal(lipgloss.Top, list, detail)
}

func (m *Model) viewEmpty(w, h int) string {
	st := m.st
	var parts []string
	if !m.ascii {
		parts = append(parts, theme.Logo(nil), "")
	}
	parts = append(parts,
		st.Title.Render("Your watchlist is empty"),
		"",
		st.Muted.Render("Press ")+st.Key.Render("a")+st.Muted.Render(" to add a ticker: AAPL, SAN.MC, BTC-USD, ^GSPC or an ISIN."),
		st.Muted.Render("Quotes come from Yahoo Finance out of the box; press ")+st.Key.Render("c")+st.Muted.Render(" to add a Finnhub key for fundamentals."),
	)
	block := lipgloss.JoinVertical(lipgloss.Center, parts...)
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

func (m *Model) viewHelp(w, h int) string {
	st := m.st
	rows := [][2]string{
		{"↑ ↓  k j", "select previous / next"},
		{"g G", "jump to first / last"},
		{"K J  shift+↑↓", "move the ticker up / down (manual order)"},
		{"a", "add a ticker or ISIN"},
		{"d", "remove the selected ticker"},
		{"r / R", "refresh selected / all now"},
		{"s", "cycle order: manual, % change ↓/↑, symbol, volume"},
		{"c", "settings: API keys, providers, refresh interval"},
		{"?", "this help"},
		{"q  ctrl+c", "quit"},
	}
	lines := []string{st.Label.Render("KEYBOARD"), ""}
	for _, r := range rows {
		lines = append(lines, "  "+st.Key.Render(pad(r[0], 16))+st.Text.Render(r[1]))
	}
	lines = append(lines, "", st.Label.Render("SOURCES"), "",
		"  "+st.Muted.Render("Quotes are tried provider by provider in the configured order."),
		"  "+st.Muted.Render("“partial data” means the source has no fundamentals (P/E, EPS…)."),
		"  "+st.Muted.Render("“offline” means every provider failed: the last cached quote is shown."),
	)
	return fit(strings.Join(lines, "\n"), w, h)
}

// --- small rendering helpers ---

func (m *Model) glyph(unicode, ascii string) string {
	if m.ascii {
		return ascii
	}
	return unicode
}

func hints(st theme.Styles, pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, st.Key.Render(pairs[i])+" "+st.Muted.Render(pairs[i+1]))
	}
	return strings.Join(parts, st.Line.Render("  ·  "))
}

// fit truncates every line to w cells and the block to h lines.
func fit(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:max(h, 0)]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "…")
	}
	return strings.Join(lines, "\n")
}

func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left+" "+right, width, "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

func pad(s string, n int) string {
	s = truncate(s, n)
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

func truncate(s string, n int) string { return ansi.Truncate(s, n, "…") }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// price formats with the shared adaptive precision.
func (m *Model) price(v float64) string { return format.Price(m.printer, v) }
