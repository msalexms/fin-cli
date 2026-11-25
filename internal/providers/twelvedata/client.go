// Package twelvedata implements domain.Provider against the Twelve Data REST
// API (https://twelvedata.com/docs). Free tier: 800 requests/day, 8/minute.
package twelvedata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/httpx"
)

const (
	name    = "twelvedata"
	baseURL = "https://api.twelvedata.com"
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

// apiError is embedded in every response; Twelve Data reports errors with
// HTTP 200 and status "error".
type apiError struct {
	Status  string `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e apiError) err(sym domain.Ticker) error {
	if e.Status != "error" {
		return nil
	}
	msg := strings.ToLower(e.Message)
	switch {
	case e.Code == 401 || e.Code == 403 || strings.Contains(msg, "api key"):
		return fmt.Errorf("%s: %w: %s", name, domain.ErrUnauthorized, e.Message)
	case e.Code == 429 || strings.Contains(msg, "credits"):
		return fmt.Errorf("%s: %w", name, domain.ErrRateLimited)
	case e.Code == 404 || strings.Contains(msg, "not found"):
		return fmt.Errorf("%s: %w: %s", name, domain.ErrNotFound, sym)
	case strings.Contains(msg, "no data"):
		return fmt.Errorf("%s: %w: %s", name, domain.ErrNoData, sym)
	default:
		return fmt.Errorf("%s: error %d: %s", name, e.Code, e.Message)
	}
}

type quoteResponse struct {
	apiError
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
	AverageVolume string `json:"average_volume"`
	Timestamp     int64  `json:"timestamp"`
	IsMarketOpen  *bool  `json:"is_market_open"`
	FiftyTwoWeek  struct {
		Low  string `json:"low"`
		High string `json:"high"`
	} `json:"fifty_two_week"`
}

// Quote implements domain.Provider.
func (c *Client) Quote(ctx context.Context, sym domain.Ticker) (domain.Quote, error) {
	var r quoteResponse
	if err := c.get(ctx, "/quote", url.Values{"symbol": {string(sym)}}, &r); err != nil {
		return domain.Quote{}, err
	}
	if err := r.err(sym); err != nil {
		return domain.Quote{}, err
	}
	price := num(r.Close)
	if price == 0 {
		return domain.Quote{}, fmt.Errorf("%s: %w: %s", name, domain.ErrNotFound, sym)
	}

	now := time.Now().UTC()
	q := domain.Quote{
		Symbol:    sym,
		Name:      r.Name,
		Currency:  r.Currency,
		Exchange:  r.Exchange,
		Price:     price,
		PrevClose: num(r.PreviousClose),
		Change:    num(r.Change),
		ChangePct: num(r.PercentChange),
		AsOf:      now,
		FetchedAt: now,
		Source:    name,
		Partial:   true, // no fundamentals on /quote
	}
	if r.Timestamp > 0 {
		q.AsOf = time.Unix(r.Timestamp, 0).UTC()
	}
	if r.IsMarketOpen != nil {
		q.Session = domain.SessionClosed
		if *r.IsMarketOpen {
			q.Session = domain.SessionRegular
		}
	}
	setPositive(&q.Open, num(r.Open))
	setPositive(&q.DayHigh, num(r.High))
	setPositive(&q.DayLow, num(r.Low))
	setPositive(&q.Week52High, num(r.FiftyTwoWeek.High))
	setPositive(&q.Week52Low, num(r.FiftyTwoWeek.Low))
	if v := int64(num(r.Volume)); v > 0 {
		q.Volume = domain.Some(v)
	}
	if v := int64(num(r.AverageVolume)); v > 0 {
		q.AvgVolume = domain.Some(v)
	}
	return q, nil
}

type seriesResponse struct {
	apiError
	Values []struct {
		Datetime string `json:"datetime"`
		Open     string `json:"open"`
		High     string `json:"high"`
		Low      string `json:"low"`
		Close    string `json:"close"`
		Volume   string `json:"volume"`
	} `json:"values"`
}

// History implements domain.Provider.
func (c *Client) History(ctx context.Context, sym domain.Ticker, sessions int) ([]domain.Candle, error) {
	var r seriesResponse
	q := url.Values{"symbol": {string(sym)}, "interval": {"1day"}, "outputsize": {strconv.Itoa(sessions)}}
	if err := c.get(ctx, "/time_series", q, &r); err != nil {
		return nil, err
	}
	if err := r.err(sym); err != nil {
		return nil, err
	}
	// Values come newest first.
	out := make([]domain.Candle, 0, len(r.Values))
	for i := len(r.Values) - 1; i >= 0; i-- {
		v := r.Values[i]
		t, err := time.Parse("2006-01-02", v.Datetime)
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
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	if c.apiKey == "" {
		return fmt.Errorf("%s: %w", name, domain.ErrNoAPIKey)
	}
	q.Set("apikey", c.apiKey)
	body, err := c.http.GetJSON(ctx, name, c.base+path+"?"+q.Encode(), nil, 1<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: decode %s: %w", name, path, err)
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
