package finnhub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/httpx"
)

func newTestClient(t *testing.T, key string, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(httpx.New(), key)
	c.base = srv.URL
	return c
}

func TestQuote_Success(t *testing.T) {
	c := newTestClient(t, "k", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Finnhub-Token") != "k" {
			t.Errorf("missing token header")
		}
		switch r.URL.Path {
		case "/quote":
			_, _ = w.Write([]byte(`{"c":190.5,"d":1.5,"dp":0.79,"h":191,"l":188,"o":189,"pc":189,"t":1700000000}`))
		case "/stock/profile2":
			_, _ = w.Write([]byte(`{"name":"Apple Inc","currency":"USD","exchange":"NASDAQ NMS - GLOBAL MARKET","country":"US","marketCapitalization":2900000}`))
		case "/stock/metric":
			_, _ = w.Write([]byte(`{"metric":{"52WeekHigh":200,"52WeekLow":150,"peTTM":30.1,"beta":1.2,"10DayAverageTradingVolume":55.5}}`))
		}
	})
	q, err := c.Quote(context.Background(), "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if q.Price != 190.5 || q.Name != "Apple Inc" || q.Partial {
		t.Errorf("unexpected quote: %+v", q)
	}
	if !q.Week52High.Valid || q.Week52High.Value != 200 || !q.PE.Valid {
		t.Errorf("metrics not mapped: %+v", q)
	}
	if q.Volume.Valid || !q.AvgVolume.Valid || q.AvgVolume.Value != 55_500_000 {
		t.Errorf("average volume must not be reported as volume: %+v / %+v", q.Volume, q.AvgVolume)
	}
	if q.AsOf.Unix() != 1700000000 {
		t.Errorf("AsOf = %v", q.AsOf)
	}
}

func TestQuote_UnknownSymbol(t *testing.T) {
	c := newTestClient(t, "k", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"c":0,"d":null,"dp":null,"h":0,"l":0,"o":0,"pc":0,"t":0}`))
	})
	if _, err := c.Quote(context.Background(), "NOPE"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestQuote_PartialWhenProfileFails(t *testing.T) {
	c := newTestClient(t, "k", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/quote" {
			_, _ = w.Write([]byte(`{"c":10,"pc":9,"d":1,"dp":11.1}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	})
	q, err := c.Quote(context.Background(), "X")
	if err != nil {
		t.Fatal(err)
	}
	if !q.Partial {
		t.Error("expected Partial=true")
	}
}

func TestQuote_BadKey(t *testing.T) {
	c := newTestClient(t, "bad", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if _, err := c.Quote(context.Background(), "AAPL"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestQuote_NoKey(t *testing.T) {
	c := New(httpx.New(), "")
	if _, err := c.Quote(context.Background(), "AAPL"); !errors.Is(err, domain.ErrNoAPIKey) {
		t.Fatalf("err = %v, want ErrNoAPIKey", err)
	}
}
