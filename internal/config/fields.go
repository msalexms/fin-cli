package config

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Field is one user-editable setting. The same registry drives
// "fin-cli config get/set/unset" and the TUI settings panel, so both always
// accept the same keys with the same validation.
type Field struct {
	Key    string // dotted key, e.g. "finnhub.api_key"
	Label  string // human label for the TUI
	Hint   string // placeholder / short help
	Secret bool   // masked when displayed
	Env    string // environment variable that overrides it, if any
	Get    func(Config) string
	Set    func(*Config, string) error // validates; "" resets optional fields
}

// Fields lists every editable setting in display order.
var Fields = []Field{
	keyField("finnhub.api_key", "Finnhub API key", "FIN_CLI_FINNHUB_KEY", func(c *Config) *string { return &c.Finnhub.APIKey }),
	keyField("twelvedata.api_key", "Twelve Data API key", "FIN_CLI_TWELVEDATA_KEY", func(c *Config) *string { return &c.TwelveData.APIKey }),
	keyField("alphavantage.api_key", "Alpha Vantage API key", "FIN_CLI_ALPHAVANTAGE_KEY", func(c *Config) *string { return &c.AlphaVantage.APIKey }),
	keyField("openfigi.api_key", "OpenFIGI API key", "FIN_CLI_OPENFIGI_KEY", func(c *Config) *string { return &c.OpenFIGI.APIKey }),
	{
		Key:   "polling_interval",
		Label: "Refresh interval",
		Hint:  "e.g. 1m, 5m, 1h (min 30s)",
		Get:   func(c Config) string { return c.PollingInterval.String() },
		Set: func(c *Config, v string) error {
			if v == "" {
				c.PollingInterval = Duration(DefaultPolling)
				return nil
			}
			d, err := time.ParseDuration(strings.TrimSpace(v))
			if err != nil {
				return fmt.Errorf("invalid duration %q (try 1m, 5m, 1h)", v)
			}
			if d < MinPolling || d > MaxPolling {
				return fmt.Errorf("interval must be between %s and %s", Duration(MinPolling), Duration(MaxPolling))
			}
			c.PollingInterval = Duration(d)
			return nil
		},
	},
	chainField("providers", "Quote providers", QuoteProviders, func(c *Config) *[]string { return &c.Providers }),
	chainField("history_providers", "Chart providers", HistoryProviders, func(c *Config) *[]string { return &c.HistoryProviders }),
	{
		Key:   "ui.sort_mode",
		Label: "Watchlist order",
		Hint:  strings.Join(SortModes, " | "),
		Get:   func(c Config) string { return c.UI.SortMode },
		Set: func(c *Config, v string) error {
			v = strings.ToLower(strings.TrimSpace(v))
			if v == "" {
				v = SortManual
			}
			if normalizeSort(v) != v {
				return fmt.Errorf("unknown sort mode %q (valid: %s)", v, strings.Join(SortModes, ", "))
			}
			c.UI.SortMode = v
			return nil
		},
	},
}

// FieldByKey looks a field up by its dotted key.
func FieldByKey(key string) (Field, bool) {
	for _, f := range Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// Keys returns the supported keys, for help and error messages.
func Keys() []string {
	ks := make([]string, len(Fields))
	for i, f := range Fields {
		ks[i] = f.Key
	}
	return ks
}

// Mask hides all but the last four characters of a secret.
func Mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "••••"
	}
	return "••••" + s[len(s)-4:]
}

func keyField(key, label, env string, ptr func(*Config) *string) Field {
	return Field{
		Key: key, Label: label, Hint: "paste key · empty to clear", Secret: true, Env: env,
		Get: func(c Config) string { return *ptr(&c) },
		Set: func(c *Config, v string) error {
			v = strings.TrimSpace(v)
			for _, r := range v {
				if r > unicode.MaxASCII || !unicode.IsPrint(r) || r == ' ' {
					return fmt.Errorf("api keys may only contain printable ASCII characters")
				}
			}
			*ptr(c) = v
			return nil
		},
	}
}

func chainField(key, label string, allowed []string, ptr func(*Config) *[]string) Field {
	return Field{
		Key: key, Label: label, Hint: strings.Join(allowed, ", "),
		Get: func(c Config) string { return strings.Join(*ptr(&c), ", ") },
		Set: func(c *Config, v string) error {
			if strings.TrimSpace(v) == "" {
				def := Default()
				*ptr(c) = *ptr(&def)
				return nil
			}
			chain, err := parseChain(v, allowed)
			if err != nil {
				return err
			}
			*ptr(c) = chain
			return nil
		},
	}
}
