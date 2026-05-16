package twelvedata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"fin-cli/internal/domain"
	"fin-cli/internal/httpx"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New(httpx.New(), "test-key")
	c.base = srv.URL
	return c
}

func TestQuote_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"symbol": "AAPL",
			"name": "Apple Inc",
			"exchange": "NASDAQ",
			"currency": "USD",
			"open": "150.00",
			"high": "155.00",
			"low": "149.50",
			"close": "153.25",
			"previous_close": "150.50",
			"change": "2.75",
			"percent_change": "1.83",
			"volume": "85000000",
			"fifty_two_week_high": "199.62",
			"fifty_two_week_low": "124.17",
			"timestamp": 1700000000
		}`))
	})

	q, err := c.Quote(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("Quote() error: %v", err)
	}
	if q.Symbol != "AAPL" {
		t.Errorf("Symbol = %q, want AAPL", q.Symbol)
	}
	if q.Price != 153.25 {
		t.Errorf("Price = %f, want 153.25", q.Price)
	}
	if q.PrevClose != 150.50 {
		t.Errorf("PrevClose = %f, want 150.50", q.PrevClose)
	}
	if q.Change != 2.75 {
		t.Errorf("Change = %f, want 2.75", q.Change)
	}
	if q.ChangePct != 1.83 {
		t.Errorf("ChangePct = %f, want 1.83", q.ChangePct)
	}
	if q.Name != "Apple Inc" {
		t.Errorf("Name = %q, want Apple Inc", q.Name)
	}
	if q.Source != domain.SourceTwelveData {
		t.Errorf("Source = %q, want twelvedata", q.Source)
	}
	if !q.Open.Valid || q.Open.Value != 150.0 {
		t.Errorf("Open = %+v, want Some(150.0)", q.Open)
	}
	if !q.Volume.Valid || q.Volume.Value != 85000000 {
		t.Errorf("Volume = %+v, want Some(85000000)", q.Volume)
	}
	if !q.Week52High.Valid || q.Week52High.Value != 199.62 {
		t.Errorf("Week52High = %+v, want Some(199.62)", q.Week52High)
	}
}

func TestQuote_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "error",
			"code": 400,
			"message": "**symbol** not found: ZZZZ. Please specify it correctly according to API documentation."
		}`))
	})

	_, err := c.Quote(context.Background(), "ZZZZ")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestQuote_RateLimited(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := c.Quote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrRateLimited) {
		t.Errorf("error = %v, want ErrRateLimited", err)
	}
}

func TestQuote_Unauthorized(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := c.Quote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

func TestQuote_ServerError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := c.Quote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("error = %v, want ErrUnavailable", err)
	}
}

func TestQuote_NoAPIKey(t *testing.T) {
	c := New(httpx.New(), "")
	_, err := c.Quote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNoAPIKey) {
		t.Errorf("error = %v, want ErrNoAPIKey", err)
	}
}

func TestHistory_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"meta": {"symbol": "AAPL", "interval": "1day", "currency": "USD", "exchange": "NASDAQ"},
			"values": [
				{"datetime": "2024-01-05", "open": "150", "high": "152", "low": "149", "close": "151", "volume": "70000000"},
				{"datetime": "2024-01-04", "open": "148", "high": "151", "low": "147", "close": "150", "volume": "65000000"},
				{"datetime": "2024-01-03", "open": "145", "high": "149", "low": "144", "close": "148", "volume": "60000000"}
			]
		}`))
	})

	candles, err := c.History(context.Background(), "AAPL", domain.DefaultRange)
	if err != nil {
		t.Fatalf("History() error: %v", err)
	}
	// Twelve Data returns newest first; we reverse to chronological.
	if len(candles) != 3 {
		t.Fatalf("got %d candles, want 3", len(candles))
	}
	// First candle should be oldest (Jan 3).
	if candles[0].Close != 148 {
		t.Errorf("candles[0].Close = %f, want 148", candles[0].Close)
	}
	// Last candle should be newest (Jan 5).
	if candles[2].Close != 151 {
		t.Errorf("candles[2].Close = %f, want 151", candles[2].Close)
	}
}

func TestHistory_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "error",
			"code": 400,
			"message": "**symbol** not found"
		}`))
	})

	_, err := c.History(context.Background(), "ZZZZ", domain.DefaultRange)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestHistory_NoAPIKey(t *testing.T) {
	c := New(httpx.New(), "")
	_, err := c.History(context.Background(), "AAPL", domain.DefaultRange)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNoAPIKey) {
		t.Errorf("error = %v, want ErrNoAPIKey", err)
	}
}
