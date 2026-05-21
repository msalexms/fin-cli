package alphavantage

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
			"Global Quote": {
				"01. symbol": "IBM",
				"02. open": "185.50",
				"03. high": "187.00",
				"04. low": "184.00",
				"05. price": "186.25",
				"06. volume": "4200000",
				"07. latest trading day": "2024-01-05",
				"08. previous close": "185.00",
				"09. change": "1.25",
				"10. change percent": "0.6757%"
			}
		}`))
	})

	q, err := c.Quote(context.Background(), "IBM")
	if err != nil {
		t.Fatalf("Quote() error: %v", err)
	}
	if q.Symbol != "IBM" {
		t.Errorf("Symbol = %q, want IBM", q.Symbol)
	}
	if q.Price != 186.25 {
		t.Errorf("Price = %f, want 186.25", q.Price)
	}
	if q.PrevClose != 185.0 {
		t.Errorf("PrevClose = %f, want 185.0", q.PrevClose)
	}
	if q.Change != 1.25 {
		t.Errorf("Change = %f, want 1.25", q.Change)
	}
	if q.ChangePct != 0.6757 {
		t.Errorf("ChangePct = %f, want 0.6757", q.ChangePct)
	}
	if q.Source != domain.SourceAlphaVantage {
		t.Errorf("Source = %q, want alphavantage", q.Source)
	}
	if !q.Partial {
		t.Error("expected Partial=true (no fundamentals)")
	}
	if !q.Open.Valid || q.Open.Value != 185.5 {
		t.Errorf("Open = %+v, want Some(185.5)", q.Open)
	}
	if !q.Volume.Valid || q.Volume.Value != 4200000 {
		t.Errorf("Volume = %+v, want Some(4200000)", q.Volume)
	}
}

func TestQuote_NotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Alpha Vantage returns an empty Global Quote for unknown symbols.
		w.Write([]byte(`{"Global Quote": {}}`))
	})

	_, err := c.Quote(context.Background(), "ZZZZZZ")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestQuote_RateLimited_Note(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"Note": "Thank you for using Alpha Vantage! Our standard API call frequency is 25 calls per day."
		}`))
	})

	_, err := c.Quote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrRateLimited) {
		t.Errorf("error = %v, want ErrRateLimited", err)
	}
}

func TestQuote_Unauthorized_Information(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"Information": "The provided API key is invalid or this is a premium endpoint."
		}`))
	})

	_, err := c.Quote(context.Background(), "AAPL")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

func TestQuote_HTTPError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"429", http.StatusTooManyRequests, domain.ErrRateLimited},
		{"401", http.StatusUnauthorized, domain.ErrUnauthorized},
		{"403", http.StatusForbidden, domain.ErrUnauthorized},
		{"500", http.StatusInternalServerError, domain.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			})
			_, err := c.Quote(context.Background(), "AAPL")
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
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
			"Meta Data": {
				"2. Symbol": "IBM",
				"3. Last Refreshed": "2024-01-05"
			},
			"Time Series (Daily)": {
				"2024-01-05": {"1. open": "185.5", "2. high": "187.0", "3. low": "184.0", "4. close": "186.25", "5. volume": "4200000"},
				"2024-01-04": {"1. open": "183.0", "2. high": "185.5", "3. low": "182.5", "4. close": "185.0", "5. volume": "3800000"},
				"2024-01-03": {"1. open": "181.0", "2. high": "184.0", "3. low": "180.0", "4. close": "183.0", "5. volume": "3500000"}
			}
		}`))
	})

	candles, err := c.History(context.Background(), "IBM", domain.DefaultRange)
	if err != nil {
		t.Fatalf("History() error: %v", err)
	}
	if len(candles) != 3 {
		t.Fatalf("got %d candles, want 3", len(candles))
	}
	// Sorted chronologically.
	if candles[0].Close != 183.0 {
		t.Errorf("candles[0].Close = %f, want 183.0", candles[0].Close)
	}
	if candles[2].Close != 186.25 {
		t.Errorf("candles[2].Close = %f, want 186.25", candles[2].Close)
	}
}

func TestHistory_RateLimited(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Note": "rate limit exceeded"}`))
	})

	_, err := c.History(context.Background(), "IBM", domain.DefaultRange)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrRateLimited) {
		t.Errorf("error = %v, want ErrRateLimited", err)
	}
}

func TestHistory_NoAPIKey(t *testing.T) {
	c := New(httpx.New(), "")
	_, err := c.History(context.Background(), "IBM", domain.DefaultRange)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, domain.ErrNoAPIKey) {
		t.Errorf("error = %v, want ErrNoAPIKey", err)
	}
}
