package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/msalexms/fin-cli/internal/config"
	"github.com/msalexms/fin-cli/internal/theme"
)

// settingsState drives the settings panel, built from config.Fields so the
// panel and `fin-cli config set` always accept the same keys and values.
type settingsState struct {
	cursor  int
	editing bool
	input   textinput.Model
	err     string
}

func newSettings(st theme.Styles) settingsState {
	in := textinput.New()
	in.Prompt = "› "
	in.CharLimit = 128
	in.PromptStyle = st.Accent
	in.TextStyle = st.Text
	in.PlaceholderStyle = st.Muted
	in.Cursor.Style = st.Brand
	return settingsState{input: in}
}

func (s *settingsState) open() {
	s.editing = false
	s.err = ""
	s.input.Blur()
}

func (m *Model) onSettingsKey(msg tea.KeyMsg) tea.Cmd {
	s := &m.set
	f := config.Fields[s.cursor]

	if s.editing {
		switch {
		case is(msg, keys.Cancel):
			s.open()
			return nil
		case is(msg, keys.Confirm):
			return m.saveField(f, s.input.Value())
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		s.err = ""
		return cmd
	}

	switch {
	case is(msg, keys.Cancel), is(msg, keys.Settings), is(msg, keys.Quit):
		m.mode = modeList
	case is(msg, keys.Up):
		s.cursor = (s.cursor - 1 + len(config.Fields)) % len(config.Fields)
	case is(msg, keys.Down):
		s.cursor = (s.cursor + 1) % len(config.Fields)
	case is(msg, keys.Confirm):
		s.editing = true
		s.err = ""
		s.input.Placeholder = f.Hint
		s.input.SetValue(f.Get(m.be.Config()))
		s.input.EchoMode = textinput.EchoNormal
		if f.Secret {
			s.input.EchoMode = textinput.EchoPassword
			s.input.EchoCharacter = '•'
		}
		s.input.CursorEnd()
		return s.input.Focus()
	}
	return nil
}

// saveField validates and persists a value, then applies its side effects
// right away: new keys/providers refetch everything, a new interval
// reschedules polling.
func (m *Model) saveField(f config.Field, value string) tea.Cmd {
	cfg := m.be.Config()
	before := f.Get(cfg)
	if err := f.Set(&cfg, value); err != nil {
		m.set.err = err.Error()
		return nil
	}
	m.set.open()
	if f.Get(cfg) == before {
		return nil
	}
	if err := m.be.UpdateConfig(cfg); err != nil {
		return m.notify(toastErr, "Could not save: "+err.Error())
	}
	cmds := []tea.Cmd{m.notify(toastOK, f.Label+" saved")}
	switch {
	case f.Key == "polling_interval":
		cmds = append(cmds, m.schedulePoll())
	case f.Key == "ui.sort_mode":
	default: // API keys and provider chains
		cmds = append(cmds, m.refreshAll(true))
	}
	return tea.Batch(cmds...)
}

func (m *Model) viewSettings(w, h int) string {
	st, s := m.st, &m.set
	cfg := m.be.Config()
	labelW := 24
	valueW := max(w-labelW-6, 10)

	lines := []string{st.Label.Render("SETTINGS"), ""}
	for i, f := range config.Fields {
		active := i == s.cursor
		cursor := "  "
		label := st.Text2.Render(pad(f.Label, labelW))
		if active {
			cursor = st.Accent.Render(m.glyph("▸", ">")) + " "
			label = st.Accent.Render(pad(f.Label, labelW))
		}
		var value string
		switch {
		case active && s.editing:
			s.input.Width = valueW - 3
			value = s.input.View()
		default:
			v := f.Get(cfg)
			if f.Secret {
				v = config.Mask(v)
			}
			if v == "" {
				value = st.Muted.Render("not set")
			} else {
				value = st.Text.Render(truncate(v, valueW))
			}
			if src := m.be.Override(f); src != "" {
				value += st.Warn.Render("  ← " + src)
			}
		}
		lines = append(lines, cursor+label+value)
	}

	f := config.Fields[s.cursor]
	lines = append(lines, "", st.Line.Render(strings.Repeat(m.glyph("─", "-"), w)))
	if s.err != "" {
		lines = append(lines, st.Down.Render(m.glyph("✗ ", "x ")+s.err))
	} else {
		lines = append(lines, st.Muted.Render(f.Hint))
	}
	if src := m.be.Override(f); src != "" {
		lines = append(lines, st.Warn.Render("Currently overridden by "+src+"; the saved value applies when it is unset."))
	}
	lines = append(lines, "", st.Muted.Render("Changes apply immediately and are saved to"), st.Muted.Render(m.be.ConfigPath()))
	return fit(strings.Join(lines, "\n"), w, h)
}
