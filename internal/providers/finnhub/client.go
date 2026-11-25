// Package finnhub implements domain.Provider against the Finnhub REST API
// (https://finnhub.io/docs/api).
//
// A quote combines three endpoints: /quote (price, mandatory), /stock/profile2
// (identity, market cap) and /stock/metric (52-week range and fundamentals).
// The last two are best effort: if they fail the quote is marked Partial.
// Daily candles are premium-only on Finnhub, so History always reports
// ErrNoData and the history chain moves on to the next provider.
package finnhub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/httpx"
)

const (
	name    = "finnhub"
	baseURL = "https://finnhub.io/api/v1"
)

// Client is a domain.Provider backed by Finnhub.
type Client struct {
	http   *httpx.Client
	apiKey string
	base   string
}

// New builds a Finnhub provider. With an empty apiKey every call fails with
// domain.ErrNoAPIKey, which the chain treats as "skip".
func New(h *httpx.Client, apiKey string) *Client {
	return &Client{http: h, apiKey: apiKey, base: baseURL}
}

// Name implements domain.Provider.
func (c *Client) Name() string { return name }

type quoteResp struct {
	C  float64 `json:"c"`
	D  float64 `json:"d"`
	DP float64 `json:"dp"`
	H  float64 `json:"h"`
	L  float64 `json:"l"`
	O  float64 `json:"o"`
	PC float64 `json:"pc"`
	T  int64   `json:"t"`
}

type profileResp struct {
	Country   string  `json:"country"`
	Currency  string  `json:"currency"`
	Exchange  string  `json:"exchange"`
	Industry  string  `json:"finnhubIndustry"`
	IPO       string  `json:"ipo"`
	MarketCap float64 `json:"marketCapitalization"`
	Name      string  `json:"name"`
}

type metricResp struct {
	Metric map[string]any `json:"metric"`
}

// Quote implements domain.Provider.
func (c *Client) Quote(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	if c.apiKey == "" {
		return domain.Quote{}, fmt.Errorf("%s: %w", name, domain.ErrNoAPIKey)
	}

	var q quoteResp
	if err := c.get(ctx, "/quote", url.Values{"symbol": {string(sym)}}, &q); err != nil {
		return domain.Quote{}, err
	}
	// Unknown symbols come back as an all-zero object instead of a 404.
	if q.C == 0 && q.PC == 0 {
		return domain.Quote{}, fmt.Errorf("%s: %w: %s", name, domain.ErrNotFound, sym)
	}

	now := time.Now().UTC()
	out := domain.Quote{
		Symbol:    sym,
		Price:     q.C,
		PrevClose: q.PC,
		Change:    q.D,
		ChangePct: q.DP,
		AsOf:      now,
		FetchedAt: now,
		Source:    name,
	}
	if q.T > 0 {
		out.AsOf = time.Unix(q.T, 0).UTC()
	}
	setPositive(&out.Open, q.O)
	setPositive(&out.DayLow, q.L)
	setPositive(&out.DayHigh, q.H)

	var prof profileResp
	if err := c.get(ctx, "/stock/profile2", url.Values{"symbol": {string(sym)}}, &prof); err != nil || prof.Name == "" {
		out.Partial = true
	} else {
		out.Name = prof.Name
		out.Currency = prof.Currency
		out.Exchange = prof.Exchange
		out.Country = prof.Country
		out.Industry = prof.Industry
		out.IPODate = prof.IPO
		setPositive(&out.MarketCap, prof.MarketCap)
	}

	var met metricResp
	if err := c.get(ctx, "/stock/metric", url.Values{"symbol": {string(sym)}, "metric": {"all"}}, &met); err != nil || met.Metric == nil {
		out.Partial = true
	} else {
		m := met.Metric
		setFrom(&out.Week52High, m, "52WeekHigh")
		setFrom(&out.Week52Low, m, "52WeekLow")
		setFrom(&out.PE, m, "peTTM", "peBasicExclExtraTTM", "peNormalizedAnnual")
		setFrom(&out.EPS, m, "epsTTM", "epsBasicExclExtraItemsTTM", "epsAnnual")
		setFrom(&out.Beta, m, "beta")
		setFrom(&out.DivYield, m, "currentDividendYieldTTM", "dividendYieldIndicatedAnnual")
		var avg domain.Optional[float64]
		setFrom(&avg, m, "10DayAverageTradingVolume") // reported in millions
		if avg.Valid {
			out.AvgVolume = domain.Some(int64(avg.Value * 1_000_000))
		}
	}

	if out.Country == "US" {
		out.Session = USSession(now)
	}
	return out, nil
}

// History implements domain.Provider. Finnhub candles require a paid plan.
func (c *Client) History(context.Context, domain.Ticker, int) ([]domain.Candle, error) {
	return nil, fmt.Errorf("%s: %w: candles need a paid plan", name, domain.ErrNoData)
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	h := http.Header{"X-Finnhub-Token": {c.apiKey}}
	body, err := c.http.GetJSON(ctx, name, c.base+path+"?"+q.Encode(), h, 1<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: decode %s: %w", name, path, err)
	}
	return nil
}

func setPositive(dst *domain.Optional[float64], v float64) {
	if v > 0 {
		*dst = domain.Some(v)
	}
}

// setFrom stores the first non-zero numeric metric among keys.
func setFrom(dst *domain.Optional[float64], m map[string]any, keys ...string) {
	for _, k := range keys {
		if v, ok := m[k].(float64); ok && v != 0 {
			*dst = domain.Some(v)
			return
		}
	}
}

// USSession approximates the US equity session for t (New York time).
// It ignores exchange holidays.
func USSession(t time.Time) domain.MarketSession {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return domain.SessionUnknown
	}
	ny := t.In(loc)
	if wd := ny.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return domain.SessionClosed
	}
	switch mins := ny.Hour()*60 + ny.Minute(); {
	case mins >= 4*60 && mins < 9*60+30:
		return domain.SessionPre
	case mins >= 9*60+30 && mins < 16*60:
		return domain.SessionRegular
	case mins >= 16*60 && mins < 20*60:
		return domain.SessionPost
	default:
		return domain.SessionClosed
	}
}
