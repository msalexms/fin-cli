// Package cli implements the Cobra subcommands and the neofetch-like
// renderer used by `fin-cli quote`.
package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"fin-cli/internal/chart"
	"fin-cli/internal/domain"
	"fin-cli/internal/locale"
)

// Exit code convention (used by `fin-cli quote` and friends):
//
//	0 OK
//	1 generic error
//	2 usage / invalid input
//	3 network / provider unavailable
//	4 config / auth
//	5 ticker not found
const (
	ExitOK       = 0
	ExitGeneric  = 1
	ExitUsage    = 2
	ExitNetwork  = 3
	ExitConfig   = 4
	ExitNotFound = 5
)

// ExitCodeFor returns the exit code appropriate for err.
func ExitCodeFor(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, domain.ErrNotFound):
		return ExitNotFound
	case errors.Is(err, domain.ErrInvalidInput):
		return ExitUsage
	case errors.Is(err, domain.ErrUnauthorized),
		errors.Is(err, domain.ErrNoAPIKey):
		return ExitConfig
	case errors.Is(err, domain.ErrNetwork),
		errors.Is(err, domain.ErrUnavailable),
		errors.Is(err, domain.ErrRateLimited):
		return ExitNetwork
	default:
		return ExitGeneric
	}
}

// ANSI (truecolor) codes. We avoid lipgloss in the one-shot renderer so the
// output is predictable and NO_COLOR is honored cleanly.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiGreen  = "\x1b[38;2;78;154;6m"    // #4E9A06
	ansiRed    = "\x1b[38;2;204;0;0m"     // #CC0000
	ansiLabel  = "\x1b[38;2;128;128;128m" // #808080
	ansiSubtle = "\x1b[38;2;48;48;48m"    // #303030
)

// RenderOptions controls how RenderQuote formats the output.
type RenderOptions struct {
	NoColor     bool
	ASCIIOnly   bool
	Width       int
	ChartHeight int
	Printer     locale.Printer
}

// RenderQuote writes a neofetch-like snapshot of q, optionally followed by a
// chart drawn from candles.
//
// Provenance badge:
//
//	[*] fresh from provider, full data
//	[!] fresh but partial (some fields missing)
//	[~] served from cache (stale)
func RenderQuote(w io.Writer, q domain.Quote, candles []domain.Candle, opt RenderOptions) {
	if opt.Width <= 0 {
		opt.Width = 80
	}
	if opt.ChartHeight <= 0 {
		opt.ChartHeight = 12
	}
	p := opt.Printer

	// --- title line ---
	badge, provider := sourceBadge(q)
	badgeColored := opt.colorize(badgeColor(q), badge)
	title := opt.colorize(ansiBold, "▌ "+string(q.Symbol))
	name := ""
	if q.Name != "" {
		name = "  " + opt.colorize(ansiLabel, q.Name)
	}
	right := badgeColored + " " + opt.colorize(ansiLabel, provider)
	fmt.Fprintln(w, pad(title+name, right, opt.Width))
	fmt.Fprintln(w, opt.colorize(ansiSubtle, repeat("─", opt.Width)))

	// --- price line ---
	priceStr := opt.colorize(ansiBold, p.Sprintf("%.2f", q.Price))
	cur := q.Currency
	arrow, body := changeParts(q, opt.ASCIIOnly)
	color := ansiLabel
	if q.Change > 0 {
		color = ansiGreen
	} else if q.Change < 0 {
		color = ansiRed
	}
	change := opt.colorize(color, arrow+" "+body)
	sessTag := ""
	if s := sessionTag(q); s != "" {
		sessTag = "   " + opt.colorize(ansiLabel, "· "+s)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s    %s%s\n", priceStr, opt.colorize(ansiLabel, cur), change, sessTag)
	fmt.Fprintln(w)

	// --- stats grid ---
	renderGrid(w, statsRows(q, p), opt)

	// --- meta line ---
	if meta := metaLine(q); meta != "" {
		fmt.Fprintln(w, opt.colorize(ansiSubtle, repeat("─", opt.Width)))
		fmt.Fprintf(w, "  %s\n", opt.colorize(ansiLabel, meta))
	}

	// --- chart ---
	if len(candles) > 0 {
		series := make(chart.Series, 0, len(candles))
		for _, c := range candles {
			series = append(series, c.Close)
		}
		r := chart.ASCII{Caption: fmt.Sprintf("%d-session close", len(series))}
		plot := r.Render(series, opt.Width-4, opt.ChartHeight)
		cc := ""
		switch series.Trend() {
		case 1:
			cc = ansiGreen
		case -1:
			cc = ansiRed
		}
		if cc != "" && !opt.NoColor {
			plot = colorizeLines(plot, cc)
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, plot)
	}

	// --- timestamp footer ---
	fmt.Fprintln(w)
	fmt.Fprintln(w, opt.colorize(ansiLabel, "  fetched "+q.FetchedAt.Local().Format(time.RFC3339)))
}

// --- sections ---

func statsRows(q domain.Quote, p locale.Printer) [][2]string {
	return [][2]string{
		{"Prev Close", p.Sprintf("%.2f", q.PrevClose)},
		{"Open", optFloat(p, q.Open)},
		{"Day Range", rangeStrCLI(p, q.DayLow, q.DayHigh)},
		{"52w Range", rangeStrCLI(p, q.Week52Low, q.Week52High)},
		{"Volume", formatVolumeCLI(q.Volume)},
		{"Market Cap", formatMarketCapCLI(q.MarketCap, q.Currency)},
		{"P/E", optFloat(p, q.PE)},
		{"EPS", optFloat(p, q.EPS)},
		{"Beta", optFloat(p, q.Beta)},
		{"Div Yield", optPercentCLI(p, q.DivYield)},
	}
}

func renderGrid(w io.Writer, rows [][2]string, opt RenderOptions) {
	inner := opt.Width - 4
	colW := inner / 2
	if colW < 16 {
		colW = 16
	}
	for i := 0; i < len(rows); i += 2 {
		left := gridCell(rows[i][0], rows[i][1], colW, opt)
		var right string
		if i+1 < len(rows) {
			right = gridCell(rows[i+1][0], rows[i+1][1], colW, opt)
		}
		fmt.Fprintf(w, "  %s  %s\n", left, right)
	}
}

func gridCell(label, value string, w int, opt RenderOptions) string {
	labelW := 11
	valW := w - labelW
	if valW < 1 {
		valW = 1
	}
	return opt.colorize(ansiLabel, padRightStr(label, labelW)) + " " + padRightStr(value, valW-1)
}

func metaLine(q domain.Quote) string {
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
	return strings.Join(parts, " · ")
}

// --- helpers ---

func sourceBadge(q domain.Quote) (string, string) {
	switch q.Source {
	case domain.SourceCache:
		return "[~]", "cached " + q.FetchedAt.Local().Format("15:04")
	case domain.SourceFinnhub:
		if q.Partial {
			return "[!]", "via finnhub (partial)"
		}
		return "[*]", "via finnhub"
	}
	return "[*]", "via " + string(q.Source)
}

func badgeColor(q domain.Quote) string {
	switch q.Source {
	case domain.SourceCache:
		return ansiLabel
	case domain.SourceFinnhub:
		if q.Partial {
			return ansiRed
		}
		return ansiGreen
	}
	return ansiLabel
}

func sessionTag(q domain.Quote) string {
	switch q.Session {
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

func changeParts(q domain.Quote, asciiOnly bool) (arrow, body string) {
	arrow = "•"
	if q.Change > 0 {
		arrow = "▲"
	} else if q.Change < 0 {
		arrow = "▼"
	}
	if asciiOnly {
		switch arrow {
		case "▲":
			arrow = "^"
		case "▼":
			arrow = "v"
		default:
			arrow = "."
		}
	}
	body = fmt.Sprintf("%+.2f (%+.2f%%)", q.Change, q.ChangePct)
	return
}

type numPrinter interface {
	Sprintf(string, ...any) string
}

func optFloat(p numPrinter, o domain.Optional[float64]) string {
	if !o.Valid {
		return "—"
	}
	return p.Sprintf("%.2f", o.Value)
}

func optPercentCLI(p numPrinter, o domain.Optional[float64]) string {
	if !o.Valid {
		return "—"
	}
	return p.Sprintf("%.2f%%", o.Value)
}

func rangeStrCLI(p numPrinter, lo, hi domain.Optional[float64]) string {
	if !lo.Valid || !hi.Valid {
		return "—"
	}
	return p.Sprintf("%.2f – %.2f", lo.Value, hi.Value)
}

func formatVolumeCLI(v domain.Optional[int64]) string {
	if !v.Valid {
		return "—"
	}
	n := v.Value
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

func formatMarketCapCLI(m domain.Optional[float64], cur string) string {
	if !m.Valid {
		return "—"
	}
	v := m.Value
	var out string
	switch {
	case v >= 1_000_000:
		out = fmt.Sprintf("%.2fT", v/1_000_000)
	case v >= 1_000:
		out = fmt.Sprintf("%.2fB", v/1_000)
	default:
		out = fmt.Sprintf("%.2fM", v)
	}
	if cur != "" {
		out += " " + cur
	}
	return out
}

// --- primitives ---

// visibleLen returns the display width of s ignoring ANSI SGR escapes.
func visibleLen(s string) int {
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

func pad(left, right string, width int) string {
	lw := visibleLen(left)
	rw := visibleLen(right)
	g := width - lw - rw
	if g < 1 {
		g = 1
	}
	return left + repeat(" ", g) + right
}

func padRightStr(s string, n int) string {
	w := visibleLen(s)
	if w >= n {
		return s
	}
	return s + repeat(" ", n-w)
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}

// --- color ---

func (opt RenderOptions) colorize(code, s string) string {
	if opt.NoColor {
		return s
	}
	return code + s + ansiReset
}

func colorizeLines(s, code string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = code + l + ansiReset
	}
	return strings.Join(lines, "\n")
}
