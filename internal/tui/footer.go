package tui

import "github.com/charmbracelet/lipgloss"

func (m *Model) renderFooter(w int) string {
	st := m.styles

	if m.mode == modeSettings {
		if m.status != "" {
			col := st.Up
			if !m.statusOK {
				col = st.Down
			}
			return st.FooterBar.Width(w).Render(col.Render(m.status))
		}
		return st.FooterBar.Width(w).Render(
			st.Label.Render("settings \u00B7 changes saved to config.toml"),
		)
	}

	if m.mode == modeAdd {
		prompt := m.input.View()
		hint := st.Label.Render("enter: add  \u00B7  esc: cancel")
		if m.busy {
			hint = st.Label.Render(m.sp.View() + " validating\u2026")
		}
		if m.status != "" {
			s := st.Up
			if !m.statusOK {
				s = st.Down
			}
			hint = s.Render(m.status)
		}
		label := st.Label.Render("add: ")
		line := label + prompt + st.Base.Render("   ") + hint
		return st.FooterBar.Width(w).Render(line)
	}

	if m.globalErr != nil {
		return st.FooterBar.Width(w).Render(
			st.Down.Render("! ") + st.Base.Render(m.globalErr.Error()),
		)
	}

	if m.status != "" {
		col := st.Up
		if !m.statusOK {
			col = st.Down
		}
		return st.FooterBar.Width(w).Render(col.Render(m.status))
	}

	// Default help line.
	sep := st.HelpSep.Render("  \u00B7  ")
	sp := st.Base.Render(" ")
	parts := []string{
		st.HelpKey.Render("\u2191/k") + sp + st.HelpDesc.Render("up"),
		st.HelpKey.Render("\u2193/j") + sp + st.HelpDesc.Render("down"),
		st.HelpKey.Render("r") + sp + st.HelpDesc.Render("refresh"),
		st.HelpKey.Render("a") + sp + st.HelpDesc.Render("add"),
		st.HelpKey.Render("d") + sp + st.HelpDesc.Render("del"),
		st.HelpKey.Render("s") + sp + st.HelpDesc.Render("sort"),
		st.HelpKey.Render("c") + sp + st.HelpDesc.Render("config"),
		st.HelpKey.Render("q") + sp + st.HelpDesc.Render("quit"),
	}

	// Truncate help items if terminal is too narrow.
	line := joinWith(parts, sep)
	for lipgloss.Width(line) > w-4 && len(parts) > 3 {
		parts = parts[:len(parts)-1]
		line = joinWith(parts, sep)
	}
	return st.FooterBar.Width(w).Render(line)
}
