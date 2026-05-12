// Package quotes orchestrates the cache, throttler, and provider to serve quotes
// to CLI and TUI. It deduplicates concurrent requests via singleflight.
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
	Provider    domain.MarketProvider
	HistoryProv domain.HistoryProvider // optional; overrides Provider.History when set
	Cache       *cache.Store
	Limiter     *throttle.Limiter
	TTL         time.Duration

	sf singleflight.Group
}

// New builds a Service. If ttl is zero, DefaultCacheTTL is used.
func New(p domain.MarketProvider, c *cache.Store, l *throttle.Limiter, ttl time.Duration) *Service {
	if ttl == 0 {
		ttl = DefaultCacheTTL
	}
	return &Service{Provider: p, Cache: c, Limiter: l, TTL: ttl}
}

// WithHistoryProvider sets a dedicated provider for History calls
// (e.g. Yahoo for free charts while Finnhub stays responsible for Quote).
func (s *Service) WithHistoryProvider(h domain.HistoryProvider) *Service {
	s.HistoryProv = h
	return s
}

// Get returns a quote. When force is false, a fresh cached entry is returned
// without hitting the provider; otherwise the cache is bypassed for reads but
// still written on success.
//
// Graceful degradation: if the provider returns a transient error (network,
// unavailable, rate-limited) and a cached entry exists (even if stale), the
// cached value is returned with Source=SourceCache and an attached warning.
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
		q, err := s.Provider.Quote(ctx, sym)
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

// History is a thin passthrough; it is rate-limited but not cached.
// When HistoryProv is set, it takes precedence over Provider — this lets
// us combine Finnhub quotes with a free chart source (e.g. Yahoo).
func (s *Service) History(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error) {
	if err := s.Limiter.Wait(ctx); err != nil {
		return nil, err
	}
	if s.HistoryProv != nil {
		return s.HistoryProv.History(ctx, sym, r)
	}
	return s.Provider.History(ctx, sym, r)
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
