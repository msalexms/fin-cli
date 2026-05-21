package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"fin-cli/internal/chart"
	"fin-cli/internal/domain"
	"fin-cli/internal/format"
)

func (m *Model) renderDetail(w, h int) string {
	st := m.styles
	if len(m.tickers) == 0 {
		return st.PaneBorder.Width(w - 2).Height(h - 2).Render("")
	}
	t := m.tickers[m.selected]
	inner := w - 4
	innerH := h - 2
	body := m.detailBody(t, inner, innerH)
	return st.PaneBorder.Width(w - 2).Height(h - 2).Render(body)
}

func (m *Model) detailBody(t domain.Ticker, w, h int) string {
	st := m.styles
	if m.loading[t] {
		return st.Label.Render(m.sp.View() + " loading " + string(t) + " \u2026")
	}
	if err, ok := m.errs[t]; ok && err != nil {
		return m.renderErrorDetail(t, err, w)
	}
	q, ok := m.quotes[t]
	if !ok {
		return st.Label.Render("no data yet")
	}

	sep := st.Subtle.Render(strings.Repeat("\u2500", w))
	header := m.detailTitleLine(q, w)
	price := m.detailPriceLine(q)
	grid := m.detailStatsGrid(q, w)
	meta := m.detailMetaLine(q, w)

	sections := []string{header, sep, "", price, "", grid, sep, meta}
	fixed := strings.Join(sections, "\n")
	fixedH := strings.Count(fixed, "\n") + 1
	chartH := h - fixedH - 1
	if chartH < 3 {
		return fixed
	}
	ch := m.detailChart(t, w, chartH)
	return fixed + "\n\n" + ch
}

func (m *Model) detailTitleLine(q domain.Quote, w int) string {
	st := m.styles
	title := st.Title.Render("\u258C " + string(q.Symbol))
	name := ""
	if q.Name != "" {
		name = st.Base.Render(" ") + st.Label.Render(q.Name)
	}
	left := title + name

	badge, provider := m.sourceBadge(q)
	right := badge + st.Base.Render(" ") + st.Label.Render(provider)

	pad := w - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return left + st.Base.Render(strings.Repeat(" ", pad)) + right
}

func (m *Model) detailPriceLine(q domain.Quote) string {
	st := m.styles
	p := m.deps.Printer

	priceStr := p.Sprintf("%.2f", q.Price)
	cur := q.Currency
	if cur == "" {
		cur = "\u2014"
	}
	price := st.Big.Render(priceStr) + st.Base.Render(" ") + st.Label.Render(cur)

	arrow, changeStr := m.formatChange(q)
	change := m.colorizeChange(arrow+" "+changeStr, q.Change)

	sess := m.sessionLabel(q)
	parts := []string{st.Base.Render("  ") + price, change}
	if sess != "" {
		parts = append(parts, st.Label.Render("\u00B7 "+sess))
	}
	sep3 := st.Base.Render("   ")
	return joinWith(parts, sep3)
}

func (m *Model) detailStatsGrid(q domain.Quote, w int) string {
	rows := format.StatsRows(q, m.deps.Printer)
	return m.twoColGrid(rows, w)
}

func (m *Model) twoColGrid(rows [][2]string, w int) string {
	st := m.styles
	colW := (w - 4) / 2
	if colW < 16 {
		colW = 16
	}
	var out []string
	for i := 0; i < len(rows); i += 2 {
		left := m.gridCell(st, rows[i][0], rows[i][1], colW)
		var right string
		if i+1 < len(rows) {
			right = m.gridCell(st, rows[i+1][0], rows[i+1][1], colW)
		} else {
			right = st.Base.Render(strings.Repeat(" ", colW))
		}
		out = append(out, st.Base.Render("  ")+left+st.Base.Render("  ")+right)
	}
	return strings.Join(out, "\n")
}

func (m *Model) gridCell(st Styles, label, value string, w int) string {
	labelW := 11
	valW := w - labelW
	if valW < 1 {
		valW = 1
	}
	return st.Label.Render(padRight(label, labelW)) + st.Base.Render(" ") + st.Base.Render(padRight(value, valW-1))
}

func (m *Model) detailMetaLine(q domain.Quote, w int) string {
	_ = w
	st := m.styles
	parts := format.MetaLine(q)
	if parts == "" {
		return ""
	}
	return st.Base.Render("  ") + st.Label.Render(parts)
}

func (m *Model) detailChart(t domain.Ticker, w, h int) string {
	candles, ok := m.candles[t]
	if !ok || len(candles) == 0 {
		return m.styles.Label.Render("  (no historical data available)")
	}
	series := make(chart.Series, 0, len(candles))
	for _, c := range candles {
		series = append(series, c.Close)
	}
	var r chart.Renderer
	if h >= 4 && !m.deps.ASCIIOnly {
		r = chart.ASCII{Caption: fmt.Sprintf("%d-session close", len(series))}
	} else {
		r = chart.Blocks{}
	}
	plot := r.Render(series, w-10, h-2)

	style := m.styles.Neutral
	switch series.Trend() {
	case 1:
		style = m.styles.Up
	case -1:
		style = m.styles.Down
	}
	lines := strings.Split(plot, "\n")
	for i, l := range lines {
		lines[i] = style.Render(l)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) renderErrorDetail(t domain.Ticker, err error, w int) string {
	_ = w
	st := m.styles
	return st.Down.Render("! ") + st.Base.Render(string(t)+": "+explainError(err))
}

// --- helpers ---

func (m *Model) sourceBadge(q domain.Quote) (string, string) {
	st := m.styles
	badge, provider := format.SourceBadge(q)
	var styledBadge string
	switch q.Source {
	case domain.SourceCache:
		styledBadge = st.BadgeCache.Render(badge)
	default:
		if q.Partial {
			styledBadge = st.BadgePartial.Render(badge)
		} else {
			styledBadge = st.BadgeFresh.Render(badge)
		}
	}
	return styledBadge, provider
}

func (m *Model) sessionLabel(q domain.Quote) string {
	return format.SessionLabel(q.Session)
}

func (m *Model) formatChange(q domain.Quote) (arrow, body string) {
	arrow = format.ChangeArrow(q.Change, m.deps.ASCIIOnly)
	body = format.ChangeBody(m.deps.Printer, q.Change, q.ChangePct)
	return
}

func (m *Model) colorizeChange(s string, change float64) string {
	switch {
	case change > 0:
		return m.styles.Up.Render(s)
	case change < 0:
		return m.styles.Down.Render(s)
	default:
		return m.styles.Neutral.Render(s)
	}
}

func explainError(err error) string {
	switch {
	case errors.Is(err, domain.ErrNoAPIKey):
		return "no api key configured; run `fin-cli config set finnhub.api_key <KEY>`"
	case errors.Is(err, domain.ErrUnauthorized):
		return "provider rejected the api key"
	case errors.Is(err, domain.ErrRateLimited):
		return "rate limited; try again shortly"
	case errors.Is(err, domain.ErrUnavailable):
		return "provider unavailable"
	case errors.Is(err, domain.ErrNetwork):
		return "network error"
	case errors.Is(err, domain.ErrNotFound):
		return "not found"
	default:
		return err.Error()
	}
}
