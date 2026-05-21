// Package openfigi implements domain.IsinResolver against https://www.openfigi.com/api.
//
// POST /v3/mapping with [{"idType":"ID_ISIN","idValue":"..."}].
// Works without an API key (25 req/min); a key raises the limit to 250 req/min
// and is sent via the X-OPENFIGI-APIKEY header.
package openfigi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"fin-cli/internal/domain"
	"fin-cli/internal/httpx"
)

const endpoint = "https://api.openfigi.com/v3/mapping"

// Client resolves ISINs to tickers.
type Client struct {
	http   *httpx.Client
	apiKey string
	url    string
}

// New builds a resolver. apiKey may be empty.
func New(h *httpx.Client, apiKey string) *Client {
	return &Client{http: h, apiKey: apiKey, url: endpoint}
}

type mappingReq struct {
	IDType  string `json:"idType"`
	IDValue string `json:"idValue"`
}

type mappingItem struct {
	Ticker          string `json:"ticker"`
	Name            string `json:"name"`
	ExchangeCode    string `json:"exchCode"`
	SecurityType    string `json:"securityType"`
	MarketSector    string `json:"marketSector"`
}

type mappingResp struct {
	Data    []mappingItem `json:"data"`
	Warning string        `json:"warning"`
	Error   string        `json:"error"`
}

// Resolve returns the preferred ticker for isin. Prefers US equities when multiple
// candidates are returned. Returns domain.ErrNotFound if no mapping exists.
func (c *Client) Resolve(ctx context.Context, isin domain.ISIN) (domain.Ticker, error) {
	if !domain.IsISIN(string(isin)) {
		return "", fmt.Errorf("%w: invalid ISIN %q", domain.ErrInvalidInput, isin)
	}
	body, err := json.Marshal([]mappingReq{{IDType: "ID_ISIN", IDValue: string(isin)}})
	if err != nil {
		return "", err
	}

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
		return "", fmt.Errorf("%w: %v", domain.ErrNetwork, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("%w: openfigi", domain.ErrRateLimited)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("%w: openfigi", domain.ErrUnauthorized)
	}
	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("%w: openfigi http %d", domain.ErrUnavailable, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("openfigi http %d: %s", resp.StatusCode, string(b))
	}

	var out []mappingResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode openfigi: %w", err)
	}
	if len(out) == 0 {
		return "", fmt.Errorf("%w: empty response", domain.ErrPartialData)
	}

	item := out[0]
	if item.Error != "" && len(item.Data) == 0 {
		if matchesNotFound(item.Error) {
			return "", fmt.Errorf("%w: %s", domain.ErrNotFound, isin)
		}
		return "", errors.New(item.Error)
	}
	if len(item.Data) == 0 {
		return "", fmt.Errorf("%w: %s", domain.ErrNotFound, isin)
	}

	best := pickBest(item.Data)
	if best.Ticker == "" {
		return "", fmt.Errorf("%w: %s", domain.ErrNotFound, isin)
	}
	return domain.Ticker(best.Ticker), nil
}

// pickBest chooses a sensible candidate: prefer US, then Common Stock equities.
func pickBest(items []mappingItem) mappingItem {
	// 1) Equity + US exchange (primary listing).
	for _, it := range items {
		if it.MarketSector == "Equity" && isUSExchange(it.ExchangeCode) && it.Ticker != "" {
			return it
		}
	}
	// 2) Any equity.
	for _, it := range items {
		if it.MarketSector == "Equity" && it.Ticker != "" {
			return it
		}
	}
	// 3) First with a ticker.
	for _, it := range items {
		if it.Ticker != "" {
			return it
		}
	}
	return items[0]
}

func isUSExchange(code string) bool {
	switch code {
	case "US", "UN", "UW", "UQ", "UA", "UP", "UR", "UV":
		return true
	}
	return false
}

func matchesNotFound(msg string) bool {
	return msg == "No identifier found." || msg == "No FIGI found."
}
