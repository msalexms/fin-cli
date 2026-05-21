//go:build ignore

// Visual preview of the CLI renderer with a synthetic Quote.
// Run with: go run ./scripts/preview
package main

import (
	"os"
	"time"

	"fin-cli/internal/cli"
	"fin-cli/internal/domain"
	"fin-cli/internal/locale"
)

func main() {
	now := time.Now().UTC()
	q := domain.Quote{
		Symbol:     "AAPL",
		Name:       "Apple Inc.",
		Currency:   "USD",
		Exchange:   "NASDAQ NMS - GLOBAL MARKET",
		Country:    "US",
		Industry:   "Technology",
		IPODate:    "1980-12-12",
		Price:      189.42,
		PrevClose:  187.11,
		Change:     2.31,
		ChangePct:  1.234,
		Open:       domain.Some(188.00),
		DayLow:     domain.Some(186.90),
		DayHigh:    domain.Some(190.20),
		Week52Low:  domain.Some(164.08),
		Week52High: domain.Some(199.62),
		Volume:     domain.Some(int64(52_300_000)),
		MarketCap:  domain.Some(2_900_000.0), // millions → 2.9T
		SharesOut:  domain.Some(15_300.0),
		PE:         domain.Some(31.2),
		EPS:        domain.Some(6.07),
		Beta:       domain.Some(1.28),
		DivYield:   domain.Some(0.52),
		Session:    domain.SessionRegular,
		AsOf:       now,
		FetchedAt:  now,
		Source:     domain.SourceFinnhub,
	}

	// Synthetic sinusoidal candle series, 22 sessions.
	candles := make([]domain.Candle, 0, 22)
	base := 180.0
	for i := 0; i < 22; i++ {
		base += float64(i%5-2) * 1.4
		candles = append(candles, domain.Candle{
			Time:  now.AddDate(0, 0, -22+i),
			Open:  base - 0.3,
			High:  base + 1.0,
			Low:   base - 1.2,
			Close: base,
			Volume: int64(40_000_000 + i*500_000),
		})
	}

	cli.RenderQuote(os.Stdout, q, candles, cli.RenderOptions{
		NoColor:     os.Getenv("NO_COLOR") != "",
		ASCIIOnly:   false,
		Width:       80,
		ChartHeight: 10,
		Printer:     locale.Detect(),
	})
}
