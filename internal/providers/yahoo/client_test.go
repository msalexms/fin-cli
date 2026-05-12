package yahoo

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"fin-cli/internal/domain"
	"fin-cli/internal/httpx"
)

// TestHistory_AAPL is an integration test that hits the real Yahoo endpoint.
// It is skipped when there is no DNS/network access (e.g. hermetic CI).
func TestHistory_AAPL(t *testing.T) {
	if _, err := net.LookupHost("query1.finance.yahoo.com"); err != nil {
		t.Skipf("no network: %v", err)
	}

	c := New(httpx.New())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	candles, err := c.History(ctx, "AAPL", domain.Range{Sessions: 22, Resolution: domain.ResolutionDaily})
	if err != nil {
		// Treat ErrNetwork as skip (transient external failure).
		if errors.Is(err, domain.ErrNetwork) || errors.Is(err, domain.ErrUnavailable) {
			t.Skipf("yahoo unavailable: %v", err)
		}
		t.Fatalf("History error: %v", err)
	}
	if len(candles) == 0 {
		t.Fatal("expected candles, got 0")
	}
	if len(candles) < 10 {
		t.Fatalf("expected ~22 candles, got %d", len(candles))
	}
	last := candles[len(candles)-1]
	if last.Close <= 0 {
		t.Fatalf("last candle close should be > 0: %+v", last)
	}
	t.Logf("got %d candles, last close = %.2f on %s",
		len(candles), last.Close, last.Time.Format("2006-01-02"))
}
