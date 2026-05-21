// Package format provides shared number and string formatting helpers
// used by both the TUI and CLI renderers.
package format

import (
	"fmt"
	"strings"

	"fin-cli/internal/domain"
)

// Printer is any type that supports locale-aware Sprintf (e.g. locale.Printer).
type Printer interface {
	Sprintf(string, ...any) string
}

// OptFloat formats an optional float64, returning "—" if not valid.
func OptFloat(p Printer, o domain.Optional[float64]) string {
	if !o.Valid {
		return "\u2014"
	}
	return p.Sprintf("%.2f", o.Value)
}

// OptPercent formats an optional float64 as a percentage, returning "—" if not valid.
func OptPercent(p Printer, o domain.Optional[float64]) string {
	if !o.Valid {
		return "\u2014"
	}
	return p.Sprintf("%.2f%%", o.Value)
}

// RangeStr formats a low–high range from two optionals, returning "—" if either is missing.
func RangeStr(p Printer, lo, hi domain.Optional[float64]) string {
	if !lo.Valid || !hi.Valid {
		return "\u2014"
	}
	return p.Sprintf("%.2f \u2013 %.2f", lo.Value, hi.Value)
}

// Volume formats an optional int64 volume into a human-readable string (K/M/B).
func Volume(v domain.Optional[int64]) string {
	if !v.Valid {
		return "\u2014"
	}
	return HumanizeInt(v.Value)
}

// MarketCap formats an optional float64 market cap (expressed in millions) with currency.
func MarketCap(m domain.Optional[float64], cur string) string {
	if !m.Valid {
		return "\u2014"
	}
	out := HumanizeMillions(m.Value)
	if cur != "" {
		out += " " + cur
	}
	return out
}

// HumanizeMillions converts a value expressed in millions into a compact string (M/B/T).
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

// HumanizeInt converts a raw integer into a compact string (K/M/B).
func HumanizeInt(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.2fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.2fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// ChangeArrow returns the direction glyph for a price change.
// If asciiOnly is true, uses ^/v/. instead of Unicode arrows.
func ChangeArrow(change float64, asciiOnly bool) string {
	arrow := "\u2022" // •
	if change > 0 {
		arrow = "\u25B2" // ▲
	} else if change < 0 {
		arrow = "\u25BC" // ▼
	}
	if asciiOnly {
		switch arrow {
		case "\u25B2":
			return "^"
		case "\u25BC":
			return "v"
		default:
			return "."
		}
	}
	return arrow
}

// ChangeBody formats the absolute change and percent change.
func ChangeBody(p Printer, change, changePct float64) string {
	return p.Sprintf("%+.2f (%+.2f%%)", change, changePct)
}

// SessionLabel returns a human-readable label for the market session.
func SessionLabel(s domain.MarketSession) string {
	switch s {
	case domain.SessionPre:
		return "pre-market"
	case domain.SessionRegular:
		return "regular"
	case domain.SessionPost:
		return "after-hours"
	case domain.SessionClosed:
		return "closed"
	}
	return ""
}

// SourceBadge returns the badge text and provider description for a quote's source.
func SourceBadge(q domain.Quote) (badge, provider string) {
	switch q.Source {
	case domain.SourceCache:
		return "[~]", "cached " + q.FetchedAt.Local().Format("15:04")
	default:
		if q.Partial {
			return "[!]", "via " + string(q.Source) + " (partial)"
		}
		return "[*]", "via " + string(q.Source)
	}
}

// MetaLine builds the exchange/industry/country/IPO metadata line.
func MetaLine(q domain.Quote) string {
	parts := []string{}
	if q.Exchange != "" {
		parts = append(parts, q.Exchange)
	}
	if q.Industry != "" {
		parts = append(parts, q.Industry)
	}
	if q.Country != "" {
		parts = append(parts, q.Country)
	}
	if q.IPODate != "" {
		parts = append(parts, "IPO "+q.IPODate)
	}
	return strings.Join(parts, " \u00B7 ")
}

// StatsRows builds the standard two-column stats data for a quote.
func StatsRows(q domain.Quote, p Printer) [][2]string {
	return [][2]string{
		{"Prev Close", p.Sprintf("%.2f", q.PrevClose)},
		{"Open", OptFloat(p, q.Open)},
		{"Day Range", RangeStr(p, q.DayLow, q.DayHigh)},
		{"52w Range", RangeStr(p, q.Week52Low, q.Week52High)},
		{"Volume", Volume(q.Volume)},
		{"Market Cap", MarketCap(q.MarketCap, q.Currency)},
		{"P/E", OptFloat(p, q.PE)},
		{"EPS", OptFloat(p, q.EPS)},
		{"Beta", OptFloat(p, q.Beta)},
		{"Div Yield", OptPercent(p, q.DivYield)},
	}
}

// ShortDuration formats a duration as "Xs" or "Xm" for display.
func ShortDuration(d interface{ Seconds() float64; Minutes() float64 }) string {
	secs := d.Seconds()
	if secs < 60 {
		return fmt.Sprintf("%ds", int(secs))
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

// VisibleLen returns the display width of s ignoring ANSI SGR escape sequences.
func VisibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}

// PadRightANSI pads s to width n using VisibleLen (ANSI-aware).
func PadRightANSI(s string, n int) string {
	w := VisibleLen(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

// Repeat returns s repeated n times, or empty if n <= 0.
func Repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}
