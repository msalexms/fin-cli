// Package domain contains the core types and ports of fin-cli.
// It has no dependencies on infrastructure or presentation packages.
package domain

import "time"

// Ticker is a stock market symbol (e.g. "AAPL").
type Ticker string

// ISIN is an International Securities Identification Number.
type ISIN string

// Optional wraps a value that may or may not be provided by a data source.
// A zero Value is meaningless when Valid is false (avoids "0.0" magic).
type Optional[T any] struct {
	Value T
	Valid bool
}

// Some returns an Optional holding v.
func Some[T any](v T) Optional[T] { return Optional[T]{Value: v, Valid: true} }

// None returns an empty Optional.
func None[T any]() Optional[T] { return Optional[T]{} }

// QuoteSource identifies where a Quote came from.
type QuoteSource string

const (
	SourceFinnhub QuoteSource = "finnhub"
	SourceCache   QuoteSource = "cache"
	SourceUnknown QuoteSource = "unknown"
)

// MarketSession indicates whether the quote is pre-market, regular, or after-hours.
type MarketSession string

const (
	SessionUnknown MarketSession = ""
	SessionPre     MarketSession = "pre"
	SessionRegular MarketSession = "regular"
	SessionPost    MarketSession = "post"
	SessionClosed  MarketSession = "closed"
)

// Quote is a rich snapshot of a financial instrument.
// Numeric fields that the provider may omit are Optional.
type Quote struct {
	// Identity
	Symbol   Ticker
	Name     string
	Currency string // ISO 4217 (e.g. "USD") or provider-specific (e.g. "GBp")
	Exchange string // e.g. "NASDAQ NMS - GLOBAL MARKET"
	Country  string // ISO 3166-1 alpha-2 (e.g. "US")
	Industry string // e.g. "Technology"
	Weburl   string
	IPODate  string // YYYY-MM-DD

	// Price
	Price     float64
	PrevClose float64
	Change    float64
	ChangePct float64

	// Intraday range
	Open    Optional[float64]
	DayLow  Optional[float64]
	DayHigh Optional[float64]

	// Long-term range
	Week52Low  Optional[float64]
	Week52High Optional[float64]

	// Volume / capitalization
	Volume    Optional[int64]
	MarketCap Optional[float64] // in millions of Currency
	SharesOut Optional[float64] // in millions

	// Fundamentals (best-effort)
	PE       Optional[float64] // trailing P/E
	EPS      Optional[float64] // TTM
	Beta     Optional[float64]
	DivYield Optional[float64] // annualized percent

	// Provenance
	Session   MarketSession
	AsOf      time.Time // timestamp provided by the source
	FetchedAt time.Time // when fin-cli obtained the data
	Source    QuoteSource
	Partial   bool // true if some expected fields were missing
}

// Candle is one OHLCV bar.
type Candle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64
}

// Resolution of a historical series.
type Resolution string

const (
	ResolutionDaily Resolution = "D"
)

// Range specifies a historical window in trading sessions (not calendar days).
type Range struct {
	Sessions   int // approximate number of trading sessions
	Resolution Resolution
}

// DefaultRange is ~30 calendar days ≈ 22 sessions.
var DefaultRange = Range{Sessions: 22, Resolution: ResolutionDaily}

// IsISIN reports whether s matches the ISIN format (12 chars, 2 letters + 9 alnum + 1 digit).
func IsISIN(s string) bool {
	if len(s) != 12 {
		return false
	}
	for i := 0; i < 2; i++ {
		c := s[i]
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	for i := 2; i < 11; i++ {
		c := s[i]
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	last := s[11]
	return last >= '0' && last <= '9'
}
