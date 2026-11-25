// Package theme is the AMSTech visual identity for the terminal, shared by
// the one-shot CLI renderer and the TUI so both look like one product.
//
// Colors come from the "noche" mode of the AMSTech design system (manual de
// marca v1.5, amstech.dev): indigo brand, Thor accent, warm light text and
// the soft green/red signal colors, tuned for dark terminal backgrounds.
// No background color is ever painted: the user's terminal theme is kept.
package theme

import "github.com/charmbracelet/lipgloss"

// AMSTech palette, noche mode.
const (
	Text     = "#E9E6E0" // --ams-texto
	Text2    = "#C4C0CB" // --ams-texto-2
	Muted    = "#9E9AA8" // --ams-gris
	Line     = "#4A4A57" // --ams-borde, one step lighter for pure-black terminals
	Brand    = "#7F89E8" // --ams-marca
	Thor     = "#9EA6F2" // --ams-thor
	Ink      = "#18181D" // --ams-fondo (noche), used inside the logo
	Yellow   = "#D9C26A" // --ams-amarillo
	Up       = "#8FD3A4" // --ams-ok
	Down     = "#F0A094" // --ams-mal
	Selected = "#262A55" // --ams-principal (noche), selected-row background
)

// Styles are the semantic text styles. Build them with New for the output
// they will be written to, so color is dropped for pipes and NO_COLOR.
type Styles struct {
	Text, Text2, Muted, Line lipgloss.Style
	Brand, Accent, Key       lipgloss.Style
	Strong, Title, Label     lipgloss.Style
	Up, Down, Warn           lipgloss.Style
	SelectedRow              lipgloss.Style
	Pane, PaneFocused        lipgloss.Style
}

// New builds Styles bound to r (nil means lipgloss' default renderer, i.e. stdout).
func New(r *lipgloss.Renderer) Styles {
	if r == nil {
		r = lipgloss.DefaultRenderer()
	}
	fg := func(c string) lipgloss.Style { return r.NewStyle().Foreground(lipgloss.Color(c)) }
	pane := r.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(Line)).Padding(0, 1)
	return Styles{
		Text:        fg(Text),
		Text2:       fg(Text2),
		Muted:       fg(Muted),
		Line:        fg(Line),
		Brand:       fg(Brand),
		Accent:      fg(Brand).Bold(true),
		Key:         fg(Thor).Bold(true),
		Strong:      fg(Text).Bold(true),
		Title:       fg(Text).Bold(true),
		Label:       fg(Muted).Bold(true),
		Up:          fg(Up),
		Down:        fg(Down),
		Warn:        fg(Yellow),
		SelectedRow: r.NewStyle().Background(lipgloss.Color(Selected)),
		Pane:        pane,
		PaneFocused: pane.BorderForeground(lipgloss.Color(Brand)),
	}
}

// Change picks the up/down/neutral style for a signed change.
func (s Styles) Change(v float64) lipgloss.Style {
	switch {
	case v > 0:
		return s.Up
	case v < 0:
		return s.Down
	default:
		return s.Muted
	}
}
