package quotes

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/msalexms/fin-cli/internal/cache"
	"github.com/msalexms/fin-cli/internal/domain"
)

type fake struct {
	name  string
	quote func(domain.Ticker) (domain.Quote, error)
	hist  func(domain.Ticker) ([]domain.Candle, error)
	calls atomic.Int32
}

func (f *fake) Name() string { return f.name }
func (f *fake) Quote(_ context.Context, s domain.Ticker) (domain.Quote, error) {
	f.calls.Add(1)
	return f.quote(s)
}
func (f *fake) History(_ context.Context, s domain.Ticker, _ int) ([]domain.Candle, error) {
	f.calls.Add(1)
	return f.hist(s)
}

func ok(name string, price float64) *fake {
	return &fake{name: name,
		quote: func(s domain.Ticker) (domain.Quote, error) {
			return domain.Quote{Symbol: s, Price: price, Source: name}, nil
		},
		hist: func(domain.Ticker) ([]domain.Candle, error) { return []domain.Candle{{Close: price}}, nil },
	}
}

func failing(name string, err error) *fake {
	e := fmt.Errorf("%s: %w", name, err)
	return &fake{name: name,
		quote: func(domain.Ticker) (domain.Quote, error) { return domain.Quote{}, e },
		hist:  func(domain.Ticker) ([]domain.Candle, error) { return nil, e },
	}
}

func newService(t *testing.T, providers ...domain.Provider) *Service {
	t.Helper()
	s := New(cache.New(t.TempDir()))
	s.Configure(providers, providers, time.Minute)
	return s
}

func TestGet_FallsThroughEveryFailure(t *testing.T) {
	for _, err := range []error{domain.ErrNotFound, domain.ErrUnauthorized, domain.ErrNetwork, domain.ErrNoAPIKey} {
		s := newService(t, failing("first", err), ok("second", 42))
		q, gerr := s.Get(context.Background(), "san.mc", true)
		if gerr != nil || q.Price != 42 || q.Source != "second" || q.Symbol != "SAN.MC" {
			t.Errorf("after %v: got %+v, %v", err, q, gerr)
		}
	}
}

func TestGet_ErrorPriority(t *testing.T) {
	s := newService(t, failing("a", domain.ErrNotFound), failing("b", domain.ErrUnauthorized), failing("c", domain.ErrNoAPIKey))
	_, err := s.Get(context.Background(), "X", true)
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized as primary", err)
	}
	var ce *ChainError
	if !errors.As(err, &ce) || len(ce.All) != 2 {
		t.Fatalf("want ChainError with the 2 configured failures, got %v", err)
	}
}

func TestGet_AllWithoutKey(t *testing.T) {
	s := newService(t, failing("a", domain.ErrNoAPIKey))
	if _, err := s.Get(context.Background(), "X", true); !errors.Is(err, domain.ErrNoAPIKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestGet_UsesFreshCacheAndStaleFallback(t *testing.T) {
	p := ok("p", 10)
	s := newService(t, p)
	ctx := context.Background()
	if _, err := s.Get(ctx, "AAPL", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "AAPL", false); err != nil {
		t.Fatal(err)
	}
	if n := p.calls.Load(); n != 1 {
		t.Fatalf("provider called %d times, want 1 (second read from cache)", n)
	}

	down := failing("p", domain.ErrNetwork)
	s.Configure([]domain.Provider{down}, nil, time.Minute)
	q, err := s.Get(ctx, "AAPL", true)
	if err != nil || !q.Stale || q.Price != 10 {
		t.Fatalf("want stale cached quote, got %+v, %v", q, err)
	}
}

func TestGet_InvalidTicker(t *testing.T) {
	s := newService(t, ok("p", 1))
	if _, err := s.Get(context.Background(), "bad ticker", false); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v", err)
	}
}

func TestHistory_CachedUntilForced(t *testing.T) {
	p := ok("p", 5)
	s := newService(t, p)
	ctx := context.Background()
	for range 3 {
		if _, err := s.History(ctx, "AAPL", 22, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.History(ctx, "AAPL", 22, true); err != nil {
		t.Fatal(err)
	}
	if n := p.calls.Load(); n != 2 {
		t.Fatalf("provider called %d times, want 2", n)
	}
}
