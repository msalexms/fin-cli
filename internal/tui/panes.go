package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/msalexms/fin-cli/internal/chart"
	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/format"
)

const sparkWidth = 10

// viewList renders the watchlist: cursor, symbol, 10-session sparkline and
// daily % change.
func (m *Model) viewList(w, h int) string {
	st := m.st
	order := m.order()
	sel := m.selectedIndex(order)

	title := st.Label.Render("WATCHLIST")
	if mode := m.sortMode(); mode != "manual" {
		title += st.Muted.Render(" · " + sortLabel(mode))
	}
	lines := []string{title, ""}

	visible := max(h-3, 1)
	offset := 0
	if sel >= visible {
		offset = sel - visible + 1
	}
	end := min(offset+visible, len(order))
	for i := offset; i < end; i++ {
		lines = append(lines, m.listRow(order[i], i == sel, w))
	}
	if len(order) > visible {
		lines = append(lines, st.Muted.Render(fmt.Sprintf("  %d/%d", sel+1, len(order))))
	}
	return fit(strings.Join(lines, "\n"), w, h)
}

func (m *Model) listRow(t domain.Ticker, selected bool, w int) string {
	st := m.st
	e := m.data[t]

	cursor, sym := "  ", st.Text.Render
	if selected {
		cursor = st.Accent.Render(m.glyph("▸", ">")) + " "
		sym = st.Accent.Render
	}

	var right string
	switch {
	case e == nil || (!e.hasQuote && e.loading):
		right = m.spin.View()
	case !e.hasQuote && e.err != nil:
		right = st.Down.Render("error")
	case e.hasQuote:
		right = st.Change(e.quote.ChangePct).Render(format.Percent(m.printer, e.quote.ChangePct))
		if e.err != nil || e.quote.Stale {
			right = st.Warn.Render("!") + " " + right
		}
	}
	const rightW = 9

	spark := ""
	sw := 0
	if w >= 30 && e != nil && len(e.candles) > 1 && !m.ascii {
		sw = sparkWidth
		series := closes(e.candles)
		spark = st.Change(float64(series.Trend())).Render(chart.Spark(lastN(series, sparkWidth*2), sparkWidth))
	}
	symW := max(w-2-rightW-sw-1, 4)
	row := cursor + sym(pad(string(t), symW-1)) + " "
	if sw > 0 {
		row += spark + " "
	}
	return row + padLeft(right, rightW)
}

// viewDetail renders the selected instrument. compact adds a position line
// because the list is hidden on narrow terminals.
func (m *Model) viewDetail(w, h int, compact bool) string {
	st := m.st
	t := m.cursor
	e := m.data[t]
	var lines []string
	if compact {
		order := m.order()
		lines = append(lines, st.Muted.Render(fmt.Sprintf("%d of %d · ↑↓ to switch", m.selectedIndex(order)+1, len(order))), "")
	}

	switch {
	case e == nil || (!e.hasQuote && e.err == nil):
		lines = append(lines, m.spin.View()+st.Muted.Render(" Loading "+string(t)+"…"))
		return fit(strings.Join(lines, "\n"), w, h)
	case !e.hasQuote:
		lines = append(lines,
			st.Strong.Render(string(t)), "",
			st.Down.Render(explain(e.err)), "",
			st.Muted.Render("Press ")+st.Key.Render("r")+st.Muted.Render(" to retry or ")+st.Key.Render("d")+st.Muted.Render(" to remove it."),
		)
		return fit(strings.Join(lines, "\n"), w, h)
	}

	q := e.quote
	rule := st.Line.Render(strings.Repeat(m.glyph("─", "-"), w))

	// Title: the name gives way first when space is short.
	prov := st.Muted.Render(format.Provenance(q))
	if q.Stale {
		prov = st.Warn.Render(format.Provenance(q))
	}
	if e.loading {
		prov = m.spin.View() + " " + prov
	}
	left := st.Accent.Render(m.glyph("▌ ", "| ")) + st.Strong.Render(string(q.Symbol))
	if nameW := w - lipgloss.Width(left) - lipgloss.Width(prov) - 4; q.Name != "" && nameW >= 6 {
		left += "  " + st.Text2.Render(truncate(q.Name, nameW))
	}
	lines = append(lines, spread(left, prov, w), rule, "")

	// Price.
	price := "  " + st.Strong.Render(m.price(q.Price))
	if q.Currency != "" {
		price += " " + st.Muted.Render(q.Currency)
	}
	price += "   " + st.Change(q.Change).Render(format.ChangeArrow(q.Change, m.ascii)+" "+format.Change(m.printer, q.Price, q.Change, q.ChangePct))
	if s := format.SessionLabel(q.Session); s != "" {
		price += "   " + st.Muted.Render("· "+s)
	}
	lines = append(lines, price, "")

	// Stats grid: two columns when there is room.
	rows := format.StatsRows(q, m.printer)
	valW := 0
	for _, r := range rows {
		valW = max(valW, lipgloss.Width(r[1]))
	}
	cols := 1
	if 2*(12+valW+2) <= w-2 {
		cols = 2
	} else {
		// One column: skip what the provider did not supply to leave room for the chart.
		rows = slices.DeleteFunc(rows, func(r [2]string) bool { return r[1] == format.Dash })
	}
	for i := 0; i < len(rows); i += cols {
		line := "  "
		for c := 0; c < cols && i+c < len(rows); c++ {
			r := rows[i+c]
			line += st.Muted.Render(pad(r[0], 12)) + st.Text.Render(pad(r[1], valW+2))
		}
		lines = append(lines, strings.TrimRight(line, " "))
	}
	if meta := format.MetaLine(q); meta != "" {
		lines = append(lines, "", "  "+st.Muted.Render(meta))
	}
	if e.err != nil {
		lines = append(lines, "", st.Warn.Render("  ! Last refresh failed: "+explain(e.err)))
	}

	// Chart fills the remaining height.
	chartH := h - len(lines) - 3
	if len(e.candles) > 1 && chartH >= 3 {
		series := closes(e.candles)
		style := st.Change(float64(series.Trend()))
		lines = append(lines, "")
		for _, l := range strings.Split(chart.Line(series, w-2, chartH, ""), "\n") {
			lines = append(lines, "  "+style.Render(l))
		}
		first, last := e.candles[0].Time, e.candles[len(e.candles)-1].Time
		lines = append(lines, "  "+st.Muted.Render(fmt.Sprintf("%d sessions · %s – %s", len(e.candles), first.Format("Jan 2"), last.Format("Jan 2"))))
	} else if e.loading && chartH >= 2 {
		lines = append(lines, "", "  "+m.spin.View()+st.Muted.Render(" Loading chart…"))
	} else if chartH >= 2 {
		lines = append(lines, "", "  "+st.Muted.Render("No price history available for the chart."))
	}
	return fit(strings.Join(lines, "\n"), w, h)
}

func closes(cs []domain.Candle) chart.Series {
	s := make(chart.Series, len(cs))
	for i, c := range cs {
		s[i] = c.Close
	}
	return s
}

func lastN(s chart.Series, n int) chart.Series {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

func padLeft(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return strings.Repeat(" ", n-w) + s
	}
	return s
}
