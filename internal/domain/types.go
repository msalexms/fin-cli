// Package domain contains the core types and ports of fin-cli.
// It has no dependencies on infrastructure or presentation packages.
package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Ticker is a market symbol as understood by the providers (e.g. "AAPL",
// "SAN.MC", "BTC-USD").
type Ticker string

// ISIN is an International Securities Identification Number.
type ISIN string

// Optional wraps a value that a data source may not provide, so that a
// missing value is never confused with a real zero.
type Optional[T any] struct {
	Value T
	Valid bool
}

// Some returns an Optional holding v.
func Some[T any](v T) Optional[T] { return Optional[T]{Value: v, Valid: true} }

// MarshalJSON encodes the value, or null when it is missing.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if !o.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}

// UnmarshalJSON is the inverse of MarshalJSON.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*o = Optional[T]{}
		return nil
	}
	if err := json.Unmarshal(b, &o.Value); err != nil {
		return err
	}
	o.Valid = true
	return nil
}

// MarketSession is the trading session the quote belongs to.
type MarketSession string

const (
	SessionUnknown MarketSession = ""
	SessionPre     MarketSession = "pre"
	SessionRegular MarketSession = "regular"
	SessionPost    MarketSession = "post"
	SessionClosed  MarketSession = "closed"
)

// Quote is a snapshot of a financial instrument. Fields that providers may
// omit are Optional.
type Quote struct {
	// Identity
	Symbol   Ticker `json:"symbol"`
	Name     string `json:"name,omitempty"`
	Currency string `json:"currency,omitempty"` // ISO 4217 or provider specific (e.g. "GBp")
	Exchange string `json:"exchange,omitempty"`
	Country  string `json:"country,omitempty"`
	Industry string `json:"industry,omitempty"`
	IPODate  string `json:"ipo_date,omitempty"`

	// Price
	Price     float64 `json:"price"`
	PrevClose float64 `json:"prev_close"`
	Change    float64 `json:"change"`
	ChangePct float64 `json:"change_pct"`

	Open       Optional[float64] `json:"open"`
	DayLow     Optional[float64] `json:"day_low"`
	DayHigh    Optional[float64] `json:"day_high"`
	Week52Low  Optional[float64] `json:"week52_low"`
	Week52High Optional[float64] `json:"week52_high"`

	Volume    Optional[int64]   `json:"volume"`     // today's volume
	AvgVolume Optional[int64]   `json:"avg_volume"` // average daily volume (provider window)
	MarketCap Optional[float64] `json:"market_cap"` // in millions of Currency

	// Fundamentals (best effort)
	PE       Optional[float64] `json:"pe"`
	EPS      Optional[float64] `json:"eps"`
	Beta     Optional[float64] `json:"beta"`
	DivYield Optional[float64] `json:"div_yield"` // annual percent

	// Provenance
	Session   MarketSession `json:"session,omitempty"`
	AsOf      time.Time     `json:"as_of"`      // timestamp reported by the source
	FetchedAt time.Time     `json:"fetched_at"` // when fin-cli obtained the data
	Source    string        `json:"source"`     // provider name
	Partial   bool          `json:"partial"`    // some sections could not be fetched
	Stale     bool          `json:"stale"`      // served from cache because every provider failed
}

// Candle is one daily OHLCV bar.
type Candle struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume int64     `json:"volume"`
}

// HistorySessions is the number of daily sessions shown in charts (~1 month).
const HistorySessions = 22

// ParseTicker normalizes and validates user input as a ticker symbol.
func ParseTicker(raw string) (Ticker, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return "", fmt.Errorf("%w: empty ticker", ErrInvalidInput)
	}
	if len(s) > 20 {
		return "", fmt.Errorf("%w: ticker %q is too long", ErrInvalidInput, raw)
	}
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '-', r == '^', r == '=', r == '_', r == ':':
		default:
			return "", fmt.Errorf("%w: ticker %q contains %q", ErrInvalidInput, raw, r)
		}
	}
	return Ticker(s), nil
}

// IsISIN reports whether s is a well-formed ISIN: 2 letters, 9 alphanumerics
// and a check digit that satisfies the Luhn algorithm. The checksum keeps
// 12-character tickers from being mistaken for ISINs.
func IsISIN(s string) bool {
	if len(s) != 12 {
		return false
	}
	for i := range 2 {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	// Expand letters to two digits (A=10 … Z=35), then apply Luhn.
	var digits []int
	for i := range 12 {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			digits = append(digits, int(c-'0'))
		case c >= 'A' && c <= 'Z' && i < 11:
			v := int(c-'A') + 10
			digits = append(digits, v/10, v%10)
		default:
			return false
		}
	}
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := digits[i]
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}
