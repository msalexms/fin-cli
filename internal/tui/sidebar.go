package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"fin-cli/internal/domain"
)

func (m *Model) renderSidebar(w, h int) string {
	st := m.styles
	inner := w - 4 // borders + padding
	if inner < 8 {
		inner = 8
	}

	titleLabel := "WATCHLIST"
	if m.sortMode != SortManual {
		titleLabel += " [" + m.sortMode.String() + "]"
	}
	title := st.PaneTitle.Width(inner).Render(titleLabel)
	sep := st.Subtle.Width(inner).Render(strings.Repeat("\u2500", inner))

	sorted := m.sortedTickers()

	// Compute visible window for scroll.
	maxVisible := h - 5 // title + sep + bottom padding
	if maxVisible < 1 {
		maxVisible = 1
	}
	scrollOffset := 0
	if m.selected >= maxVisible {
		scrollOffset = m.selected - maxVisible + 1
	}

	var rows []string
	end := scrollOffset + maxVisible
	if end > len(sorted) {
		end = len(sorted)
	}
	for i := scrollOffset; i < end; i++ {
		rows = append(rows, m.renderSidebarRow(sorted[i], i == m.selected, inner))
	}

	// Scroll indicator.
	scrollHint := ""
	if len(sorted) > maxVisible {
		pos := m.selected + 1
		scrollHint = st.Label.Width(inner).Render(
			fmt.Sprintf(" %d/%d", pos, len(sorted)),
		)
	}

	parts := []string{title, sep, strings.Join(rows, "\n")}
	if scrollHint != "" {
		parts = append(parts, scrollHint)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return st.PaneBorder.Width(w - 2).Height(h - 2).Render(content)
}

func (m *Model) renderSidebarRow(t domain.Ticker, selected bool, width int) string {
	st := m.styles
	var marker, sym string
	if selected {
		marker = st.Accent.Render("\u258C")
		sym = st.SidebarSelected.Render(string(t))
	} else {
		marker = st.Base.Render(" ")
		sym = st.SidebarRow.Render(string(t))
	}

	// Sparkline (5-day mini chart).
	spark := ""
	if data, ok := m.sparklines[t]; ok && len(data) > 1 {
		spark = st.Label.Render(miniSparkline(data))
	}

	var right string
	if q, ok := m.quotes[t]; ok {
		right = m.colorizeChange(fmt.Sprintf("%+.2f%%", q.ChangePct), q.Change)
	} else if m.loading[t] {
		right = st.Label.Render(m.sp.View())
	} else if err, ok := m.errs[t]; ok && err != nil {
		right = st.Down.Render("!")
	}

	leftW := lipgloss.Width(marker) + 1 + lipgloss.Width(sym)
	sparkW := lipgloss.Width(spark)
	rightW := lipgloss.Width(right)
	usedW := leftW + sparkW + rightW
	if sparkW > 0 {
		usedW++
	}
	pad := width - usedW
	if pad < 1 {
		pad = 1
	}
	gap1 := st.Base.Render(" ")
	gap2 := st.Base.Render(strings.Repeat(" ", pad))
	if spark != "" {
		return marker + gap1 + sym + gap2 + spark + st.Base.Render(" ") + right
	}
	return marker + gap1 + sym + gap2 + right
}

// miniSparkline renders a tiny sparkline using Unicode block characters.
func miniSparkline(data []float64) string {
	if len(data) == 0 {
		return ""
	}
	blocks := []rune{'\u2581', '\u2582', '\u2583', '\u2584', '\u2585', '\u2586', '\u2587', '\u2588'}
	min, max := data[0], data[0]
	for _, v := range data[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	spread := max - min
	if spread == 0 {
		r := make([]rune, len(data))
		for i := range r {
			r[i] = blocks[3]
		}
		return string(r)
	}
	r := make([]rune, len(data))
	for i, v := range data {
		idx := int(((v - min) / spread) * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		r[i] = blocks[idx]
	}
	return string(r)
}
