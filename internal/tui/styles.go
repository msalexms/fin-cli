package tui

import "github.com/charmbracelet/lipgloss"

// Palette collects the semantic colors used across the TUI.
type Palette struct {
	Bg         lipgloss.Color
	Base       lipgloss.Color
	Label      lipgloss.Color
	Subtle     lipgloss.Color
	VerySubtle lipgloss.Color
	Up         lipgloss.Color
	Down       lipgloss.Color
	Accent     lipgloss.Color
}

// DefaultPalette is the strict OpenCode palette.
var DefaultPalette = Palette{
	Bg:         lipgloss.Color("#000000"),
	Base:       lipgloss.Color("#E0E0E0"),
	Label:      lipgloss.Color("#808080"),
	Subtle:     lipgloss.Color("#303030"),
	VerySubtle: lipgloss.Color("#1A1A1A"),
	Up:         lipgloss.Color("#4E9A06"),
	Down:       lipgloss.Color("#CC0000"),
	Accent:     lipgloss.Color("#E0E0E0"),
}

// Styles holds the precomputed lipgloss styles.
type Styles struct {
	P Palette

	// Text
	Base    lipgloss.Style
	Label   lipgloss.Style
	Subtle  lipgloss.Style
	Title   lipgloss.Style
	Heading lipgloss.Style
	Big     lipgloss.Style
	Up      lipgloss.Style
	Down    lipgloss.Style
	Neutral lipgloss.Style
	Accent  lipgloss.Style

	// Chrome
	HeaderBar lipgloss.Style
	FooterBar lipgloss.Style

	// Panes
	PaneBorder       lipgloss.Style
	PaneBorderActive lipgloss.Style
	PaneTitle        lipgloss.Style

	// Sidebar rows
	SidebarRow      lipgloss.Style
	SidebarSelected lipgloss.Style

	// Badges
	BadgeFresh   lipgloss.Style
	BadgePartial lipgloss.Style
	BadgeCache   lipgloss.Style

	// Help line
	HelpKey  lipgloss.Style
	HelpDesc lipgloss.Style
	HelpSep  lipgloss.Style
}

// NewStyles constructs Styles from a Palette.
func NewStyles(p Palette) Styles {
	base := lipgloss.NewStyle().Background(p.Bg).Foreground(p.Base)

	return Styles{
		P:       p,
		Base:    base,
		Label:   base.Foreground(p.Label),
		Subtle:  base.Foreground(p.Subtle),
		Title:   base.Foreground(p.Base).Bold(true),
		Heading: base.Foreground(p.Label).Bold(true),
		Big:     base.Foreground(p.Base).Bold(true),
		Up:      base.Foreground(p.Up),
		Down:    base.Foreground(p.Down),
		Neutral: base.Foreground(p.Label),
		Accent:  base.Foreground(p.Accent).Bold(true),

		HeaderBar: base.
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(p.Subtle).
			BorderBackground(p.Bg),

		FooterBar: base.
			Foreground(p.Label).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(p.Subtle).
			BorderBackground(p.Bg),

		PaneBorder: base.
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.Subtle).
			BorderBackground(p.Bg),

		PaneBorderActive: base.
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.Base).
			BorderBackground(p.Bg),

		PaneTitle: base.Foreground(p.Label).Bold(true),

		SidebarRow:      base,
		SidebarSelected: base.Foreground(p.Base).Bold(true),

		BadgeFresh:   base.Foreground(p.Up).Bold(true),
		BadgePartial: base.Foreground(p.Down).Bold(true),
		BadgeCache:   base.Foreground(p.Label),

		HelpKey:  base.Foreground(p.Base).Bold(true),
		HelpDesc: base.Foreground(p.Label),
		HelpSep:  base.Foreground(p.Subtle),
	}
}
