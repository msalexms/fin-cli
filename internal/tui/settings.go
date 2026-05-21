package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"fin-cli/internal/config"
)

// settingsField describes one editable row in the settings panel.
type settingsField struct {
	Label       string
	Key         string // config path for display
	Value       func(config.Config) string
	Apply       func(*config.Config, string)
	Placeholder string
	Secret      bool // mask display value
}

func settingsFields() []settingsField {
	return []settingsField{
		{
			Label:       "Finnhub API Key",
			Key:         "finnhub.api_key",
			Value:       func(c config.Config) string { return c.Finnhub.APIKey },
			Apply:       func(c *config.Config, v string) { c.Finnhub.APIKey = v },
			Placeholder: "paste key here",
			Secret:      true,
		},
		{
			Label:       "Twelve Data API Key",
			Key:         "twelvedata.api_key",
			Value:       func(c config.Config) string { return c.TwelveData.APIKey },
			Apply:       func(c *config.Config, v string) { c.TwelveData.APIKey = v },
			Placeholder: "paste key here",
			Secret:      true,
		},
		{
			Label:       "Alpha Vantage API Key",
			Key:         "alphavantage.api_key",
			Value:       func(c config.Config) string { return c.AlphaVantage.APIKey },
			Apply:       func(c *config.Config, v string) { c.AlphaVantage.APIKey = v },
			Placeholder: "paste key here",
			Secret:      true,
		},
		{
			Label:       "OpenFIGI API Key",
			Key:         "openfigi.api_key",
			Value:       func(c config.Config) string { return c.OpenFIGI.APIKey },
			Apply:       func(c *config.Config, v string) { c.OpenFIGI.APIKey = v },
			Placeholder: "optional",
			Secret:      true,
		},
		{
			Label:       "Polling Interval",
			Key:         "polling_interval",
			Value:       func(c config.Config) string { return c.PollingInterval.Std().String() },
			Apply:       func(c *config.Config, v string) {
				if d, err := time.ParseDuration(v); err == nil {
					c.PollingInterval = config.Duration(d)
				}
			},
			Placeholder: "e.g. 5m, 2m30s",
		},
		{
			Label: "Quote Providers",
			Key:   "providers",
			Value: func(c config.Config) string { return strings.Join(c.Providers, ", ") },
			Apply: func(c *config.Config, v string) {
				parts := strings.Split(v, ",")
				var clean []string
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						clean = append(clean, p)
					}
				}
				if len(clean) > 0 {
					c.Providers = clean
				}
			},
			Placeholder: "finnhub, yahoo, twelvedata, alphavantage",
		},
		{
			Label: "History Providers",
			Key:   "history_providers",
			Value: func(c config.Config) string { return strings.Join(c.HistoryProviders, ", ") },
			Apply: func(c *config.Config, v string) {
				parts := strings.Split(v, ",")
				var clean []string
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						clean = append(clean, p)
					}
				}
				if len(clean) > 0 {
					c.HistoryProviders = clean
				}
			},
			Placeholder: "yahoo, twelvedata, alphavantage",
		},
	}
}

// renderSettings renders the full-screen settings panel.
func (m *Model) renderSettings(w, h int) string {
	st := m.styles
	inner := w - 4
	if inner < 20 {
		inner = 20
	}

	title := st.Title.Width(inner).Render("\u258C SETTINGS")
	sep := st.Subtle.Width(inner).Render(strings.Repeat("\u2500", inner))
	hint := st.Label.Width(inner).Render("  \u2191/\u2193 navigate  \u00B7  enter edit  \u00B7  esc back")

	fields := settingsFields()
	cfg := m.deps.Config.GetConfig()

	var rows []string
	for i, f := range fields {
		val := f.Value(cfg)
		display := val
		if f.Secret && val != "" {
			display = maskSecret(val)
		}
		if display == "" {
			display = st.Subtle.Render("(not set)")
		} else {
			display = st.Base.Render(display)
		}

		label := st.Label.Render(padRight(f.Label, 22))
		cursor := "  "
		if i == m.settingsCursor {
			cursor = st.Accent.Render("\u258C ")
			label = st.Title.Render(padRight(f.Label, 22))
		}

		row := cursor + label + st.Base.Render(" ") + display

		// If editing this field, show the input instead.
		if m.settingsEditing && i == m.settingsCursor {
			row = cursor + label + st.Base.Render(" ") + m.settingsInput.View()
		}

		rows = append(rows, row)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		sep,
		"",
		strings.Join(rows, "\n"),
		"",
		sep,
		hint,
	)
	return st.PaneBorder.Width(w - 2).Height(h - 2).Render(content)
}

// onSettingsKey handles key events in settings mode.
func (m *Model) onSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	fields := settingsFields()

	if m.settingsEditing {
		switch {
		case keyMatches(m.keys.Cancel, msg):
			m.settingsEditing = false
			m.settingsInput.Blur()
			return m, nil
		case keyMatches(m.keys.Submit, msg):
			// Apply the value.
			cfg := m.deps.Config.GetConfig()
			fields[m.settingsCursor].Apply(&cfg, m.settingsInput.Value())
			if err := m.deps.Config.SetConfig(cfg); err != nil {
				m.setStatus(false, "save failed: "+err.Error())
			} else {
				m.setStatus(true, "saved "+fields[m.settingsCursor].Key)
			}
			m.settingsEditing = false
			m.settingsInput.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.settingsInput, cmd = m.settingsInput.Update(msg)
		return m, cmd
	}

	switch {
	case keyMatches(m.keys.Cancel, msg), keyMatches(m.keys.Settings, msg):
		m.mode = modeList
		return m, nil
	case keyMatches(m.keys.Quit, msg):
		m.mode = modeList
		return m, nil
	case keyMatches(m.keys.Up, msg):
		m.settingsCursor--
		if m.settingsCursor < 0 {
			m.settingsCursor = len(fields) - 1
		}
		return m, nil
	case keyMatches(m.keys.Down, msg):
		m.settingsCursor++
		if m.settingsCursor >= len(fields) {
			m.settingsCursor = 0
		}
		return m, nil
	case keyMatches(m.keys.Submit, msg):
		// Enter edit mode for the selected field.
		cfg := m.deps.Config.GetConfig()
		f := fields[m.settingsCursor]
		m.settingsEditing = true
		m.settingsInput.Placeholder = f.Placeholder
		m.settingsInput.SetValue(f.Value(cfg))
		m.settingsInput.Focus()
		m.settingsInput.CursorEnd()
		return m, nil
	}
	return m, nil
}

// maskSecret shows only the last 4 chars of a secret.
func maskSecret(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}
