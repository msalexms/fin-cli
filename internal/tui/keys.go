package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap collects the TUI keybindings.
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Refresh  key.Binding
	Add      key.Binding
	Delete   key.Binding
	Sort     key.Binding
	Settings key.Binding
	Help     key.Binding
	Quit     key.Binding

	// Input mode
	Submit key.Binding
	Cancel key.Binding
}

// DefaultKeyMap returns the baseline bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("\u2191/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("\u2193/j", "down"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Add: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "add"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "del"),
		),
		Sort: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "sort"),
		),
		Settings: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "config"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

// ShortHelp renders a compact help line for the footer.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Refresh, k.Add, k.Delete, k.Sort, k.Settings, k.Quit}
}
