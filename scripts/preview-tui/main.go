//go:build preview

// Non-interactive preview of the TUI layout. Writes the View() output to stdout
// using synthetic data so you can inspect colors, backgrounds, and layout.
//
//	go run -tags preview ./scripts/preview-tui | less -R
//
// To inspect raw bytes (e.g. ANSI escapes) use:
//	go run -tags preview ./scripts/preview-tui | cat -v
package main

import (
	"fmt"
	"os"
	"time"

	"fin-cli/internal/cli"
	"fin-cli/internal/domain"
	"fin-cli/internal/tui"
)

func main() {
	cfgPath := "/tmp/fincli-preview-cfg.toml"
	_ = os.Remove(cfgPath)

	app, err := cli.NewApp(cli.AppOptions{ConfigPath: cfgPath, WatchlistPath: "/tmp/fincli-preview-wl.toml"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "app:", err)
		os.Exit(1)
	}
	defer app.Close()

	now := time.Now().UTC()
	quotes := []domain.Quote{
		{
			Symbol: "AAPL", Name: "Apple Inc.", Currency: "USD",
			Exchange: "NASDAQ NMS - GLOBAL MARKET", Country: "US", Industry: "Technology",
			IPODate:  "1980-12-12",
			Price:    189.42, PrevClose: 187.11, Change: 2.31, ChangePct: 1.234,
			Open: domain.Some(188.00), DayLow: domain.Some(186.90), DayHigh: domain.Some(190.20),
			Week52Low: domain.Some(164.08), Week52High: domain.Some(199.62),
			Volume: domain.Some(int64(52_300_000)),
			MarketCap: domain.Some(2_900_000.0),
			PE:  domain.Some(31.2), EPS: domain.Some(6.07),
			Beta: domain.Some(1.28), DivYield: domain.Some(0.52),
			Session: domain.SessionRegular,
			Source:  domain.SourceFinnhub, AsOf: now, FetchedAt: now,
		},
		{
			Symbol: "MSFT", Name: "Microsoft Corp.", Currency: "USD",
			Price: 341.21, PrevClose: 342.80, Change: -1.59, ChangePct: -0.464,
			Source: domain.SourceFinnhub, AsOf: now, FetchedAt: now,
		},
		{
			Symbol: "GOOGL", Name: "Alphabet Inc.", Currency: "USD",
			Price: 139.85, PrevClose: 139.85, Change: 0.00, ChangePct: 0.0,
			Source: domain.SourceCache, AsOf: now, FetchedAt: now,
		},
	}

	m := tui.NewPreviewModel(app, 120, 30, quotes)
	fmt.Print(m.Render())
}
