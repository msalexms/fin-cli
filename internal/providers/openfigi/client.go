// Package openfigi resolves ISINs to tickers with the OpenFIGI mapping API
// (https://www.openfigi.com/api). It works without a key (25 req/min); a key
// raises the limit to 250 req/min.
package openfigi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/httpx"
)

const (
	name     = "openfigi"
	endpoint = "https://api.openfigi.com/v3/mapping"
)

// Client implements domain.ISINResolver.
type Client struct {
	http   *httpx.Client
	apiKey string
	url    string
}

// New builds a resolver. apiKey may be empty.
func New(h *httpx.Client, apiKey string) *Client {
	return &Client{http: h, apiKey: apiKey, url: endpoint}
}

type mapping struct {
	Data []struct {
		Ticker       string `json:"ticker"`
		ExchangeCode string `json:"exchCode"`
		MarketSector string `json:"marketSector"`
	} `json:"data"`
	Error string `json:"error"`
}

// Resolve returns the preferred ticker for isin: a US equity listing when
// available, otherwise the first equity, otherwise the first ticker.
func (c *Client) Resolve(ctx context.Context, isin domain.ISIN) (domain.Ticker, error) {
	body, _ := json.Marshal([]map[string]string{{"idType": "ID_ISIN", "idValue": string(isin)}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-OPENFIGI-APIKEY", c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	defer resp.Body.Close()
	if err := httpx.StatusError(name, resp.StatusCode); err != nil {
		return "", err
	}

	var out []mapping
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("%s: decode: %w", name, err)
	}
	if len(out) == 0 || len(out[0].Data) == 0 {
		return "", fmt.Errorf("%s: %w: no listing for ISIN %s", name, domain.ErrNotFound, isin)
	}

	items := out[0].Data
	pick := func(match func(i int) bool) domain.Ticker {
		for i, it := range items {
			if it.Ticker != "" && match(i) {
				return domain.Ticker(it.Ticker)
			}
		}
		return ""
	}
	isEquity := func(i int) bool { return items[i].MarketSector == "Equity" }
	for _, try := range []func(int) bool{
		func(i int) bool { return isEquity(i) && isUSExchange(items[i].ExchangeCode) },
		isEquity,
		func(int) bool { return true },
	} {
		if t := pick(try); t != "" {
			return t, nil
		}
	}
	return "", fmt.Errorf("%s: %w: no ticker for ISIN %s", name, domain.ErrNotFound, isin)
}

func isUSExchange(code string) bool {
	switch code {
	case "US", "UN", "UW", "UQ", "UA", "UP", "UR", "UV":
		return true
	}
	return false
}
