package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// View renders the current state. Called on every tick.
func (m *Model) View() string {
	if !m.ready {
		return ""
	}
	if m.width < 24 || m.height < 8 {
		return m.styles.Base.Width(m.width).Height(m.height).
			Render("terminal too small")
	}

	header := m.renderHeader(m.width)
	footer := m.renderFooter(m.width)
	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyH < 4 {
		bodyH = 4
	}
	body := m.renderBody(m.width, bodyH)

	composed := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	return lipgloss.NewStyle().
		Background(m.styles.P.Bg).
		Foreground(m.styles.P.Base).
		Width(m.width).
		Height(m.height).
		Render(composed)
}

// renderBody dispatches to the appropriate layout based on terminal width.
func (m *Model) renderBody(w, h int) string {
	if m.mode == modeSettings {
		return m.renderSettings(w, h)
	}
	if len(m.tickers) == 0 {
		return m.renderEmpty(w, h)
	}
	if w < 50 {
		return m.renderCollapsed(w, h)
	}

	left := w / 4
	if left < 14 {
		left = 14
	}
	right := w - left
	if right < 36 {
		right = 36
		left = w - right
	}

	sidebar := m.renderSidebar(left, h)
	detail := m.renderDetail(right, h)
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, detail)
}

func (m *Model) renderEmpty(w, h int) string {
	st := m.styles
	msg := lipgloss.JoinVertical(lipgloss.Center,
		st.Label.Render("watchlist is empty"),
		"",
		st.Base.Render("press ")+st.Title.Render("a")+st.Base.Render(" to add a ticker"),
	)
	box := st.PaneBorder.Width(w - 2).Height(h - 2).Render(centerVert(msg, h-4))
	return box
}

func (m *Model) renderCollapsed(w, h int) string {
	st := m.styles
	t := m.tickers[m.selected]
	nav := st.Label.Render(fmt.Sprintf("%d/%d \u2014 \u2191/\u2193 to switch", m.selected+1, len(m.tickers)))
	detail := m.detailBody(t, w-4, h-5)
	return st.PaneBorder.Width(w - 2).Height(h - 2).Render(nav + "\n" + detail)
}

// --- utility ---

func joinWith(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for i := 1; i < len(ss); i++ {
		out += sep + ss[i]
	}
	return out
}

func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func centerVert(s string, h int) string {
	lines := strings.Count(s, "\n") + 1
	pad := (h - lines) / 2
	if pad <= 0 {
		return s
	}
	return strings.Repeat("\n", pad) + s
}
