package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"

	"github.com/msalexms/fin-cli/internal/chart"
	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/format"
	"github.com/msalexms/fin-cli/internal/locale"
	"github.com/msalexms/fin-cli/internal/theme"
)

// RenderOptions controls RenderQuote.
type RenderOptions struct {
	Printer locale.Printer
	Width   int // total columns; 0 means 80
}

// RenderQuote writes a snapshot of q followed by a chart of candles. Colors
// are only emitted when w is a color-capable terminal and NO_COLOR is unset.
func RenderQuote(w io.Writer, q domain.Quote, candles []domain.Candle, opt RenderOptions) {
	width := opt.Width
	if width <= 0 {
		width = 80
	}
	r := lipgloss.NewRenderer(w)
	st := theme.New(r)
	p := opt.Printer
	ascii := p.ASCIIOnly()
	rule := st.Line.Render(strings.Repeat(lineChar(ascii), width))

	// Title: symbol + name on the left, provenance on the right.
	left := st.Accent.Render(mark(ascii)+" ") + st.Strong.Render(string(q.Symbol))
	if q.Name != "" {
		left += "  " + st.Text2.Render(q.Name)
	}
	prov := st.Muted
	if q.Stale {
		prov = st.Warn
	}
	fmt.Fprintln(w, spread(left, prov.Render(format.Provenance(q)), width))
	fmt.Fprintln(w, rule)

	// Price line.
	change := st.Change(q.Change).Render(format.ChangeArrow(q.Change, ascii) + " " + format.Change(p, q.Price, q.Change, q.ChangePct))
	line := "  " + st.Strong.Render(format.Price(p, q.Price))
	if q.Currency != "" {
		line += " " + st.Muted.Render(q.Currency)
	}
	line += "   " + change
	if s := format.SessionLabel(q.Session); s != "" {
		line += "   " + st.Muted.Render("· "+s)
	}
	fmt.Fprintf(w, "\n%s\n\n", line)

	// Stats grid, two columns.
	rows := format.StatsRows(q, p)
	colW := max((width-4)/2, 24)
	for i := 0; i < len(rows); i += 2 {
		cell := func(r [2]string) string {
			return st.Muted.Render(padTo(r[0], 12)) + st.Text.Render(padTo(r[1], colW-12))
		}
		row := "  " + cell(rows[i])
		if i+1 < len(rows) {
			row += cell(rows[i+1])
		}
		fmt.Fprintln(w, strings.TrimRight(row, " "))
	}

	if meta := format.MetaLine(q); meta != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  "+st.Muted.Render(meta))
	}

	if len(candles) > 1 {
		series := make(chart.Series, len(candles))
		for i, c := range candles {
			series[i] = c.Close
		}
		plot := chart.Line(series, width-2, 10, "")
		style := st.Change(float64(series.Trend()))
		fmt.Fprintln(w)
		for _, l := range strings.Split(plot, "\n") {
			fmt.Fprintln(w, "  "+style.Render(l))
		}
		span := candles[0].Time.Format("Jan 2") + " – " + candles[len(candles)-1].Time.Format("Jan 2")
		fmt.Fprintln(w, "  "+st.Muted.Render(fmt.Sprintf("%d sessions · %s", len(candles), span)))
	}

	fmt.Fprintln(w)
	foot := "as of " + q.AsOf.Local().Format("2006-01-02 15:04")
	if q.Stale {
		foot += " · providers unreachable, showing the last cached quote"
	}
	fmt.Fprintln(w, "  "+st.Muted.Render(foot))
}

// termWidth returns the terminal width for w, capped to keep output readable.
func termWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		if cols, _, err := term.GetSize(f.Fd()); err == nil && cols > 0 {
			return min(cols, 100)
		}
	}
	return 80
}

func spread(left, right string, width int) string {
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 2)
	return left + strings.Repeat(" ", gap) + right
}

func padTo(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s + " "
}

func mark(ascii bool) string {
	if ascii {
		return ">"
	}
	return "▌"
}

func lineChar(ascii bool) string {
	if ascii {
		return "-"
	}
	return "─"
}
