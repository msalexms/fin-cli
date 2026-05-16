// Package yahoo implements domain.MarketProvider against the public
// (unofficial) Yahoo Finance chart endpoint at
//
//	https://query1.finance.yahoo.com/v8/finance/chart/{symbol}
//
// No API key or authentication is required. The endpoint is not officially
// documented by Yahoo; it may break at any time. We treat it as a best-effort
// source for free historical candles and basic quotes (global coverage).
package yahoo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fin-cli/internal/domain"
	"fin-cli/internal/httpx"
)

const baseURL = "https://query1.finance.yahoo.com/v8/finance/chart"

// Browser-like User-Agent: Yahoo's endpoint has been observed to reject
// generic/library UAs with 429/403.
const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"

// Client implements domain.MarketProvider (Quote + History).
type Client struct {
	http *httpx.Client
	base string
}

// New returns a Client.
func New(h *httpx.Client) *Client { return &Client{http: h, base: baseURL} }

// Name satisfies domain.MarketProvider.
func (c *Client) Name() string { return "yahoo" }

type response struct {
	Chart struct {
		Result []result `json:"result"`
		Error  *apiErr  `json:"error"`
	} `json:"chart"`
}

type apiErr struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type result struct {
	Meta       meta        `json:"meta"`
	Timestamp  []int64     `json:"timestamp"`
	Indicators indicatorsT `json:"indicators"`
}

type meta struct {
	Currency             string  `json:"currency"`
	Symbol               string  `json:"symbol"`
	Exchange             string  `json:"exchangeName"`
	FullExchangeName     string  `json:"fullExchangeName"`
	InstrumentType       string  `json:"instrumentType"`
	RegularMarketPrice   float64 `json:"regularMarketPrice"`
	ChartPreviousClose   float64 `json:"chartPreviousClose"`
	PreviousClose        float64 `json:"previousClose"`
	RegularMarketDayHigh float64 `json:"regularMarketDayHigh"`
	RegularMarketDayLow  float64 `json:"regularMarketDayLow"`
	RegularMarketVolume  int64   `json:"regularMarketVolume"`
	FiftyTwoWeekHigh     float64 `json:"fiftyTwoWeekHigh"`
	FiftyTwoWeekLow      float64 `json:"fiftyTwoWeekLow"`
	RegularMarketTime    int64   `json:"regularMarketTime"`
	Timezone             string  `json:"timezone"`
}

type indicatorsT struct {
	Quote    []quoteSeries    `json:"quote"`
	AdjClose []adjCloseSeries `json:"adjclose"`
}

type quoteSeries struct {
	Open   []float64 `json:"open"`
	High   []float64 `json:"high"`
	Low    []float64 `json:"low"`
	Close  []float64 `json:"close"`
	Volume []int64   `json:"volume"`
}

type adjCloseSeries struct {
	AdjClose []float64 `json:"adjclose"`
}

// Quote fetches a basic quote snapshot via the chart endpoint's meta field.
// Yahoo provides global coverage but no fundamentals (P/E, EPS, Beta, etc.).
// The returned Quote always has Partial=true.
func (c *Client) Quote(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	r0, err := c.fetchChart(ctx, sym, "5d", "1d")
	if err != nil {
		return domain.Quote{}, err
	}

	m := r0.Meta
	if m.RegularMarketPrice == 0 && m.PreviousClose == 0 {
		return domain.Quote{}, fmt.Errorf("%w: %s (all zeros)", domain.ErrNotFound, sym)
	}

	prevClose := m.PreviousClose
	if prevClose == 0 {
		prevClose = m.ChartPreviousClose
	}
	price := m.RegularMarketPrice
	change := price - prevClose
	var changePct float64
	if prevClose != 0 {
		changePct = (change / prevClose) * 100
	}

	q := domain.Quote{
		Symbol:    sym,
		Currency:  m.Currency,
		Exchange:  m.FullExchangeName,
		Price:     price,
		PrevClose: prevClose,
		Change:    change,
		ChangePct: changePct,
		Source:    domain.SourceYahoo,
		FetchedAt: time.Now().UTC(),
		Partial:   true, // no fundamentals from Yahoo
	}

	// Day range from meta.
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

	// Try to derive Open from the first candle of the latest day.
	if len(r0.Indicators.Quote) > 0 {
		qs := r0.Indicators.Quote[0]
		if n := len(qs.Open); n > 0 && qs.Open[n-1] > 0 {
			q.Open = domain.Some(qs.Open[n-1])
		}
	}

	// Set AsOf from the regular market time.
	if m.RegularMarketTime > 0 {
		q.AsOf = time.Unix(m.RegularMarketTime, 0).UTC()
	}

	return q, nil
}

// History fetches ~r.Sessions daily candles.
// Yahoo's range values are pre-set strings; we pick the smallest range that
// covers the requested number of sessions and then trim.
func (c *Client) History(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error) {
	sessions := r.Sessions
	if sessions <= 0 {
		sessions = 22
	}
	rng := selectRange(sessions)

	r0, err := c.fetchChart(ctx, sym, rng, "1d")
	if err != nil {
		return nil, err
	}

	if len(r0.Timestamp) == 0 || len(r0.Indicators.Quote) == 0 {
		return nil, fmt.Errorf("%w: no candles", domain.ErrPartialData)
	}

	qs := r0.Indicators.Quote[0]
	n := len(r0.Timestamp)
	candles := make([]domain.Candle, 0, n)
	for i := 0; i < n; i++ {
		// Yahoo returns nulls for missing values as 0 after unmarshal; skip
		// rows with zero close to avoid polluting the chart.
		if at(qs.Close, i) == 0 {
			continue
		}
		candles = append(candles, domain.Candle{
			Time:   time.Unix(r0.Timestamp[i], 0).UTC(),
			Open:   at(qs.Open, i),
			High:   at(qs.High, i),
			Low:    at(qs.Low, i),
			Close:  qs.Close[i],
			Volume: atI(qs.Volume, i),
		})
	}
	if len(candles) == 0 {
		return nil, fmt.Errorf("%w: empty candles", domain.ErrPartialData)
	}
	if len(candles) > sessions {
		candles = candles[len(candles)-sessions:]
	}
	return candles, nil
}

// fetchChart is the shared HTTP request logic for the Yahoo chart endpoint.
func (c *Client) fetchChart(ctx context.Context, sym domain.Ticker, rng, interval string) (*result, error) {
	u := fmt.Sprintf("%s/%s?%s", c.base, url.PathEscape(strings.ToUpper(string(sym))),
		url.Values{
			"interval":       {interval},
			"range":          {rng},
			"includePrePost": {"false"},
			"events":         {"div,splits"},
		}.Encode(),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrNetwork, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("%w: yahoo", domain.ErrRateLimited)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", domain.ErrNotFound, sym)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%w: yahoo http %d", domain.ErrUnavailable, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("%w: yahoo http %d: %s", domain.ErrNetwork, resp.StatusCode, string(body))
	}

	var out response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode yahoo: %w", err)
	}
	if out.Chart.Error != nil {
		if isNotFound(out.Chart.Error) {
			return nil, fmt.Errorf("%w: %s", domain.ErrNotFound, sym)
		}
		return nil, errors.New("yahoo: " + out.Chart.Error.Description)
	}
	if len(out.Chart.Result) == 0 {
		return nil, fmt.Errorf("%w: empty result", domain.ErrPartialData)
	}

	return &out.Chart.Result[0], nil
}

// selectRange picks the smallest Yahoo range string covering n trading sessions.
func selectRange(n int) string {
	switch {
	case n <= 5:
		return "5d"
	case n <= 22:
		return "1mo"
	case n <= 66:
		return "3mo"
	case n <= 130:
		return "6mo"
	case n <= 260:
		return "1y"
	default:
		return "2y"
	}
}

func isNotFound(e *apiErr) bool {
	if e == nil {
		return false
	}
	if strings.EqualFold(e.Code, "Not Found") {
		return true
	}
	return strings.Contains(strings.ToLower(e.Description), "no data")
}

func at(s []float64, i int) float64 {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

func atI(s []int64, i int) int64 {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}
