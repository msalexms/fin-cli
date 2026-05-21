// Package twelvedata implements domain.MarketProvider against the Twelve Data
// REST API (https://twelvedata.com/docs).
//
// Free tier: 800 requests/day, 8 requests/minute.
// Requires an API key from https://twelvedata.com.
package twelvedata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fin-cli/internal/domain"
	"fin-cli/internal/httpx"
)

const baseURL = "https://api.twelvedata.com"

// Client implements domain.MarketProvider.
type Client struct {
	http   *httpx.Client
	apiKey string
	base   string
}

// New returns a Client. If apiKey is empty, all calls return ErrNoAPIKey.
func New(h *httpx.Client, apiKey string) *Client {
	return &Client{http: h, apiKey: apiKey, base: baseURL}
}

// Name satisfies domain.MarketProvider.
func (c *Client) Name() string { return "twelvedata" }

// --- Quote ---

type quoteResponse struct {
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	Exchange      string `json:"exchange"`
	Currency      string `json:"currency"`
	Open          string `json:"open"`
	High          string `json:"high"`
	Low           string `json:"low"`
	Close         string `json:"close"`
	PreviousClose string `json:"previous_close"`
	Change        string `json:"change"`
	PercentChange string `json:"percent_change"`
	Volume        string `json:"volume"`
	FiftyTwoWkH  string `json:"fifty_two_week_high"`
	FiftyTwoWkL  string `json:"fifty_two_week_low"`
	Timestamp     int64  `json:"timestamp"`

	// Error fields (returned on error instead of data).
	Status  string `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Quote fetches a real-time quote from Twelve Data.
func (c *Client) Quote(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	if c.apiKey == "" {
		return domain.Quote{}, fmt.Errorf("%w: twelvedata requires an API key (set twelvedata.api_key in config)", domain.ErrNoAPIKey)
	}

	u := fmt.Sprintf("%s/quote?%s", c.base, url.Values{
		"symbol": {strings.ToUpper(string(sym))},
		"apikey": {c.apiKey},
	}.Encode())

	body, err := c.doGet(ctx, u)
	if err != nil {
		return domain.Quote{}, err
	}

	var resp quoteResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return domain.Quote{}, fmt.Errorf("decode twelvedata: %w", err)
	}

	if resp.Status == "error" {
		return domain.Quote{}, classifyError(resp.Code, resp.Message, sym)
	}

	price := parseFloat(resp.Close)
	prevClose := parseFloat(resp.PreviousClose)
	change := parseFloat(resp.Change)
	changePct := parseFloat(resp.PercentChange)

	if price == 0 && prevClose == 0 {
		return domain.Quote{}, fmt.Errorf("%w: %s (all zeros)", domain.ErrNotFound, sym)
	}

	q := domain.Quote{
		Symbol:    sym,
		Name:      resp.Name,
		Currency:  resp.Currency,
		Exchange:  resp.Exchange,
		Price:     price,
		PrevClose: prevClose,
		Change:    change,
		ChangePct: changePct,
		Source:    domain.SourceTwelveData,
		FetchedAt: time.Now().UTC(),
		Partial:   true, // no P/E, EPS, Beta, Div Yield from /quote
	}

	if v := parseFloat(resp.Open); v > 0 {
		q.Open = domain.Some(v)
	}
	if v := parseFloat(resp.High); v > 0 {
		q.DayHigh = domain.Some(v)
	}
	if v := parseFloat(resp.Low); v > 0 {
		q.DayLow = domain.Some(v)
	}
	if v := parseInt(resp.Volume); v > 0 {
		q.Volume = domain.Some(v)
	}
	if v := parseFloat(resp.FiftyTwoWkH); v > 0 {
		q.Week52High = domain.Some(v)
	}
	if v := parseFloat(resp.FiftyTwoWkL); v > 0 {
		q.Week52Low = domain.Some(v)
	}
	if resp.Timestamp > 0 {
		q.AsOf = time.Unix(resp.Timestamp, 0).UTC()
	}

	return q, nil
}

// --- History ---

type timeSeriesResponse struct {
	Meta   tsMeta    `json:"meta"`
	Values []tsValue `json:"values"`

	// Error fields.
	Status  string `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tsMeta struct {
	Symbol   string `json:"symbol"`
	Interval string `json:"interval"`
	Currency string `json:"currency"`
	Exchange string `json:"exchange"`
}

type tsValue struct {
	Datetime string `json:"datetime"`
	Open     string `json:"open"`
	High     string `json:"high"`
	Low      string `json:"low"`
	Close    string `json:"close"`
	Volume   string `json:"volume"`
}

// History fetches daily OHLCV candles from Twelve Data.
func (c *Client) History(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("%w: twelvedata requires an API key", domain.ErrNoAPIKey)
	}

	sessions := r.Sessions
	if sessions <= 0 {
		sessions = 22
	}

	u := fmt.Sprintf("%s/time_series?%s", c.base, url.Values{
		"symbol":     {strings.ToUpper(string(sym))},
		"interval":   {"1day"},
		"outputsize": {strconv.Itoa(sessions)},
		"apikey":     {c.apiKey},
	}.Encode())

	body, err := c.doGet(ctx, u)
	if err != nil {
		return nil, err
	}

	var resp timeSeriesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode twelvedata: %w", err)
	}

	if resp.Status == "error" {
		return nil, classifyError(resp.Code, resp.Message, sym)
	}

	if len(resp.Values) == 0 {
		return nil, fmt.Errorf("%w: no candles", domain.ErrPartialData)
	}

	// Twelve Data returns newest first; reverse for chronological order.
	candles := make([]domain.Candle, 0, len(resp.Values))
	for i := len(resp.Values) - 1; i >= 0; i-- {
		v := resp.Values[i]
		t, _ := time.Parse("2006-01-02", v.Datetime)
		cl := parseFloat(v.Close)
		if cl == 0 {
			continue
		}
		candles = append(candles, domain.Candle{
			Time:   t,
			Open:   parseFloat(v.Open),
			High:   parseFloat(v.High),
			Low:    parseFloat(v.Low),
			Close:  cl,
			Volume: parseInt(v.Volume),
		})
	}

	if len(candles) == 0 {
		return nil, fmt.Errorf("%w: empty candles", domain.ErrPartialData)
	}
	return candles, nil
}

// --- HTTP helpers ---

func (c *Client) doGet(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrNetwork, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("%w: twelvedata", domain.ErrRateLimited)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: twelvedata", domain.ErrUnauthorized)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%w: twelvedata http %d", domain.ErrUnavailable, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB max
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", domain.ErrNetwork, err)
	}
	return body, nil
}

func classifyError(code int, msg string, sym domain.Ticker) error {
	lower := strings.ToLower(msg)
	switch {
	case code == 401:
		return fmt.Errorf("%w: twelvedata: %s", domain.ErrUnauthorized, msg)
	case code == 429:
		return fmt.Errorf("%w: twelvedata", domain.ErrRateLimited)
	case strings.Contains(lower, "not found") || strings.Contains(lower, "symbol not found"):
		return fmt.Errorf("%w: %s", domain.ErrNotFound, sym)
	case strings.Contains(lower, "no data"):
		return fmt.Errorf("%w: %s", domain.ErrPartialData, sym)
	default:
		return fmt.Errorf("twelvedata error %d: %s", code, msg)
	}
}

// --- parsing helpers ---

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func parseInt(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}
