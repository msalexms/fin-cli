// Package yahoo implements domain.Provider against the public (unofficial)
// Yahoo Finance chart endpoint:
//
//	https://query1.finance.yahoo.com/v8/finance/chart/{symbol}
//
// It needs no API key and covers global markets, crypto and FX, which makes
// it the default fallback. It is undocumented and may change without notice.
package yahoo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/httpx"
)

const (
	name    = "yahoo"
	baseURL = "https://query1.finance.yahoo.com/v8/finance/chart"
	// Yahoo rejects library User-Agents with 429/403.
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

// Client implements domain.Provider.
type Client struct {
	http *httpx.Client
	base string
	now  func() time.Time
}

// New returns a Client.
func New(h *httpx.Client) *Client { return &Client{http: h, base: baseURL, now: time.Now} }

// Name implements domain.Provider.
func (c *Client) Name() string { return name }

type response struct {
	Chart struct {
		Result []result `json:"result"`
		Error  *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

type result struct {
	Meta       meta    `json:"meta"`
	Timestamp  []int64 `json:"timestamp"`
	Indicators struct {
		Quote []struct {
			Open   []*float64 `json:"open"`
			High   []*float64 `json:"high"`
			Low    []*float64 `json:"low"`
			Close  []*float64 `json:"close"`
			Volume []*int64   `json:"volume"`
		} `json:"quote"`
	} `json:"indicators"`
}

type period struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type meta struct {
	Currency             string   `json:"currency"`
	FullExchangeName     string   `json:"fullExchangeName"`
	LongName             string   `json:"longName"`
	ShortName            string   `json:"shortName"`
	RegularMarketPrice   float64  `json:"regularMarketPrice"`
	RegularMarketTime    int64    `json:"regularMarketTime"`
	RegularMarketDayHigh float64  `json:"regularMarketDayHigh"`
	RegularMarketDayLow  float64  `json:"regularMarketDayLow"`
	RegularMarketVolume  int64    `json:"regularMarketVolume"`
	RegularChangePct     *float64 `json:"regularMarketChangePercent"`
	FulldayChange        *float64 `json:"fulldayChange"`
	PreviousClose        float64  `json:"previousClose"`
	FiftyTwoWeekHigh     float64  `json:"fiftyTwoWeekHigh"`
	FiftyTwoWeekLow      float64  `json:"fiftyTwoWeekLow"`
	CurrentTradingPeriod struct {
		Pre     period `json:"pre"`
		Regular period `json:"regular"`
		Post    period `json:"post"`
	} `json:"currentTradingPeriod"`
}

// Quote implements domain.Provider. Yahoo's chart endpoint carries no
// fundamentals, so quotes are always Partial.
func (c *Client) Quote(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	r, err := c.fetch(ctx, sym, "5d")
	if err != nil {
		return domain.Quote{}, err
	}
	m := r.Meta
	if m.RegularMarketPrice == 0 {
		return domain.Quote{}, fmt.Errorf("%s: %w: %s", name, domain.ErrNotFound, sym)
	}

	price := m.RegularMarketPrice
	prev := previousClose(r)
	change := price - prev
	if m.FulldayChange != nil {
		change = *m.FulldayChange
		prev = price - change
	}
	prev, change = round6(prev), round6(change)
	var pct float64
	switch {
	case m.RegularChangePct != nil:
		pct = *m.RegularChangePct
	case prev != 0:
		pct = change / prev * 100
	}

	now := c.now()
	q := domain.Quote{
		Symbol:    sym,
		Name:      firstNonEmpty(m.LongName, m.ShortName),
		Currency:  m.Currency,
		Exchange:  m.FullExchangeName,
		Price:     price,
		PrevClose: prev,
		Change:    change,
		ChangePct: pct,
		Session:   session(m, now),
		AsOf:      time.Unix(m.RegularMarketTime, 0).UTC(),
		FetchedAt: now.UTC(),
		Source:    name,
		Partial:   true,
	}
	if m.RegularMarketTime == 0 {
		q.AsOf = q.FetchedAt
	}
	if m.RegularMarketDayHigh > 0 {
		q.DayHigh = domain.Some(m.RegularMarketDayHigh)
	}
	if m.RegularMarketDayLow > 0 {
		q.DayLow = domain.Some(m.RegularMarketDayLow)
	}
	if m.RegularMarketVolume > 0 {
		q.Volume = domain.Some(m.RegularMarketVolume)
	}
	if m.FiftyTwoWeekHigh > 0 {
		q.Week52High = domain.Some(m.FiftyTwoWeekHigh)
	}
	if m.FiftyTwoWeekLow > 0 {
		q.Week52Low = domain.Some(m.FiftyTwoWeekLow)
	}
	if bars := candles(r); len(bars) > 0 && sameDay(bars[len(bars)-1].Time, q.AsOf) {
		q.Open = domain.Some(bars[len(bars)-1].Open)
	}
	return q, nil
}

// History implements domain.Provider.
func (c *Client) History(ctx context.Context, sym domain.Ticker, sessions int) ([]domain.Candle, error) {
	r, err := c.fetch(ctx, sym, rangeFor(sessions))
	if err != nil {
		return nil, err
	}
	out := candles(r)
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: %w: empty series", name, domain.ErrNoData)
	}
	if len(out) > sessions {
		out = out[len(out)-sessions:]
	}
	return out, nil
}

func (c *Client) fetch(ctx context.Context, sym domain.Ticker, rng string) (*result, error) {
	q := url.Values{"interval": {"1d"}, "range": {rng}, "includePrePost": {"false"}}
	u := c.base + "/" + url.PathEscape(string(sym)) + "?" + q.Encode()
	body, err := c.http.GetJSON(ctx, name, u, http.Header{"User-Agent": {userAgent}}, 2<<20)
	if err != nil {
		return nil, err
	}
	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s: decode: %w", name, err)
	}
	if e := resp.Chart.Error; e != nil {
		if strings.EqualFold(e.Code, "Not Found") || strings.Contains(strings.ToLower(e.Description), "no data") {
			return nil, fmt.Errorf("%s: %w: %s", name, domain.ErrNotFound, sym)
		}
		return nil, fmt.Errorf("%s: %s", name, e.Description)
	}
	if len(resp.Chart.Result) == 0 {
		return nil, fmt.Errorf("%s: %w: %s", name, domain.ErrNotFound, sym)
	}
	return &resp.Chart.Result[0], nil
}

// candles converts the indicator arrays, skipping bars with a null close.
func candles(r *result) []domain.Candle {
	if len(r.Indicators.Quote) == 0 {
		return nil
	}
	ind := r.Indicators.Quote[0]
	out := make([]domain.Candle, 0, len(r.Timestamp))
	for i, ts := range r.Timestamp {
		cl := at(ind.Close, i)
		if cl == 0 {
			continue
		}
		var vol int64
		if i < len(ind.Volume) && ind.Volume[i] != nil {
			vol = *ind.Volume[i]
		}
		out = append(out, domain.Candle{
			Time:   time.Unix(ts, 0).UTC(),
			Open:   at(ind.Open, i),
			High:   at(ind.High, i),
			Low:    at(ind.Low, i),
			Close:  cl,
			Volume: vol,
		})
	}
	return out
}

// previousClose finds the close of the session before the latest quote.
// meta.chartPreviousClose is not used: it is the close before the whole
// requested range, not before today.
func previousClose(r *result) float64 {
	if r.Meta.PreviousClose > 0 {
		return r.Meta.PreviousClose
	}
	bars := candles(r)
	if len(bars) == 0 {
		return 0
	}
	last := bars[len(bars)-1]
	if sameDay(last.Time, time.Unix(r.Meta.RegularMarketTime, 0)) {
		if len(bars) < 2 {
			return 0
		}
		return bars[len(bars)-2].Close
	}
	return last.Close
}

func session(m meta, now time.Time) domain.MarketSession {
	p := m.CurrentTradingPeriod
	if p.Regular.Start == 0 {
		return domain.SessionUnknown
	}
	in := func(x period) bool { return x.Start < x.End && now.Unix() >= x.Start && now.Unix() < x.End }
	switch {
	case in(p.Regular):
		return domain.SessionRegular
	case in(p.Pre):
		return domain.SessionPre
	case in(p.Post):
		return domain.SessionPost
	default:
		return domain.SessionClosed
	}
}

func rangeFor(sessions int) string {
	switch {
	case sessions <= 5:
		return "5d"
	case sessions <= 21:
		return "1mo"
	case sessions <= 63:
		return "3mo"
	case sessions <= 126:
		return "6mo"
	case sessions <= 252:
		return "1y"
	default:
		return "2y"
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

func at(s []*float64, i int) float64 {
	if i < 0 || i >= len(s) || s[i] == nil {
		return 0
	}
	return *s[i]
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// round6 removes float noise from derived prices (354.07000000000005).
func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }
