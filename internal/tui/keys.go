package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type keyMap struct {
	Up, Down, Top, Bottom      key.Binding
	MoveUp, MoveDown           key.Binding
	Refresh, RefreshAll        key.Binding
	Add, Delete, Sort          key.Binding
	Settings, Help, Quit       key.Binding
	Confirm, Cancel, Interrupt key.Binding
}

var keys = keyMap{
	Up:         key.NewBinding(key.WithKeys("up", "k")),
	Down:       key.NewBinding(key.WithKeys("down", "j")),
	Top:        key.NewBinding(key.WithKeys("home", "g")),
	Bottom:     key.NewBinding(key.WithKeys("end", "G")),
	MoveUp:     key.NewBinding(key.WithKeys("shift+up", "K")),
	MoveDown:   key.NewBinding(key.WithKeys("shift+down", "J")),
	Refresh:    key.NewBinding(key.WithKeys("r")),
	RefreshAll: key.NewBinding(key.WithKeys("R")),
	Add:        key.NewBinding(key.WithKeys("a", "+")),
	Delete:     key.NewBinding(key.WithKeys("d", "x", "delete")),
	Sort:       key.NewBinding(key.WithKeys("s")),
	Settings:   key.NewBinding(key.WithKeys("c", ",")),
	Help:       key.NewBinding(key.WithKeys("?")),
	Quit:       key.NewBinding(key.WithKeys("q")),
	Confirm:    key.NewBinding(key.WithKeys("enter")),
	Cancel:     key.NewBinding(key.WithKeys("esc")),
	Interrupt:  key.NewBinding(key.WithKeys("ctrl+c")),
}

func is(msg tea.KeyMsg, b key.Binding) bool { return key.Matches(msg, b) }
