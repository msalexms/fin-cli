// Package alphavantage implements domain.Provider against the Alpha Vantage
// REST API (https://www.alphavantage.co/documentation/). The free tier allows
// 25 requests per day, so it is meant as a last-resort fallback.
package alphavantage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/httpx"
)

const (
	name    = "alphavantage"
	baseURL = "https://www.alphavantage.co/query"
)

// Client implements domain.Provider.
type Client struct {
	http   *httpx.Client
	apiKey string
	base   string
}

// New returns a Client. With an empty apiKey every call fails with ErrNoAPIKey.
func New(h *httpx.Client, apiKey string) *Client {
	return &Client{http: h, apiKey: apiKey, base: baseURL}
}

// Name implements domain.Provider.
func (c *Client) Name() string { return name }

// notice holds the free-text fields Alpha Vantage uses instead of HTTP errors.
type notice struct {
	Note         string `json:"Note"`
	Information  string `json:"Information"`
	ErrorMessage string `json:"Error Message"`
}

func (n notice) err(sym domain.Ticker) error {
	msg := strings.ToLower(n.Note + " " + n.Information)
	switch {
	case n.ErrorMessage != "":
		return fmt.Errorf("%s: %w: %s", name, domain.ErrNotFound, sym)
	case strings.TrimSpace(msg) == "":
		return nil
	// The daily-limit message also mentions "premium plans", so check it first.
	case strings.Contains(msg, "rate limit") || strings.Contains(msg, "per day") ||
		strings.Contains(msg, "call frequency") || strings.Contains(msg, "requests"):
		return fmt.Errorf("%s: %w: daily limit reached", name, domain.ErrRateLimited)
	case strings.Contains(msg, "api key") || strings.Contains(msg, "apikey") || strings.Contains(msg, "premium"):
		return fmt.Errorf("%s: %w", name, domain.ErrUnauthorized)
	default:
		return fmt.Errorf("%s: %w: %s", name, domain.ErrUnavailable, strings.TrimSpace(n.Note+" "+n.Information))
	}
}

type globalQuote struct {
	notice
	Quote struct {
		Symbol    string `json:"01. symbol"`
		Open      string `json:"02. open"`
		High      string `json:"03. high"`
		Low       string `json:"04. low"`
		Price     string `json:"05. price"`
		Volume    string `json:"06. volume"`
		LatestDay string `json:"07. latest trading day"`
		PrevClose string `json:"08. previous close"`
		Change    string `json:"09. change"`
		ChangePct string `json:"10. change percent"`
	} `json:"Global Quote"`
}

// Quote implements domain.Provider.
func (c *Client) Quote(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	var r globalQuote
	if err := c.get(ctx, url.Values{"function": {"GLOBAL_QUOTE"}, "symbol": {string(sym)}}, &r); err != nil {
		return domain.Quote{}, err
	}
	if err := r.err(sym); err != nil {
		return domain.Quote{}, err
	}
	gq := r.Quote
	price := num(gq.Price)
	if price == 0 {
		return domain.Quote{}, fmt.Errorf("%s: %w: %s", name, domain.ErrNotFound, sym)
	}

	now := time.Now().UTC()
	q := domain.Quote{
		Symbol:    sym,
		Price:     price,
		PrevClose: num(gq.PrevClose),
		Change:    num(gq.Change),
		ChangePct: num(strings.TrimSuffix(gq.ChangePct, "%")),
		AsOf:      now,
		FetchedAt: now,
		Source:    name,
		Partial:   true, // GLOBAL_QUOTE has no identity or fundamentals
	}
	if t, err := time.Parse("2006-01-02", gq.LatestDay); err == nil {
		q.AsOf = t
	}
	setPositive(&q.Open, num(gq.Open))
	setPositive(&q.DayHigh, num(gq.High))
	setPositive(&q.DayLow, num(gq.Low))
	if v := int64(num(gq.Volume)); v > 0 {
		q.Volume = domain.Some(v)
	}
	return q, nil
}

type dailySeries struct {
	notice
	Series map[string]struct {
		Open   string `json:"1. open"`
		High   string `json:"2. high"`
		Low    string `json:"3. low"`
		Close  string `json:"4. close"`
		Volume string `json:"5. volume"`
	} `json:"Time Series (Daily)"`
}

// History implements domain.Provider (compact output: last 100 sessions).
func (c *Client) History(ctx context.Context, sym domain.Ticker, sessions int) ([]domain.Candle, error) {
	var r dailySeries
	q := url.Values{"function": {"TIME_SERIES_DAILY"}, "symbol": {string(sym)}, "outputsize": {"compact"}}
	if err := c.get(ctx, q, &r); err != nil {
		return nil, err
	}
	if err := r.err(sym); err != nil {
		return nil, err
	}
	out := make([]domain.Candle, 0, len(r.Series))
	for day, v := range r.Series {
		t, err := time.Parse("2006-01-02", day)
		if err != nil || num(v.Close) == 0 {
			continue
		}
		out = append(out, domain.Candle{
			Time: t, Open: num(v.Open), High: num(v.High), Low: num(v.Low),
			Close: num(v.Close), Volume: int64(num(v.Volume)),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: %w: empty series", name, domain.ErrNoData)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	if len(out) > sessions {
		out = out[len(out)-sessions:]
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, q url.Values, out any) error {
	if c.apiKey == "" {
		return fmt.Errorf("%s: %w", name, domain.ErrNoAPIKey)
	}
	q.Set("apikey", c.apiKey)
	body, err := c.http.GetJSON(ctx, name, c.base+"?"+q.Encode(), nil, 2<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: decode: %w", name, err)
	}
	return nil
}

func num(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func setPositive(dst *domain.Optional[float64], v float64) {
	if v > 0 {
		*dst = domain.Some(v)
	}
}
