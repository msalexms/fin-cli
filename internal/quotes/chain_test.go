package quotes

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"fin-cli/internal/domain"
)

// mockProvider is a configurable mock for testing chains.
type mockProvider struct {
	name       string
	quoteFunc  func(ctx context.Context, sym domain.Ticker) (domain.Quote, error)
	histFunc   func(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error)
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Quote(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	if m.quoteFunc != nil {
		return m.quoteFunc(ctx, sym)
	}
	return domain.Quote{}, fmt.Errorf("not implemented")
}

func (m *mockProvider) History(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error) {
	if m.histFunc != nil {
		return m.histFunc(ctx, sym, r)
	}
	return nil, fmt.Errorf("not implemented")
}

func TestProviderChain_Quote_FirstSucceeds(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		quoteFunc: func(_ context.Context, sym domain.Ticker) (domain.Quote, error) {
			return domain.Quote{Symbol: sym, Price: 100, Source: "first"}, nil
		},
	}
	p2 := &mockProvider{
		name: "second",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			t.Error("second provider should not be called")
			return domain.Quote{}, nil
		},
	}

	chain := NewProviderChain(p1, p2)
	q, err := chain.Quote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Price != 100 {
		t.Errorf("Price = %f, want 100", q.Price)
	}
}

func TestProviderChain_Quote_FallbackOnTransient(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			return domain.Quote{}, fmt.Errorf("%w: first timeout", domain.ErrNetwork)
		},
	}
	p2 := &mockProvider{
		name: "second",
		quoteFunc: func(_ context.Context, sym domain.Ticker) (domain.Quote, error) {
			return domain.Quote{Symbol: sym, Price: 200, Source: "second"}, nil
		},
	}

	chain := NewProviderChain(p1, p2)
	q, err := chain.Quote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Price != 200 {
		t.Errorf("Price = %f, want 200 (from second provider)", q.Price)
	}
}

func TestProviderChain_Quote_SkipNoAPIKey(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			return domain.Quote{}, fmt.Errorf("%w: first", domain.ErrNoAPIKey)
		},
	}
	p2 := &mockProvider{
		name: "second",
		quoteFunc: func(_ context.Context, sym domain.Ticker) (domain.Quote, error) {
			return domain.Quote{Symbol: sym, Price: 300, Source: "second"}, nil
		},
	}

	chain := NewProviderChain(p1, p2)
	q, err := chain.Quote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Price != 300 {
		t.Errorf("Price = %f, want 300", q.Price)
	}
}

func TestProviderChain_Quote_StopsOnNotFound(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			return domain.Quote{}, fmt.Errorf("%w: ZZZZ", domain.ErrNotFound)
		},
	}
	p2 := &mockProvider{
		name: "second",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			t.Error("second provider should not be called on terminal ErrNotFound")
			return domain.Quote{}, nil
		},
	}

	chain := NewProviderChain(p1, p2)
	_, err := chain.Quote(context.Background(), "ZZZZ")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestProviderChain_Quote_StopsOnUnauthorized(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			return domain.Quote{}, fmt.Errorf("%w: bad key", domain.ErrUnauthorized)
		},
	}
	p2 := &mockProvider{
		name: "second",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			t.Error("second provider should not be called on terminal ErrUnauthorized")
			return domain.Quote{}, nil
		},
	}

	chain := NewProviderChain(p1, p2)
	_, err := chain.Quote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

func TestProviderChain_Quote_AllFail(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			return domain.Quote{}, fmt.Errorf("%w: first", domain.ErrRateLimited)
		},
	}
	p2 := &mockProvider{
		name: "second",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			return domain.Quote{}, fmt.Errorf("%w: second", domain.ErrUnavailable)
		},
	}

	chain := NewProviderChain(p1, p2)
	_, err := chain.Quote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	// Last meaningful error should be from the second provider.
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("error = %v, want ErrUnavailable (last error)", err)
	}
}

func TestProviderChain_Quote_Empty(t *testing.T) {
	chain := NewProviderChain()
	_, err := chain.Quote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNoAPIKey) {
		t.Errorf("error = %v, want ErrNoAPIKey", err)
	}
}

func TestProviderChain_Quote_AllSkippedNoKey(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			return domain.Quote{}, fmt.Errorf("%w: first", domain.ErrNoAPIKey)
		},
	}
	p2 := &mockProvider{
		name: "second",
		quoteFunc: func(_ context.Context, _ domain.Ticker) (domain.Quote, error) {
			return domain.Quote{}, fmt.Errorf("%w: second", domain.ErrNoAPIKey)
		},
	}

	chain := NewProviderChain(p1, p2)
	_, err := chain.Quote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNoAPIKey) {
		t.Errorf("error = %v, want ErrNoAPIKey", err)
	}
}

func TestProviderChain_Name(t *testing.T) {
	chain := NewProviderChain(
		&mockProvider{name: "a"},
		&mockProvider{name: "b"},
	)
	got := chain.Name()
	if got != "a,b" {
		t.Errorf("Name() = %q, want %q", got, "a,b")
	}

	single := NewProviderChain(&mockProvider{name: "only"})
	if single.Name() != "only" {
		t.Errorf("single Name() = %q, want %q", single.Name(), "only")
	}

	empty := NewProviderChain()
	if empty.Name() != "none" {
		t.Errorf("empty Name() = %q, want %q", empty.Name(), "none")
	}
}

// --- History chain tests ---

func TestProviderChain_History_FirstSucceeds(t *testing.T) {
	candle := domain.Candle{Close: 150}
	p1 := &mockProvider{
		name: "first",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			return []domain.Candle{candle}, nil
		},
	}
	chain := NewProviderChain(p1)
	candles, err := chain.History(context.Background(), "AAPL", domain.DefaultRange)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candles) != 1 || candles[0].Close != 150 {
		t.Errorf("candles = %v, want [{Close:150}]", candles)
	}
}

func TestProviderChain_History_UnauthorizedIsNonTerminal(t *testing.T) {
	// For history, ErrUnauthorized is non-terminal (Finnhub free tier 403).
	p1 := &mockProvider{
		name: "first",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			return nil, fmt.Errorf("%w: first", domain.ErrUnauthorized)
		},
	}
	p2 := &mockProvider{
		name: "second",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			return []domain.Candle{{Close: 200}}, nil
		},
	}
	chain := NewProviderChain(p1, p2)
	candles, err := chain.History(context.Background(), "AAPL", domain.DefaultRange)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candles) != 1 || candles[0].Close != 200 {
		t.Errorf("expected fallback to second provider")
	}
}

func TestProviderChain_History_StopsOnNotFound(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			return nil, fmt.Errorf("%w: ZZZZ", domain.ErrNotFound)
		},
	}
	p2 := &mockProvider{
		name: "second",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			t.Error("should not be called")
			return nil, nil
		},
	}
	chain := NewProviderChain(p1, p2)
	_, err := chain.History(context.Background(), "ZZZZ", domain.DefaultRange)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// --- HistoryChain tests ---

func TestHistoryChain_FallsBack(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			return nil, fmt.Errorf("%w: first", domain.ErrNetwork)
		},
	}
	p2 := &mockProvider{
		name: "second",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			return []domain.Candle{{Close: 300}}, nil
		},
	}

	chain := NewHistoryChain(p1, p2)
	candles, err := chain.History(context.Background(), "AAPL", domain.DefaultRange)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candles) != 1 || candles[0].Close != 300 {
		t.Errorf("expected fallback result")
	}
}

func TestHistoryChain_StopsOnNotFound(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			return nil, fmt.Errorf("%w: ZZZZ", domain.ErrNotFound)
		},
	}
	p2 := &mockProvider{
		name: "second",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			t.Error("should not be called")
			return nil, nil
		},
	}

	chain := NewHistoryChain(p1, p2)
	_, err := chain.History(context.Background(), "ZZZZ", domain.DefaultRange)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestHistoryChain_SkipsNoAPIKey(t *testing.T) {
	p1 := &mockProvider{
		name: "first",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			return nil, fmt.Errorf("%w: first", domain.ErrNoAPIKey)
		},
	}
	p2 := &mockProvider{
		name: "second",
		histFunc: func(_ context.Context, _ domain.Ticker, _ domain.Range) ([]domain.Candle, error) {
			return []domain.Candle{{Close: 400}}, nil
		},
	}

	chain := NewHistoryChain(p1, p2)
	candles, err := chain.History(context.Background(), "AAPL", domain.DefaultRange)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candles) != 1 || candles[0].Close != 400 {
		t.Errorf("expected fallback result")
	}
}
