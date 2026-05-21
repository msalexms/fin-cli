package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"fin-cli/internal/format"
)

func (m *Model) renderHeader(w int) string {
	st := m.styles
	title := st.Title.Render("\u258C fin-cli")
	count := st.Label.Render(fmt.Sprintf("%d tickers", len(m.tickers)))
	clock := st.Label.Render(time.Now().Format("15:04"))
	sep := st.HelpSep.Render(" \u00B7 ")
	center := count + sep + m.nextPollText()

	used := lipgloss.Width(title) + lipgloss.Width(center) + lipgloss.Width(clock)
	pad := w - used - 2
	if pad < 2 {
		pad = 2
	}
	left := pad / 2
	right := pad - left
	lfill := st.Base.Render(strings.Repeat(" ", left))
	rfill := st.Base.Render(strings.Repeat(" ", right))
	content := title + lfill + center + rfill + clock
	return st.HeaderBar.Width(w).Render(content)
}

func (m *Model) nextPollText() string {
	next := m.lastTick.Add(m.pollInterval())
	remaining := time.Until(next)
	if remaining < 0 {
		remaining = 0
	}
	return m.styles.Label.Render(fmt.Sprintf("next %s", format.ShortDuration(remaining)))
}
