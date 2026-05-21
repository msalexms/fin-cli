package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"fin-cli/internal/cache"
	"fin-cli/internal/config"
	"fin-cli/internal/domain"
	"fin-cli/internal/httpx"
	"fin-cli/internal/isin"
	"fin-cli/internal/locale"
	"fin-cli/internal/logging"
	"fin-cli/internal/providers/alphavantage"
	"fin-cli/internal/providers/finnhub"
	"fin-cli/internal/providers/openfigi"
	"fin-cli/internal/providers/twelvedata"
	"fin-cli/internal/providers/yahoo"
	"fin-cli/internal/quotes"
	"fin-cli/internal/throttle"
	"fin-cli/internal/watchlist"
)

// App bundles dependencies required by every subcommand.
type App struct {
	Paths     config.Paths
	Config    config.Config
	Logger    *slog.Logger
	LogCloser io.Closer

	HTTP       *httpx.Client
	Throttle   *throttle.Limiter
	Printer    locale.Printer
	QuoteStore *cache.Store
	ISINStore  *cache.Store
	Watchlist  *watchlist.Store

	Quotes *quotes.Service
	ISINs  *isin.Service

	NoColor   bool
	ASCIIOnly bool

	finnhubKey      string
	openfigiKey     string
	twelvedataKey   string
	alphavantageKey string
}

// AppOptions are toggles passed from the root command.
type AppOptions struct {
	Debug           bool
	FinnhubKey      string
	OpenFIGIKey     string
	TwelveDataKey   string
	AlphaVantageKey string
	ConfigPath      string
	WatchlistPath   string
}

// NewApp wires up all dependencies.
func NewApp(opt AppOptions) (*App, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, fmt.Errorf("paths: %w", err)
	}
	if opt.ConfigPath != "" {
		paths.ConfigFile = opt.ConfigPath
	}
	if opt.WatchlistPath != "" {
		paths.Watchlist = opt.WatchlistPath
	}
	if err := paths.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("mkdirs: %w", err)
	}

	logger, closer, err := logging.Setup(paths.LogFile, opt.Debug)
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		_ = closer.Close()
		return nil, fmt.Errorf("config: %w", err)
	}

	finnhubKey := firstNonEmpty(opt.FinnhubKey, os.Getenv("FIN_CLI_FINNHUB_KEY"), cfg.Finnhub.APIKey)
	openfigiKey := firstNonEmpty(opt.OpenFIGIKey, os.Getenv("FIN_CLI_OPENFIGI_KEY"), cfg.OpenFIGI.APIKey)
	twelvedataKey := firstNonEmpty(opt.TwelveDataKey, os.Getenv("FIN_CLI_TWELVEDATA_KEY"), cfg.TwelveData.APIKey)
	alphavantageKey := firstNonEmpty(opt.AlphaVantageKey, os.Getenv("FIN_CLI_ALPHAVANTAGE_KEY"), cfg.AlphaVantage.APIKey)

	httpC := httpx.New()
	lim := throttle.NewPerMinute(60, 5)
	printer := locale.Detect()

	qStore := cache.New(paths.QuoteCache)
	iStore := cache.New(paths.ISINCache)
	wStore := watchlist.New(paths.Watchlist)

	// --- Build provider instances ---
	finnhubProv := finnhub.New(finnhubKey)
	yahooProv := yahoo.New(httpC)
	twelvedataProv := twelvedata.New(httpC, twelvedataKey)
	alphaProv := alphavantage.New(httpC, alphavantageKey)
	openfigiProv := openfigi.New(httpC, openfigiKey)

	// Provider registry: maps name -> MarketProvider instance.
	providerRegistry := map[string]domain.MarketProvider{
		"finnhub":      finnhubProv,
		"yahoo":        yahooProv,
		"twelvedata":   twelvedataProv,
		"alphavantage": alphaProv,
	}

	// Build the quote provider chain from config.
	quoteProviders := buildProviderChain(cfg.Providers, providerRegistry, finnhubKey, twelvedataKey, alphavantageKey)

	// Build the history provider chain from config.
	historyProviders := buildHistoryChain(cfg.HistoryProviders, providerRegistry)

	chain := quotes.NewProviderChain(quoteProviders...)
	histChain := quotes.NewHistoryChain(historyProviders...)

	// TTL of the quotes cache == polling interval.
	quoteSvc := quotes.NewWithChain(chain, histChain, qStore, lim, cfg.PollingInterval.Std())
	isinSvc := isin.New(openfigiProv, iStore, 0)

	noColor := envFlag("NO_COLOR")
	forceColor := envFlag("FORCE_COLOR")
	if forceColor {
		noColor = false
	}

	app := &App{
		Paths:           paths,
		Config:          cfg,
		Logger:          logger,
		LogCloser:       closer,
		HTTP:            httpC,
		Throttle:        lim,
		Printer:         printer,
		QuoteStore:      qStore,
		ISINStore:       iStore,
		Watchlist:       wStore,
		Quotes:          quoteSvc,
		ISINs:           isinSvc,
		NoColor:         noColor,
		ASCIIOnly:       printer.ASCIIOnly(),
		finnhubKey:      finnhubKey,
		openfigiKey:     openfigiKey,
		twelvedataKey:   twelvedataKey,
		alphavantageKey: alphavantageKey,
	}

	if quoteSvc.TTL <= 0 {
		quoteSvc.TTL = 5 * time.Minute
	}
	app.Logger.Debug("app initialized",
		slog.String("config", paths.ConfigFile),
		slog.Bool("has_finnhub_key", finnhubKey != ""),
		slog.Bool("has_openfigi_key", openfigiKey != ""),
		slog.Bool("has_twelvedata_key", twelvedataKey != ""),
		slog.Bool("has_alphavantage_key", alphavantageKey != ""),
		slog.Any("providers", cfg.Providers),
		slog.Any("history_providers", cfg.HistoryProviders),
	)

	return app, nil
}

// Close releases resources held by the app.
func (a *App) Close() error {
	if a.LogCloser == nil {
		return nil
	}
	return a.LogCloser.Close()
}

// RenderOptions returns render options derived from the app state.
func (a *App) RenderOptions() RenderOptions {
	return RenderOptions{
		NoColor:   a.NoColor,
		ASCIIOnly: a.ASCIIOnly,
		Printer:   a.Printer,
	}
}

// HasFinnhubKey reports whether an API key is configured.
func (a *App) HasFinnhubKey() bool { return a.finnhubKey != "" }

// GetConfig returns the current in-memory configuration.
func (a *App) GetConfig() config.Config { return a.Config }

// SetConfig persists cfg to disk and updates the in-memory copy.
func (a *App) SetConfig(cfg config.Config) error {
	if err := config.Save(a.Paths.ConfigFile, cfg); err != nil {
		return err
	}
	a.Config = cfg
	return nil
}

// ResolveInput returns the ticker to query, honoring --isin and autodetection.
// If arg looks like an ISIN (or forceISIN is true), it is resolved via the
// ISIN service; otherwise the argument is used verbatim as a ticker.
func (a *App) ResolveInput(ctx context.Context, arg string, forceISIN bool) (domain.Ticker, error) {
	if forceISIN || domain.IsISIN(arg) {
		return a.ISINs.Resolve(ctx, domain.ISIN(arg))
	}
	return domain.Ticker(arg), nil
}

// --- provider chain builders ---

// buildProviderChain creates an ordered slice of MarketProviders from config names.
// Providers that require an API key but have none are still included; the chain
// will receive ErrNoAPIKey and skip them at runtime.
func buildProviderChain(names []string, registry map[string]domain.MarketProvider, finnhubKey, twelvedataKey, alphaKey string) []domain.MarketProvider {
	if len(names) == 0 {
		// Fallback: if config has no providers, use finnhub + yahoo.
		names = []string{"finnhub", "yahoo"}
	}
	var providers []domain.MarketProvider
	for _, name := range names {
		p, ok := registry[name]
		if !ok {
			continue
		}
		providers = append(providers, p)
	}
	if len(providers) == 0 {
		// Safety: always have at least yahoo (keyless).
		if p, ok := registry["yahoo"]; ok {
			providers = append(providers, p)
		}
	}
	return providers
}

// buildHistoryChain creates an ordered slice of HistoryProviders from config names.
func buildHistoryChain(names []string, registry map[string]domain.MarketProvider) []domain.HistoryProvider {
	if len(names) == 0 {
		names = []string{"yahoo"}
	}
	var providers []domain.HistoryProvider
	for _, name := range names {
		p, ok := registry[name]
		if !ok {
			continue
		}
		// MarketProvider satisfies HistoryProvider (it has History method).
		providers = append(providers, p)
	}
	return providers
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func envFlag(k string) bool {
	v := os.Getenv(k)
	return v != "" && v != "0" && v != "false"
}
