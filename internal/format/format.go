// Package format holds the number and label formatting shared by the CLI
// renderer and the TUI, so both always show the same text for a quote.
package format

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/msalexms/fin-cli/internal/domain"
)

// Dash marks a value the provider did not supply.
const Dash = "—"

// Printer is a locale-aware Sprintf (see locale.Printer).
type Printer interface {
	Sprintf(format string, a ...any) string
}

// Decimals is the precision used for a price of this magnitude: two for
// regular prices, four below 10 (FX pairs, penny stocks) and six below 0.01
// (small crypto), so meaningful digits are never rounded away.
func Decimals(v float64) int {
	switch a := math.Abs(v); {
	case a == 0 || a >= 10:
		return 2
	case a >= 0.01:
		return 4
	default:
		return 6
	}
}

// Price formats a price with Decimals precision.
func Price(p Printer, v float64) string {
	return p.Sprintf("%.*f", Decimals(v), v)
}

// OptPrice formats an optional price.
func OptPrice(p Printer, o domain.Optional[float64]) string {
	if !o.Valid {
		return Dash
	}
	return Price(p, o.Value)
}

// OptFloat formats an optional ratio with two decimals.
func OptFloat(p Printer, o domain.Optional[float64]) string {
	if !o.Valid {
		return Dash
	}
	return p.Sprintf("%.2f", o.Value)
}

// OptPercent formats an optional percentage.
func OptPercent(p Printer, o domain.Optional[float64]) string {
	if !o.Valid {
		return Dash
	}
	return p.Sprintf("%.2f%%", o.Value)
}

// Range formats "low – high", or a dash if either bound is missing.
func Range(p Printer, lo, hi domain.Optional[float64]) string {
	if !lo.Valid || !hi.Valid {
		return Dash
	}
	return Price(p, lo.Value) + " – " + Price(p, hi.Value)
}

// Volume formats an optional share count compactly (K/M/B).
func Volume(v domain.Optional[int64]) string {
	if !v.Valid {
		return Dash
	}
	return HumanizeInt(v.Value)
}

// MarketCap formats a market cap expressed in millions of currency.
func MarketCap(m domain.Optional[float64], currency string) string {
	if !m.Valid {
		return Dash
	}
	return strings.TrimSpace(HumanizeMillions(m.Value) + " " + currency)
}

// HumanizeMillions converts a value in millions to M/B/T.
func HumanizeMillions(v float64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.2fT", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.2fB", v/1_000)
	default:
		return fmt.Sprintf("%.2fM", v)
	}
}

// HumanizeInt converts a raw count to K/M/B.
func HumanizeInt(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.2fK", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// ChangeArrow returns ▲/▼/• (or ^/v/= in ASCII mode) for the sign of change.
func ChangeArrow(change float64, asciiOnly bool) string {
	switch {
	case change > 0 && asciiOnly:
		return "^"
	case change > 0:
		return "▲"
	case change < 0 && asciiOnly:
		return "v"
	case change < 0:
		return "▼"
	case asciiOnly:
		return "="
	default:
		return "•"
	}
}

// Change formats "+1.23 (+0.65%)", with the absolute change at the
// precision of price.
func Change(p Printer, price, change, pct float64) string {
	return p.Sprintf("%+.*f (%+.2f%%)", Decimals(price), change, pct)
}

// Percent formats "+0.65%".
func Percent(p Printer, pct float64) string {
	return p.Sprintf("%+.2f%%", pct)
}

// SessionLabel names the market session.
func SessionLabel(s domain.MarketSession) string {
	switch s {
	case domain.SessionPre:
		return "pre-market"
	case domain.SessionRegular:
		return "market open"
	case domain.SessionPost:
		return "after-hours"
	case domain.SessionClosed:
		return "market closed"
	}
	return ""
}

// Provenance describes where a quote came from: "via yahoo · partial data"
// or "offline · cached 14:02".
func Provenance(q domain.Quote) string {
	if q.Stale {
		return "offline · cached " + q.FetchedAt.Local().Format("15:04")
	}
	s := "via " + q.Source
	if q.Partial {
		s += " · partial data"
	}
	return s
}

// MetaLine joins exchange, industry, country and IPO date.
func MetaLine(q domain.Quote) string {
	var parts []string
	for _, s := range []string{q.Exchange, q.Industry, q.Country} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if q.IPODate != "" {
		parts = append(parts, "IPO "+q.IPODate)
	}
	return strings.Join(parts, " · ")
}

// StatsRows returns the label/value pairs of the stats grid.
func StatsRows(q domain.Quote, p Printer) [][2]string {
	vol := [2]string{"Volume", Volume(q.Volume)}
	if !q.Volume.Valid && q.AvgVolume.Valid {
		vol = [2]string{"Avg Volume", Volume(q.AvgVolume)}
	}
	return [][2]string{
		{"Prev Close", Price(p, q.PrevClose)},
		{"Open", OptPrice(p, q.Open)},
		{"Day Range", Range(p, q.DayLow, q.DayHigh)},
		{"52w Range", Range(p, q.Week52Low, q.Week52High)},
		vol,
		{"Market Cap", MarketCap(q.MarketCap, q.Currency)},
		{"P/E", OptFloat(p, q.PE)},
		{"EPS", OptFloat(p, q.EPS)},
		{"Beta", OptFloat(p, q.Beta)},
		{"Div Yield", OptPercent(p, q.DivYield)},
	}
}

// Duration formats a countdown compactly: "45s", "4m", "1h05m".
func Duration(d time.Duration) string {
	d = max(d, 0).Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}
