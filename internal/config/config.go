// Package config loads, validates and saves fin-cli's config.toml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/msalexms/fin-cli/internal/fsutil"
)

// SchemaVersion is the config format version written by this build.
const SchemaVersion = 2

// Polling bounds keep the TUI from hammering free-tier APIs.
const (
	MinPolling     = 30 * time.Second
	MaxPolling     = 24 * time.Hour
	DefaultPolling = 5 * time.Minute
)

// Provider names.
const (
	Finnhub      = "finnhub"
	Yahoo        = "yahoo"
	TwelveData   = "twelvedata"
	AlphaVantage = "alphavantage"
)

// QuoteProviders and HistoryProviders list the valid chain members, in the
// recommended order. Finnhub has no free candles, so it is quote-only.
var (
	QuoteProviders   = []string{Finnhub, Yahoo, TwelveData, AlphaVantage}
	HistoryProviders = []string{Yahoo, TwelveData, AlphaVantage}
)

// Sort modes for the watchlist.
const (
	SortManual     = "manual"
	SortChangeDesc = "change_desc"
	SortChangeAsc  = "change_asc"
	SortAlpha      = "alpha"
	SortVolume     = "volume"
)

// SortModes lists the valid sort modes in cycling order.
var SortModes = []string{SortManual, SortChangeDesc, SortChangeAsc, SortAlpha, SortVolume}

// Config is the on-disk configuration.
type Config struct {
	SchemaVersion    int       `toml:"schema_version"`
	PollingInterval  Duration  `toml:"polling_interval"`
	Providers        []string  `toml:"providers"`
	HistoryProviders []string  `toml:"history_providers"`
	Finnhub          KeyConfig `toml:"finnhub"`
	OpenFIGI         KeyConfig `toml:"openfigi"`
	TwelveData       KeyConfig `toml:"twelvedata"`
	AlphaVantage     KeyConfig `toml:"alphavantage"`
	UI               UIConfig  `toml:"ui"`
}

// KeyConfig holds a provider API key.
type KeyConfig struct {
	APIKey string `toml:"api_key"`
}

// UIConfig gathers presentation options.
type UIConfig struct {
	SortMode string `toml:"sort_mode"`
}

// Default returns the first-run configuration. It works without any API key:
// Yahoo serves quotes and charts, Finnhub adds fundamentals once keyed.
func Default() Config {
	return Config{
		SchemaVersion:    SchemaVersion,
		PollingInterval:  Duration(DefaultPolling),
		Providers:        []string{Finnhub, Yahoo},
		HistoryProviders: []string{Yahoo, TwelveData, AlphaVantage},
		UI:               UIConfig{SortMode: SortManual},
	}
}

// Duration marshals as a TOML string such as "5m".
type Duration time.Duration

// UnmarshalText parses a Go duration string.
func (d *Duration) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("invalid duration %q", b)
	}
	*d = Duration(v)
	return nil
}

// MarshalText formats the duration.
func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// Std returns the time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// String formats without trailing zero units ("5m", not "5m0s").
func (d Duration) String() string {
	s := time.Duration(d).String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

// Load reads path, writing a documented default file on first run. Unknown
// or invalid values are replaced by defaults rather than failing, so a hand
// edit can never lock the user out of the app.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		return cfg, Save(path, cfg)
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.SchemaVersion > SchemaVersion {
		return Config{}, fmt.Errorf("%s uses schema %d; this fin-cli supports up to %d, please upgrade",
			path, cfg.SchemaVersion, SchemaVersion)
	}
	cfg.normalize() // also upgrades v1 files, which had no provider chains
	return cfg, nil
}

// normalize replaces invalid values with defaults.
func (c *Config) normalize() {
	def := Default()
	c.SchemaVersion = SchemaVersion
	if d := c.PollingInterval.Std(); d < MinPolling || d > MaxPolling {
		c.PollingInterval = def.PollingInterval
	}
	if p, err := parseChain(strings.Join(c.Providers, ","), QuoteProviders); err == nil {
		c.Providers = p
	} else {
		c.Providers = def.Providers
	}
	if p, err := parseChain(strings.Join(c.HistoryProviders, ","), HistoryProviders); err == nil {
		c.HistoryProviders = p
	} else {
		c.HistoryProviders = def.HistoryProviders
	}
	c.UI.SortMode = normalizeSort(c.UI.SortMode)
}

// Save writes cfg atomically, keeping the explanatory comments.
func Save(path string, cfg Config) error {
	unlock, err := fsutil.Lock(path)
	if err != nil {
		return err
	}
	defer unlock()
	var buf bytes.Buffer
	if err := fileTemplate.Execute(&buf, cfg); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, buf.Bytes())
}

func parseChain(raw string, allowed []string) ([]string, error) {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if !slices.Contains(allowed, p) {
			return nil, fmt.Errorf("unknown provider %q (valid: %s)", p, strings.Join(allowed, ", "))
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("at least one provider is required")
	}
	return out, nil
}

func normalizeSort(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case SortChangeDesc, "%desc":
		return SortChangeDesc
	case SortChangeAsc, "%asc":
		return SortChangeAsc
	case SortAlpha:
		return SortAlpha
	case SortVolume:
		return SortVolume
	default:
		return SortManual
	}
}

var fileTemplate = template.Must(template.New("config").Funcs(template.FuncMap{
	"q": strconv.Quote,
	"list": func(ss []string) string {
		q := make([]string, len(ss))
		for i, s := range ss {
			q[i] = strconv.Quote(s)
		}
		return "[" + strings.Join(q, ", ") + "]"
	},
}).Parse(`# fin-cli configuration · https://github.com/msalexms/fin-cli
#
# Edit with "fin-cli config edit", "fin-cli config set <key> <value>" or the
# settings panel inside the TUI (key: c). Environment variables override the
# API keys below: FIN_CLI_FINNHUB_KEY, FIN_CLI_TWELVEDATA_KEY,
# FIN_CLI_ALPHAVANTAGE_KEY, FIN_CLI_OPENFIGI_KEY.

schema_version = {{.SchemaVersion}}

# How often the TUI refreshes quotes (minimum 30s), e.g. "1m", "5m", "1h".
polling_interval = {{q .PollingInterval.String}}

# Quote providers, tried in order until one answers.
# Valid: "finnhub", "yahoo", "twelvedata", "alphavantage".
# Yahoo needs no key; providers without a key are skipped.
providers = {{list .Providers}}

# Chart (daily history) providers, tried in order.
# Valid: "yahoo", "twelvedata", "alphavantage".
history_providers = {{list .HistoryProviders}}

[finnhub]
# Free key at https://finnhub.io (adds P/E, EPS, beta, market cap...).
api_key = {{q .Finnhub.APIKey}}

[twelvedata]
# Free key at https://twelvedata.com (800 requests/day).
api_key = {{q .TwelveData.APIKey}}

[alphavantage]
# Free key at https://www.alphavantage.co/support/#api-key (25 requests/day).
api_key = {{q .AlphaVantage.APIKey}}

[openfigi]
# Optional, for ISIN lookups: 25 req/min without key, 250 with one.
api_key = {{q .OpenFIGI.APIKey}}

[ui]
# Watchlist order: "manual", "change_desc", "change_asc", "alpha", "volume".
sort_mode = {{q .UI.SortMode}}
`))
