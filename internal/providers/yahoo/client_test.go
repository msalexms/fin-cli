package yahoo

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/httpx"
)

// Five daily bars; the last one is "today" (regularMarketTime 2026-11-24).
// chartPreviousClose (352.45) is the close *before the 5-day range* and must
// not be used as the previous close.
const fixture = `{"chart":{"result":[{"meta":{"currency":"USD","symbol":"AAPL",
"fullExchangeName":"NasdaqGS","regularMarketTime":1795620156,"regularMarketPrice":347.40,
"regularMarketDayHigh":353.84,"regularMarketDayLow":346.76,"regularMarketVolume":9620973,
"longName":"Apple Inc.","chartPreviousClose":352.45,"fiftyTwoWeekHigh":360.83,"fiftyTwoWeekLow":243.42,
"currentTradingPeriod":{"pre":{"start":1795597200,"end":1795617000},"regular":{"start":1795617000,"end":1795640400},"post":{"start":1795640400,"end":1795654800}}},
"timestamp":[1795098600,1795185000,1795444200,1795530600,1795617000],
"indicators":{"quote":[{"open":[352.9,352.8,347.8,348.0,353.58],"high":[355,354,352,354,353.84],
"low":[346,340,345,345,346.76],"close":[350.1,349.2,null,354.07,347.40],"volume":[1,2,null,4,9620973]}]}}],"error":null}}`

func newTestClient(t *testing.T, status int, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != userAgent {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := New(httpx.New())
	c.base = srv.URL
	c.now = func() time.Time { return time.Unix(1795620156, 0) } // during the regular session
	return c
}

func TestQuote_DailyChangeUsesPreviousSession(t *testing.T) {
	q, err := newTestClient(t, 200, fixture).Quote(context.Background(), "AAPL")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(q.PrevClose-354.07) > 1e-9 {
		t.Errorf("PrevClose = %v, want 354.07 (previous session)", q.PrevClose)
	}
	if math.Abs(q.Change-(347.40-354.07)) > 1e-9 {
		t.Errorf("Change = %v", q.Change)
	}
	if q.Name != "Apple Inc." || q.Session != domain.SessionRegular || !q.Partial {
		t.Errorf("unexpected quote: %+v", q)
	}
	if !q.Open.Valid || q.Open.Value != 353.58 {
		t.Errorf("Open = %+v, want today's open", q.Open)
	}
}

func TestHistory_SkipsNullBars(t *testing.T) {
	bars, err := newTestClient(t, 200, fixture).History(context.Background(), "AAPL", 22)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 4 {
		t.Fatalf("len = %d, want 4 (null close skipped)", len(bars))
	}
	if !bars[0].Time.Before(bars[3].Time) {
		t.Error("bars must be oldest first")
	}
}

func TestQuote_NotFound(t *testing.T) {
	body := `{"chart":{"result":null,"error":{"code":"Not Found","description":"No data found, symbol may be delisted"}}}`
	_, err := newTestClient(t, 404, body).Quote(context.Background(), "NOPE")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestQuote_RateLimited(t *testing.T) {
	_, err := newTestClient(t, 429, "Too Many Requests").Quote(context.Background(), "AAPL")
	if !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}
