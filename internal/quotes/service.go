// Package quotes orchestrates the cache, throttler, and provider chain to serve
// quotes to CLI and TUI. It deduplicates concurrent requests via singleflight.
package quotes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"fin-cli/internal/cache"
	"fin-cli/internal/domain"
	"fin-cli/internal/throttle"
)

// DefaultCacheTTL is the default freshness window for cached quotes.
const DefaultCacheTTL = 5 * time.Minute

// Service implements domain.QuoteService.
type Service struct {
	// Provider is the legacy single-provider field (kept for backwards compat).
	// When Chain is set, Provider is ignored for Quote calls.
	Provider domain.MarketProvider

	// Chain is the ordered list of providers for Quote calls.
	// If nil, Provider is used as a single-element chain.
	Chain *ProviderChain

	// HistoryProv is the legacy single history provider.
	// When HistoryChain is set, HistoryProv is ignored.
	HistoryProv domain.HistoryProvider

	// HistoryChain is the ordered list of providers for History calls.
	// If nil, HistoryProv or Provider.History is used.
	HistoryChain *HistoryChain

	// Limiter rate-limits outbound API calls (shared across all providers).
	// Per-provider limiters are handled at construction in cli/app.go if needed.
	Limiter *throttle.Limiter

	Cache *cache.Store
	TTL   time.Duration

	sf singleflight.Group
}

// New builds a Service. If ttl is zero, DefaultCacheTTL is used.
// This constructor preserves the legacy single-provider API.
func New(p domain.MarketProvider, c *cache.Store, l *throttle.Limiter, ttl time.Duration) *Service {
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	return &Service{Provider: p, Cache: c, Limiter: l, TTL: ttl}
}

// NewWithChain builds a Service using a ProviderChain and HistoryChain.
func NewWithChain(chain *ProviderChain, histChain *HistoryChain, c *cache.Store, l *throttle.Limiter, ttl time.Duration) *Service {
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	return &Service{
		Chain:        chain,
		HistoryChain: histChain,
		Cache:        c,
		Limiter:      l,
		TTL:          ttl,
	}
}

// WithHistoryProvider sets a dedicated provider for History calls.
// Deprecated: use NewWithChain with a HistoryChain instead.
func (s *Service) WithHistoryProvider(h domain.HistoryProvider) *Service {
	s.HistoryProv = h
	return s
}

// Get returns a quote. When force is false, a fresh cached entry is returned
// without hitting the provider; otherwise the cache is bypassed for reads but
// still written on success.
//
// Graceful degradation: if all providers return transient errors and a cached
// entry exists (even if stale), the cached value is returned with Source=SourceCache.
func (s *Service) Get(ctx context.Context, sym domain.Ticker, force bool) (domain.Quote, error) {
	sym = domain.Ticker(strings.ToUpper(strings.TrimSpace(string(sym))))
	if sym == "" {
		return domain.Quote{}, fmt.Errorf("%w: empty ticker", domain.ErrInvalidInput)
	}
	key := "Q_" + string(sym)

	if !force {
		if q, ok := s.freshCached(key); ok {
			return q, nil
		}
	}

	// singleflight dedupes concurrent refreshes of the same key.
	v, err, _ := s.sf.Do(key, func() (any, error) {
		if err := s.Limiter.Wait(ctx); err != nil {
			return nil, err
		}

		q, err := s.quoteFromChain(ctx, sym)
		if err != nil {
			// On transient errors, fall back to stale cache if any.
			if isTransient(err) {
				if stale, ok := s.staleCached(key); ok {
					return stale, nil
				}
			}
			return domain.Quote{}, err
		}
		_ = cache.Set(s.Cache, key, q)
		return q, nil
	})
	if err != nil {
		return domain.Quote{}, err
	}
	return v.(domain.Quote), nil
}

// quoteFromChain delegates to the Chain if set, otherwise uses Provider.
func (s *Service) quoteFromChain(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	if s.Chain != nil && s.Chain.Len() > 0 {
		return s.Chain.Quote(ctx, sym)
	}
	if s.Provider != nil {
		return s.Provider.Quote(ctx, sym)
	}
	return domain.Quote{}, fmt.Errorf("%w: no providers configured", domain.ErrNoAPIKey)
}

// History returns candles. Uses HistoryChain if set, then HistoryProv, then Provider.
func (s *Service) History(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error) {
	if err := s.Limiter.Wait(ctx); err != nil {
		return nil, err
	}

	if s.HistoryChain != nil {
		return s.HistoryChain.History(ctx, sym, r)
	}
	if s.HistoryProv != nil {
		return s.HistoryProv.History(ctx, sym, r)
	}
	if s.Chain != nil && s.Chain.Len() > 0 {
		return s.Chain.History(ctx, sym, r)
	}
	if s.Provider != nil {
		return s.Provider.History(ctx, sym, r)
	}
	return nil, fmt.Errorf("%w: no history providers configured", domain.ErrNoAPIKey)
}

// freshCached returns a cached quote whose FetchedAt is within TTL.
func (s *Service) freshCached(key string) (domain.Quote, bool) {
	e, err := cache.Get[domain.Quote](s.Cache, key)
	if err != nil {
		return domain.Quote{}, false
	}
	if time.Since(e.FetchedAt) > s.TTL {
		return domain.Quote{}, false
	}
	q := e.Value
	q.Source = domain.SourceCache
	q.FetchedAt = e.FetchedAt
	return q, true
}

// staleCached returns the last cached quote regardless of TTL, marked as cache source.
func (s *Service) staleCached(key string) (domain.Quote, bool) {
	e, err := cache.Get[domain.Quote](s.Cache, key)
	if err != nil {
		return domain.Quote{}, false
	}
	q := e.Value
	q.Source = domain.SourceCache
	q.FetchedAt = e.FetchedAt
	return q, true
}

func isTransient(err error) bool {
	return errors.Is(err, domain.ErrNetwork) ||
		errors.Is(err, domain.ErrUnavailable) ||
		errors.Is(err, domain.ErrRateLimited)
}
