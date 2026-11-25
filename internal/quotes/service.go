// Package quotes serves quotes and price history to the CLI and TUI. It
// combines the provider chains with a disk cache for quotes (with stale
// fallback when every provider is down), an in-memory cache for history,
// request de-duplication and a shared rate limit.
package quotes

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
	"golang.org/x/time/rate"

	"github.com/msalexms/fin-cli/internal/cache"
	"github.com/msalexms/fin-cli/internal/domain"
)

// historyTTL is how long daily candles are reused before refetching.
const historyTTL = 30 * time.Minute

// Service is safe for concurrent use. Its providers can be swapped at
// runtime with Configure (e.g. after the user edits API keys in the TUI).
type Service struct {
	cache   *cache.Store
	limiter *rate.Limiter
	sf      singleflight.Group

	mu      sync.RWMutex
	quote   []domain.Provider
	history []domain.Provider
	ttl     time.Duration

	histMu sync.Mutex
	hist   map[string]histEntry
}

type histEntry struct {
	at      time.Time
	candles []domain.Candle
}

// New returns a Service backed by the given quote cache.
func New(c *cache.Store) *Service {
	return &Service{
		cache:   c,
		limiter: rate.NewLimiter(rate.Every(200*time.Millisecond), 10), // 5 calls/s, bursts of 10
		hist:    make(map[string]histEntry),
		ttl:     5 * time.Minute,
	}
}

// Configure replaces the provider chains and the quote freshness window, and
// drops cached history so charts reflect the new providers.
func (s *Service) Configure(quote, history []domain.Provider, ttl time.Duration) {
	s.mu.Lock()
	s.quote, s.history, s.ttl = quote, history, ttl
	s.mu.Unlock()
	s.histMu.Lock()
	clear(s.hist)
	s.histMu.Unlock()
}

func (s *Service) snapshot() (quote, history []domain.Provider, ttl time.Duration) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.quote, s.history, s.ttl
}

// Get returns a quote for sym. Unless force is set, a cached quote younger
// than the polling interval is returned without touching the network. When
// all providers fail transiently, the last cached quote is returned with
// Stale=true instead of an error.
func (s *Service) Get(ctx context.Context, sym domain.Ticker, force bool) (domain.Quote, error) {
	sym, err := domain.ParseTicker(string(sym))
	if err != nil {
		return domain.Quote{}, err
	}
	providers, _, ttl := s.snapshot()
	key := "Q_" + string(sym)

	if !force {
		if e, err := cache.Get[domain.Quote](s.cache, key); err == nil && time.Since(e.FetchedAt) <= ttl {
			return e.Value, nil
		}
	}

	v, err, _ := s.sf.Do(key, func() (any, error) {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		q, err := runChain(ctx, providers, func(p domain.Provider) (domain.Quote, error) {
			return p.Quote(ctx, sym)
		})
		if err != nil {
			if domain.IsTransient(err) {
				if e, cerr := cache.Get[domain.Quote](s.cache, key); cerr == nil {
					stale := e.Value
					stale.Stale = true
					return stale, nil
				}
			}
			return nil, err
		}
		q.Symbol = sym
		_ = cache.Set(s.cache, key, q)
		return q, nil
	})
	if err != nil {
		return domain.Quote{}, err
	}
	return v.(domain.Quote), nil
}

// History returns up to sessions daily candles, oldest first. Results are
// reused for historyTTL unless force is set.
func (s *Service) History(ctx context.Context, sym domain.Ticker, sessions int, force bool) ([]domain.Candle, error) {
	sym, err := domain.ParseTicker(string(sym))
	if err != nil {
		return nil, err
	}
	_, providers, _ := s.snapshot()
	key := fmt.Sprintf("H_%s_%d", sym, sessions)

	if !force {
		s.histMu.Lock()
		e, ok := s.hist[key]
		s.histMu.Unlock()
		if ok && time.Since(e.at) <= historyTTL {
			return e.candles, nil
		}
	}

	v, err, _ := s.sf.Do(key, func() (any, error) {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		candles, err := runChain(ctx, providers, func(p domain.Provider) ([]domain.Candle, error) {
			return p.History(ctx, sym, sessions)
		})
		if err != nil {
			return nil, err
		}
		s.histMu.Lock()
		s.hist[key] = histEntry{at: time.Now(), candles: candles}
		s.histMu.Unlock()
		return candles, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]domain.Candle), nil
}
