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
	"fin-cli/internal/format"
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
	badge, provider := format.SourceBadge(q)
	badgeColored := opt.colorize(badgeColor(q), badge)
	title := opt.colorize(ansiBold, "▌ "+string(q.Symbol))
	name := ""
	if q.Name != "" {
		name = "  " + opt.colorize(ansiLabel, q.Name)
	}
	right := badgeColored + " " + opt.colorize(ansiLabel, provider)
	fmt.Fprintln(w, pad(title+name, right, opt.Width))
	fmt.Fprintln(w, opt.colorize(ansiSubtle, format.Repeat("─", opt.Width)))

	// --- price line ---
	priceStr := opt.colorize(ansiBold, p.Sprintf("%.2f", q.Price))
	cur := q.Currency
	arrow := format.ChangeArrow(q.Change, opt.ASCIIOnly)
	body := format.ChangeBody(p, q.Change, q.ChangePct)
	color := ansiLabel
	if q.Change > 0 {
		color = ansiGreen
	} else if q.Change < 0 {
		color = ansiRed
	}
	change := opt.colorize(color, arrow+" "+body)
	sessTag := ""
	if s := format.SessionLabel(q.Session); s != "" {
		sessTag = "   " + opt.colorize(ansiLabel, "· "+s)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s    %s%s\n", priceStr, opt.colorize(ansiLabel, cur), change, sessTag)
	fmt.Fprintln(w)

	// --- stats grid ---
	renderGrid(w, format.StatsRows(q, p), opt)

	// --- meta line ---
	if meta := format.MetaLine(q); meta != "" {
		fmt.Fprintln(w, opt.colorize(ansiSubtle, format.Repeat("─", opt.Width)))
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

// --- helpers ---

func badgeColor(q domain.Quote) string {
	switch q.Source {
	case domain.SourceCache:
		return ansiLabel
	default:
		if q.Partial {
			return ansiRed
		}
		return ansiGreen
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
	return opt.colorize(ansiLabel, format.PadRightANSI(label, labelW)) + " " + format.PadRightANSI(value, valW-1)
}

// --- color ---

func pad(left, right string, width int) string {
	lw := format.VisibleLen(left)
	rw := format.VisibleLen(right)
	g := width - lw - rw
	if g < 1 {
		g = 1
	}
	return left + format.Repeat(" ", g) + right
}

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
