// Package isin resolves ISIN codes to tickers via OpenFIGI, with a 30-day disk cache.
package isin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fin-cli/internal/cache"
	"fin-cli/internal/domain"
)

// DefaultTTL is the default freshness window for ISIN→ticker mappings.
// Mappings change extremely rarely, so 30 days is conservative.
const DefaultTTL = 30 * 24 * time.Hour

// Service caches resolutions from a domain.IsinResolver.
type Service struct {
	Inner domain.IsinResolver
	Cache *cache.Store
	TTL   time.Duration
}

// New builds a Service.
func New(inner domain.IsinResolver, c *cache.Store, ttl time.Duration) *Service {
	if ttl == 0 {
		ttl = DefaultTTL
	}
	return &Service{Inner: inner, Cache: c, TTL: ttl}
}

// Resolve returns the ticker for isin, using the cache when fresh.
func (s *Service) Resolve(ctx context.Context, isin domain.ISIN) (domain.Ticker, error) {
	norm := domain.ISIN(strings.ToUpper(strings.TrimSpace(string(isin))))
	if !domain.IsISIN(string(norm)) {
		return "", fmt.Errorf("%w: %q is not a valid ISIN", domain.ErrInvalidInput, isin)
	}
	key := "ISIN_" + string(norm)

	if e, err := cache.Get[string](s.Cache, key); err == nil {
		if time.Since(e.FetchedAt) <= s.TTL && e.Value != "" {
			return domain.Ticker(e.Value), nil
		}
	}

	t, err := s.Inner.Resolve(ctx, norm)
	if err != nil {
		return "", err
	}
	_ = cache.Set(s.Cache, key, string(t))
	return t, nil
}
