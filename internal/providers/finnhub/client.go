// Package finnhub implements domain.MarketProvider using the official Finnhub
// Go SDK (github.com/Finnhub-Stock-API/finnhub-go/v2).
//
// The provider gathers four endpoints per quote to assemble a rich snapshot:
//   - Quote                    : price, day OHLC, change, timestamp
//   - CompanyProfile2          : name, exchange, industry, currency, marketCap, shares, IPO, web
//   - CompanyBasicFinancials   : 52-week range, P/E, EPS, beta, dividend yield (best-effort)
//   - StockCandles             : OHLCV history (premium on free tier; caller handles failure)
//
// Missing sub-responses set Partial=true but do not fail the whole call.
// Network/HTTP errors are classified to domain sentinels.
package finnhub

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	finnhubsdk "github.com/Finnhub-Stock-API/finnhub-go/v2"

	"fin-cli/internal/domain"
)

// Client is a domain.MarketProvider backed by the Finnhub SDK.
type Client struct {
	api    *finnhubsdk.DefaultApiService
	apiKey string
}

// New builds a Finnhub provider. An empty apiKey makes every call fail with
// domain.ErrNoAPIKey.
func New(apiKey string) *Client {
	cfg := finnhubsdk.NewConfiguration()
	cfg.AddDefaultHeader("X-Finnhub-Token", apiKey)
	cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	cfg.UserAgent = "fin-cli"
	return &Client{
		api:    finnhubsdk.NewAPIClient(cfg).DefaultApi,
		apiKey: apiKey,
	}
}

// Name satisfies domain.MarketProvider.
func (c *Client) Name() string { return "finnhub" }

// ---------------- Quote ----------------

// Quote assembles a rich snapshot from multiple Finnhub endpoints.
// The price call is mandatory; profile and metrics are best-effort.
func (c *Client) Quote(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	if c.apiKey == "" {
		return domain.Quote{}, fmt.Errorf("%w: finnhub", domain.ErrNoAPIKey)
	}
	symU := strings.ToUpper(strings.TrimSpace(string(sym)))

	q, resp, err := c.api.Quote(ctx).Symbol(symU).Execute()
	if err != nil {
		return domain.Quote{}, classify(resp, err)
	}

	// Finnhub returns all-zero Quote for unknown symbols (no 404).
	if f32(q.C) == 0 && f32(q.Pc) == 0 && f32(q.O) == 0 {
		return domain.Quote{}, fmt.Errorf("%w: %s", domain.ErrNotFound, symU)
	}

	now := time.Now().UTC()
	out := domain.Quote{
		Symbol:    domain.Ticker(symU),
		Price:     f64(q.C),
		PrevClose: f64(q.Pc),
		Change:    f64(q.D),
		ChangePct: f64(q.Dp),
		AsOf:      now,
		FetchedAt: now,
		Source:    domain.SourceFinnhub,
		Session:   inferSession(now),
	}
	if v := f32(q.O); v != 0 {
		out.Open = domain.Some(f64(q.O))
	}
	if v := f32(q.L); v != 0 {
		out.DayLow = domain.Some(f64(q.L))
	}
	if v := f32(q.H); v != 0 {
		out.DayHigh = domain.Some(f64(q.H))
	}

	// --- Profile (best-effort) ---
	prof, presp, perr := c.api.CompanyProfile2(ctx).Symbol(symU).Execute()
	if perr != nil {
		// Profile unavailable; continue with partial data.
		_ = presp
		out.Partial = true
	} else {
		out.Name = strVal(prof.Name)
		out.Currency = strVal(prof.Currency)
		out.Exchange = strVal(prof.Exchange)
		out.Country = strVal(prof.Country)
		out.Industry = strVal(prof.FinnhubIndustry)
		out.Weburl = strVal(prof.Weburl)
		out.IPODate = strVal(prof.Ipo)
		if prof.MarketCapitalization != nil && *prof.MarketCapitalization != 0 {
			out.MarketCap = domain.Some(f64(prof.MarketCapitalization))
		}
		if prof.ShareOutstanding != nil && *prof.ShareOutstanding != 0 {
			out.SharesOut = domain.Some(f64(prof.ShareOutstanding))
		}
	}

	// --- Basic financials (best-effort: Finnhub free includes most) ---
	fin, fresp, ferr := c.api.CompanyBasicFinancials(ctx).Symbol(symU).Metric("all").Execute()
	if ferr != nil {
		_ = fresp
		out.Partial = true
	} else if fin.Metric != nil {
		m := *fin.Metric
		if v, ok := metricFloat(m, "52WeekHigh"); ok {
			out.Week52High = domain.Some(v)
		}
		if v, ok := metricFloat(m, "52WeekLow"); ok {
			out.Week52Low = domain.Some(v)
		}
		if v, ok := metricFloat(m, "peTTM", "peNormalizedAnnual", "peBasicExclExtraTTM"); ok {
			out.PE = domain.Some(v)
		}
		if v, ok := metricFloat(m, "epsTTM", "epsAnnual", "epsBasicExclExtraItemsTTM"); ok {
			out.EPS = domain.Some(v)
		}
		if v, ok := metricFloat(m, "beta"); ok {
			out.Beta = domain.Some(v)
		}
		if v, ok := metricFloat(m, "dividendYieldIndicatedAnnual", "currentDividendYieldTTM"); ok {
			out.DivYield = domain.Some(v)
		}
		if v, ok := metricFloat(m, "10DayAverageTradingVolume"); ok {
			out.Volume = domain.Some(int64(v * 1_000_000))
		}
	}

	return out, nil
}

// ---------------- History ----------------

// History fetches daily candles for ~r.Sessions trading days.
// Note: on Finnhub free tier, /stock/candle returns 403 for most equities.
// The caller treats the error as best-effort.
func (c *Client) History(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("%w: finnhub", domain.ErrNoAPIKey)
	}
	if r.Resolution == "" {
		r.Resolution = domain.ResolutionDaily
	}
	sessions := r.Sessions
	if sessions <= 0 {
		sessions = 22
	}
	// Pad calendar days to cover weekends + holidays.
	calendarDays := sessions*7/5 + 10
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -calendarDays)

	cr, resp, err := c.api.StockCandles(ctx).
		Symbol(strings.ToUpper(string(sym))).
		Resolution(string(r.Resolution)).
		From(from.Unix()).
		To(to.Unix()).
		Execute()
	if err != nil {
		return nil, classify(resp, err)
	}
	if cr.S == nil || *cr.S != "ok" || cr.C == nil || len(*cr.C) == 0 {
		return nil, fmt.Errorf("%w: no candles", domain.ErrPartialData)
	}

	closes := *cr.C
	opens := deref(cr.O)
	highs := deref(cr.H)
	lows := deref(cr.L)
	vols := deref(cr.V)
	ts := derefInt64(cr.T)

	n := len(closes)
	out := make([]domain.Candle, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, domain.Candle{
			Time:   time.Unix(at(ts, i), 0).UTC(),
			Open:   f64v(at(opens, i)),
			High:   f64v(at(highs, i)),
			Low:    f64v(at(lows, i)),
			Close:  f64v(closes[i]),
			Volume: int64(f64v(at(vols, i))),
		})
	}
	if len(out) > sessions {
		out = out[len(out)-sessions:]
	}
	return out, nil
}

// ---------------- helpers ----------------

func f64(p *float32) float64 {
	if p == nil {
		return 0
	}
	return float64(*p)
}
func f32(p *float32) float32 {
	if p == nil {
		return 0
	}
	return *p
}
func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func f64v(v float32) float64 { return float64(v) }

func deref[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}

func derefInt64(p *[]int64) []int64 {
	if p == nil {
		return nil
	}
	return *p
}

func at[T any](s []T, i int) T {
	var zero T
	if i < 0 || i >= len(s) {
		return zero
	}
	return s[i]
}

// metricFloat looks up keys in order and returns the first numeric value.
// The SDK returns metrics as map[string]interface{}; numbers decode as float64.
func metricFloat(m map[string]interface{}, keys ...string) (float64, bool) {
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case float64:
			if x != 0 {
				return x, true
			}
		case float32:
			if x != 0 {
				return float64(x), true
			}
		case int:
			if x != 0 {
				return float64(x), true
			}
		case int64:
			if x != 0 {
				return float64(x), true
			}
		}
	}
	return 0, false
}

// inferSession approximates US market session from the quote timestamp.
// This is a heuristic; Finnhub free tier does not flag sessions.
func inferSession(t time.Time) domain.MarketSession {
	if t.IsZero() {
		return domain.SessionUnknown
	}
	// Convert to US Eastern. If the zoneinfo database is missing, fall back to UTC.
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return domain.SessionUnknown
	}
	local := t.In(loc)
	wday := local.Weekday()
	if wday == time.Saturday || wday == time.Sunday {
		return domain.SessionClosed
	}
	h := local.Hour()
	m := local.Minute()
	mins := h*60 + m
	const (
		preOpen = 4 * 60        // 04:00
		regular = 9*60 + 30     // 09:30
		close_  = 16 * 60       // 16:00
		postEnd = 20 * 60       // 20:00
	)
	switch {
	case mins >= preOpen && mins < regular:
		return domain.SessionPre
	case mins >= regular && mins < close_:
		return domain.SessionRegular
	case mins >= close_ && mins < postEnd:
		return domain.SessionPost
	default:
		return domain.SessionClosed
	}
}
