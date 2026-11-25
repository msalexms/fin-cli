// Package isin resolves ISIN codes to tickers through a domain.ISINResolver
// with a 30-day disk cache (mappings practically never change).
package isin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/msalexms/fin-cli/internal/cache"
	"github.com/msalexms/fin-cli/internal/domain"
)

const ttl = 30 * 24 * time.Hour

// Service caches resolutions.
type Service struct {
	inner domain.ISINResolver
	cache *cache.Store
}

// New builds a Service.
func New(inner domain.ISINResolver, c *cache.Store) *Service {
	return &Service{inner: inner, cache: c}
}

// Resolve returns the ticker for isin, using the cache when fresh.
func (s *Service) Resolve(ctx context.Context, isin domain.ISIN) (domain.Ticker, error) {
	norm := strings.ToUpper(strings.TrimSpace(string(isin)))
	if !domain.IsISIN(norm) {
		return "", fmt.Errorf("%w: %q is not a valid ISIN", domain.ErrInvalidInput, isin)
	}
	key := "ISIN_" + norm
	if e, err := cache.Get[string](s.cache, key); err == nil && time.Since(e.FetchedAt) <= ttl && e.Value != "" {
		return domain.Ticker(e.Value), nil
	}
	t, err := s.inner.Resolve(ctx, domain.ISIN(norm))
	if err != nil {
		return "", err
	}
	_ = cache.Set(s.cache, key, string(t))
	return t, nil
}
