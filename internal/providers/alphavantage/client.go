// Package alphavantage implements domain.MarketProvider against the Alpha Vantage
// REST API (https://www.alphavantage.co/documentation/).
//
// Free tier: 25 requests/day, 5 requests/minute.
// This provider is intended as a last-resort fallback due to the harsh daily limit.
// Requires a free API key from https://www.alphavantage.co/support/#api-key.
package alphavantage

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

const baseURL = "https://www.alphavantage.co/query"

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
func (c *Client) Name() string { return "alphavantage" }

// --- Quote ---

// globalQuoteResponse maps the Alpha Vantage GLOBAL_QUOTE response.
type globalQuoteResponse struct {
	GlobalQuote globalQuoteData `json:"Global Quote"`
	Note        string          `json:"Note"`        // rate limit notice
	Information string          `json:"Information"` // invalid key / other messages
}

type globalQuoteData struct {
	Symbol        string `json:"01. symbol"`
	Open          string `json:"02. open"`
	High          string `json:"03. high"`
	Low           string `json:"04. low"`
	Price         string `json:"05. price"`
	Volume        string `json:"06. volume"`
	LatestDay     string `json:"07. latest trading day"`
	PreviousClose string `json:"08. previous close"`
	Change        string `json:"09. change"`
	ChangePct     string `json:"10. change percent"`
}

// Quote fetches a real-time quote via GLOBAL_QUOTE.
func (c *Client) Quote(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	if c.apiKey == "" {
		return domain.Quote{}, fmt.Errorf("%w: alphavantage requires an API key (set alphavantage.api_key in config)", domain.ErrNoAPIKey)
	}

	u := fmt.Sprintf("%s?%s", c.base, url.Values{
		"function": {"GLOBAL_QUOTE"},
		"symbol":   {strings.ToUpper(string(sym))},
		"apikey":   {c.apiKey},
	}.Encode())

	body, err := c.doGet(ctx, u)
	if err != nil {
		return domain.Quote{}, err
	}

	var resp globalQuoteResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return domain.Quote{}, fmt.Errorf("decode alphavantage: %w", err)
	}

	// Check for rate limit / error messages.
	if resp.Note != "" {
		return domain.Quote{}, fmt.Errorf("%w: alphavantage daily limit reached", domain.ErrRateLimited)
	}
	if resp.Information != "" {
		if strings.Contains(strings.ToLower(resp.Information), "invalid") ||
			strings.Contains(strings.ToLower(resp.Information), "premium") {
			return domain.Quote{}, fmt.Errorf("%w: alphavantage: %s", domain.ErrUnauthorized, resp.Information)
		}
		return domain.Quote{}, fmt.Errorf("%w: alphavantage: %s", domain.ErrRateLimited, resp.Information)
	}

	gq := resp.GlobalQuote
	if gq.Symbol == "" && gq.Price == "" {
		return domain.Quote{}, fmt.Errorf("%w: %s", domain.ErrNotFound, sym)
	}

	price := parseFloat(gq.Price)
	prevClose := parseFloat(gq.PreviousClose)
	change := parseFloat(gq.Change)
	changePct := parseFloat(strings.TrimSuffix(gq.ChangePct, "%"))

	if price == 0 && prevClose == 0 {
		return domain.Quote{}, fmt.Errorf("%w: %s (all zeros)", domain.ErrNotFound, sym)
	}

	q := domain.Quote{
		Symbol:    sym,
		Price:     price,
		PrevClose: prevClose,
		Change:    change,
		ChangePct: changePct,
		Source:    domain.SourceAlphaVantage,
		FetchedAt: time.Now().UTC(),
		Partial:   true, // no fundamentals from GLOBAL_QUOTE
	}

	if v := parseFloat(gq.Open); v > 0 {
		q.Open = domain.Some(v)
	}
	if v := parseFloat(gq.High); v > 0 {
		q.DayHigh = domain.Some(v)
	}
	if v := parseFloat(gq.Low); v > 0 {
		q.DayLow = domain.Some(v)
	}
	if v := parseInt(gq.Volume); v > 0 {
		q.Volume = domain.Some(v)
	}
	if gq.LatestDay != "" {
		if t, err := time.Parse("2006-01-02", gq.LatestDay); err == nil {
			q.AsOf = t
		}
	}

	return q, nil
}

// --- History ---

type dailyResponse struct {
	MetaData   dailyMeta                  `json:"Meta Data"`
	TimeSeries map[string]dailyDataPoint  `json:"Time Series (Daily)"`
	Note       string                     `json:"Note"`
	Information string                    `json:"Information"`
}

type dailyMeta struct {
	Symbol      string `json:"2. Symbol"`
	LastRefresh string `json:"3. Last Refreshed"`
}

type dailyDataPoint struct {
	Open   string `json:"1. open"`
	High   string `json:"2. high"`
	Low    string `json:"3. low"`
	Close  string `json:"4. close"`
	Volume string `json:"5. volume"`
}

// History fetches daily OHLCV candles via TIME_SERIES_DAILY.
func (c *Client) History(ctx context.Context, sym domain.Ticker, r domain.Range) ([]domain.Candle, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("%w: alphavantage requires an API key", domain.ErrNoAPIKey)
	}

	u := fmt.Sprintf("%s?%s", c.base, url.Values{
		"function":   {"TIME_SERIES_DAILY"},
		"symbol":     {strings.ToUpper(string(sym))},
		"outputsize": {"compact"}, // latest 100 data points
		"apikey":     {c.apiKey},
	}.Encode())

	body, err := c.doGet(ctx, u)
	if err != nil {
		return nil, err
	}

	var resp dailyResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode alphavantage: %w", err)
	}

	if resp.Note != "" {
		return nil, fmt.Errorf("%w: alphavantage daily limit reached", domain.ErrRateLimited)
	}
	if resp.Information != "" {
		return nil, fmt.Errorf("%w: alphavantage: %s", domain.ErrRateLimited, resp.Information)
	}

	if len(resp.TimeSeries) == 0 {
		return nil, fmt.Errorf("%w: %s no history data", domain.ErrNotFound, sym)
	}

	// Collect and sort dates.
	all := make([]dated, 0, len(resp.TimeSeries))
	for dateStr, dp := range resp.TimeSeries {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		all = append(all, dated{t: t, d: dp})
	}

	// Sort chronologically (oldest first).
	sortDated(all)

	// Trim to requested sessions.
	sessions := r.Sessions
	if sessions <= 0 {
		sessions = 22
	}
	if len(all) > sessions {
		all = all[len(all)-sessions:]
	}

	candles := make([]domain.Candle, 0, len(all))
	for _, item := range all {
		cl := parseFloat(item.d.Close)
		if cl == 0 {
			continue
		}
		candles = append(candles, domain.Candle{
			Time:   item.t,
			Open:   parseFloat(item.d.Open),
			High:   parseFloat(item.d.High),
			Low:    parseFloat(item.d.Low),
			Close:  cl,
			Volume: parseInt(item.d.Volume),
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
		return nil, fmt.Errorf("%w: alphavantage", domain.ErrRateLimited)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w: alphavantage", domain.ErrUnauthorized)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%w: alphavantage http %d", domain.ErrUnavailable, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2MB max
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", domain.ErrNetwork, err)
	}
	return body, nil
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

// dated pairs a time with a data point for sorting.
type dated struct {
	t time.Time
	d dailyDataPoint
}

// sortDated sorts a slice of dated items chronologically (oldest first).
func sortDated(items []dated) {
	// Simple insertion sort — the dataset is at most 100 items.
	for i := 1; i < len(items); i++ {
		key := items[i]
		j := i - 1
		for j >= 0 && items[j].t.After(key.t) {
			items[j+1] = items[j]
			j--
		}
		items[j+1] = key
	}
}
