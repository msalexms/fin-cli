// Package app wires fin-cli together: paths, config, logging, providers,
// caches and services. Both the CLI commands and the TUI work against an
// *App, and config changes made at runtime are applied without a restart.
package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/msalexms/fin-cli/internal/cache"
	"github.com/msalexms/fin-cli/internal/config"
	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/httpx"
	"github.com/msalexms/fin-cli/internal/isin"
	"github.com/msalexms/fin-cli/internal/locale"
	"github.com/msalexms/fin-cli/internal/logging"
	"github.com/msalexms/fin-cli/internal/providers/alphavantage"
	"github.com/msalexms/fin-cli/internal/providers/finnhub"
	"github.com/msalexms/fin-cli/internal/providers/openfigi"
	"github.com/msalexms/fin-cli/internal/providers/twelvedata"
	"github.com/msalexms/fin-cli/internal/providers/yahoo"
	"github.com/msalexms/fin-cli/internal/quotes"
	"github.com/msalexms/fin-cli/internal/watchlist"
)

// Options are the global command-line flags.
type Options struct {
	Debug         bool
	ConfigPath    string
	WatchlistPath string
	// KeyFlags maps a config key (e.g. "finnhub.api_key") to a --*-key flag value.
	KeyFlags map[string]string
}

// App bundles the services every command needs.
type App struct {
	Paths      config.Paths
	Logger     *slog.Logger
	Printer    locale.Printer
	Watchlist  *watchlist.Store
	Quotes     *quotes.Service
	QuoteCache *cache.Store
	ISINCache  *cache.Store

	opts   Options
	http   *httpx.Client
	closer io.Closer

	mu   sync.RWMutex
	cfg  config.Config
	isin *isin.Service
}

// New loads the configuration and builds every service.
func New(opts Options) (*App, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, fmt.Errorf("locate home directory: %w", err)
	}
	if opts.ConfigPath != "" {
		paths.ConfigFile = opts.ConfigPath
	}
	if opts.WatchlistPath != "" {
		paths.Watchlist = opts.WatchlistPath
	}

	logger, closer, err := logging.Setup(paths.LogFile, opts.Debug)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	slog.SetDefault(logger)

	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		_ = closer.Close()
		return nil, fmt.Errorf("config: %w", err)
	}

	a := &App{
		Paths:      paths,
		Logger:     logger,
		Printer:    locale.Detect(),
		Watchlist:  watchlist.New(paths.Watchlist),
		QuoteCache: cache.New(paths.QuoteCache),
		ISINCache:  cache.New(paths.ISINCache),
		opts:       opts,
		http:       httpx.New(),
		closer:     closer,
	}
	a.Quotes = quotes.New(a.QuoteCache)
	a.apply(cfg)
	return a, nil
}

// Close releases the log file.
func (a *App) Close() error { return a.closer.Close() }

// Config returns the current configuration.
func (a *App) Config() config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

// UpdateConfig saves cfg and rebuilds the providers with it.
func (a *App) UpdateConfig(cfg config.Config) error {
	if err := config.Save(a.Paths.ConfigFile, cfg); err != nil {
		return err
	}
	a.apply(cfg)
	return nil
}

// Override reports where a field's effective value comes from when it is not
// config.toml: "flag" or the environment variable name. Empty otherwise.
func (a *App) Override(f config.Field) string {
	if a.opts.KeyFlags[f.Key] != "" {
		return "--" + strings.TrimSuffix(f.Key, ".api_key") + "-key flag"
	}
	if f.Env != "" && os.Getenv(f.Env) != "" {
		return "$" + f.Env
	}
	return ""
}

// key returns the effective API key for a config key: flag, then
// environment variable, then config file.
func (a *App) key(cfg config.Config, key string) string {
	f, _ := config.FieldByKey(key)
	if v := a.opts.KeyFlags[key]; v != "" {
		return v
	}
	if v := os.Getenv(f.Env); f.Env != "" && v != "" {
		return v
	}
	return f.Get(cfg)
}

func (a *App) apply(cfg config.Config) {
	registry := map[string]domain.Provider{
		config.Finnhub:      finnhub.New(a.http, a.key(cfg, "finnhub.api_key")),
		config.Yahoo:        yahoo.New(a.http),
		config.TwelveData:   twelvedata.New(a.http, a.key(cfg, "twelvedata.api_key")),
		config.AlphaVantage: alphavantage.New(a.http, a.key(cfg, "alphavantage.api_key")),
	}
	pick := func(names []string) []domain.Provider {
		out := make([]domain.Provider, 0, len(names))
		for _, n := range names {
			if p, ok := registry[n]; ok {
				out = append(out, p)
			}
		}
		return out
	}
	a.Quotes.Configure(pick(cfg.Providers), pick(cfg.HistoryProviders), cfg.PollingInterval.Std())

	a.mu.Lock()
	a.cfg = cfg
	a.isin = isin.New(openfigi.New(a.http, a.key(cfg, "openfigi.api_key")), a.ISINCache)
	a.mu.Unlock()

	a.Logger.Debug("config applied",
		"providers", cfg.Providers, "history_providers", cfg.HistoryProviders,
		"polling", cfg.PollingInterval.String())
}

// ResolveInput turns user input into a ticker. ISINs (detected by format and
// checksum, or forced with forceISIN) are resolved through OpenFIGI.
func (a *App) ResolveInput(ctx context.Context, raw string, forceISIN bool) (domain.Ticker, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if forceISIN || domain.IsISIN(s) {
		a.mu.RLock()
		svc := a.isin
		a.mu.RUnlock()
		return svc.Resolve(ctx, domain.ISIN(s))
	}
	return domain.ParseTicker(s)
}
