package quotes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"fin-cli/internal/domain"
)

// ProviderChain tries multiple MarketProviders in order for Quote requests.
// It stops on terminal errors (ErrNotFound, ErrUnauthorized) and advances
// to the next provider on transient errors or ErrNoAPIKey.
type ProviderChain struct {
	providers []domain.MarketProvider
}

// NewProviderChain creates a chain from the given providers (order matters).
func NewProviderChain(providers ...domain.MarketProvider) *ProviderChain {
	return &ProviderChain{providers: providers}
}

// Name returns a comma-separated list of provider names for logging.
func (c *ProviderChain) Name() string {
	if len(c.providers) == 0 {
		return "none"
	}
	if len(c.providers) == 1 {
		return c.providers[0].Name()
	}
	names := ""
	for i, p := range c.providers {
		if i > 0 {
			names += ","
		}
		names += p.Name()
	}
	return names
}

// Len returns the number of providers in the chain.
func (c *ProviderChain) Len() int { return len(c.providers) }

// Quote tries each provider in order. Returns the first successful result.
//
// Error handling:
//   - ErrNotFound: terminal, stops the chain immediately
//   - ErrNoAPIKey: skip this provider silently, try next
//   - ErrNetwork, ErrRateLimited, ErrUnavailable: transient, try next
//   - ErrUnauthorized: terminal (bad key), stops the chain
//   - Other errors: treated as transient, try next
//
// If all providers fail, returns the last meaningful error.
func (c *ProviderChain) Quote(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	if len(c.providers) == 0 {
		return domain.Quote{}, fmt.Errorf("%w: no providers configured", domain.ErrNoAPIKey)
	}

	var lastErr error
	for _, p := range c.providers {
		q, err := p.Quote(ctx, sym)
		if err == nil {
			return q, nil
		}

		slog.Debug("provider chain: quote failed",
			"provider", p.Name(),
			"symbol", sym,
			"error", err,
		)

		// Terminal errors stop the chain.
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Quote{}, err
		}
		if errors.Is(err, domain.ErrUnauthorized) {
			return domain.Quote{}, err
		}

		// ErrNoAPIKey means this provider is not configured; skip silently.
		if errors.Is(err, domain.ErrNoAPIKey) {
			continue
		}

		// Transient errors: advance to next provider.
		lastErr = err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("%w: all providers skipped (no API keys configured)", domain.ErrNoAPIKey)
	}
	return domain.Quote{}, lastErr
}

// History tries each provider in order for historical candle data.
func (c *ProviderChain) History(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error) {
	if len(c.providers) == 0 {
		return nil, fmt.Errorf("%w: no history providers configured", domain.ErrNoAPIKey)
	}

	var lastErr error
	for _, p := range c.providers {
		candles, err := p.History(ctx, sym, r)
		if err == nil {
			return candles, nil
		}

		slog.Debug("provider chain: history failed",
			"provider", p.Name(),
			"symbol", sym,
			"error", err,
		)

		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		if errors.Is(err, domain.ErrUnauthorized) {
			// For history, unauthorized is non-terminal (Finnhub free tier
			// returns 403 for candles). Try next provider.
			lastErr = err
			continue
		}
		if errors.Is(err, domain.ErrNoAPIKey) {
			continue
		}

		lastErr = err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("%w: all history providers skipped", domain.ErrNoAPIKey)
	}
	return nil, lastErr
}

// HistoryChain wraps a chain of HistoryProviders (which may not implement
// the full MarketProvider interface). It adapts them to a common History call.
type HistoryChain struct {
	providers []domain.HistoryProvider
}

// NewHistoryChain creates a history chain from the given providers.
func NewHistoryChain(providers ...domain.HistoryProvider) *HistoryChain {
	return &HistoryChain{providers: providers}
}

// History tries each provider in order.
func (c *HistoryChain) History(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error) {
	if len(c.providers) == 0 {
		return nil, fmt.Errorf("%w: no history providers configured", domain.ErrNoAPIKey)
	}

	var lastErr error
	for _, p := range c.providers {
		candles, err := p.History(ctx, sym, r)
		if err == nil {
			return candles, nil
		}

		slog.Debug("history chain: failed",
			"provider", p.Name(),
			"symbol", sym,
			"error", err,
		)

		if errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
		if errors.Is(err, domain.ErrNoAPIKey) {
			continue
		}

		lastErr = err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("%w: all history providers skipped", domain.ErrNoAPIKey)
	}
	return nil, lastErr
}
