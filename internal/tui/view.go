package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"fin-cli/internal/chart"
	"fin-cli/internal/domain"
)

// View renders the current state. It is called on every tick.
func (m *Model) View() string {
	if !m.ready {
		return ""
	}
	if m.width < 24 || m.height < 8 {
		return m.styles.Base.Width(m.width).Height(m.height).
			Render("terminal too small")
	}

	header := m.renderHeader(m.width)
	footer := m.renderFooter(m.width)
	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyH < 4 {
		bodyH = 4
	}
	body := m.renderBody(m.width, bodyH)

	composed := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	// Root wrapper: forces the entire terminal rectangle to the palette
	// background, eliminating any "default terminal bg" bleed-through from
	// JoinVertical/Horizontal padding or sub-component gaps.
	return lipgloss.NewStyle().
		Background(m.styles.P.Bg).
		Foreground(m.styles.P.Base).
		Width(m.width).
		Height(m.height).
		Render(composed)
}

// --------- header bar ---------

func (m *Model) renderHeader(w int) string {
	st := m.styles
	title := st.Title.Render("▌ fin-cli")
	count := st.Label.Render(fmt.Sprintf("%d tickers", len(m.tickers)))
	clock := st.Label.Render(time.Now().Format("15:04:05"))
	sep := st.HelpSep.Render(" · ")
	center := count + sep + m.nextPollText()

	used := lipgloss.Width(title) + lipgloss.Width(center) + lipgloss.Width(clock)
	pad := w - used - 2 // account for HeaderBar padding
	if pad < 2 {
		pad = 2
	}
	left := pad / 2
	right := pad - left
	// Pre-bg-wrap each plain-whitespace region so the embedded ANSI resets
	// from the inner Renders never leave a terminal-default gap in the middle
	// of the bar.
	lfill := st.Base.Render(strings.Repeat(" ", left))
	rfill := st.Base.Render(strings.Repeat(" ", right))
	content := title + lfill + center + rfill + clock
	return st.HeaderBar.Width(w).Render(content)
}

func (m *Model) nextPollText() string {
	next := m.lastTick.Add(m.pollInterval())
	remaining := time.Until(next)
	if remaining < 0 {
		remaining = 0
	}
	return m.styles.Label.Render(fmt.Sprintf("next poll %s", shortDuration(remaining)))
}

// --------- footer ---------

func (m *Model) renderFooter(w int) string {
	st := m.styles

	// 1) Input mode: show the prompt and instructions.
	if m.mode == modeAdd {
		prompt := m.input.View()
		hint := st.Label.Render("enter: add  ·  esc: cancel")
		if m.busy {
			hint = st.Label.Render(m.sp.View() + " validating…")
		}
		if m.status != "" {
			st2 := st.Up
			if !m.statusOK {
				st2 = st.Down
			}
			hint = st2.Render(m.status)
		}
		// "Add ticker: <input>    <hint>"
		label := st.Label.Render("add: ")
		line := label + prompt + st.Base.Render("   ") + hint
		return st.FooterBar.Width(w).Render(line)
	}

	if m.globalErr != nil {
		return st.FooterBar.Width(w).Render(
			st.Down.Render("! ") + st.Base.Render(m.globalErr.Error()),
		)
	}

	// 2) Transient status (e.g. "added AAPL").
	if m.status != "" {
		col := st.Up
		if !m.statusOK {
			col = st.Down
		}
		return st.FooterBar.Width(w).Render(col.Render(m.status))
	}

	// 3) Default help line.
	sep := st.HelpSep.Render("  ·  ")
	sp := st.Base.Render(" ")
	parts := []string{
		st.HelpKey.Render("↑/k") + sp + st.HelpDesc.Render("up"),
		st.HelpKey.Render("↓/j") + sp + st.HelpDesc.Render("down"),
		st.HelpKey.Render("r") + sp + st.HelpDesc.Render("refresh"),
		st.HelpKey.Render("a") + sp + st.HelpDesc.Render("add"),
		st.HelpKey.Render("d") + sp + st.HelpDesc.Render("del"),
		st.HelpKey.Render("q") + sp + st.HelpDesc.Render("quit"),
	}
	return st.FooterBar.Width(w).Render(joinWith(parts, sep))
}

// --------- body: split or collapsed ---------

func (m *Model) renderBody(w, h int) string {
	if len(m.tickers) == 0 {
		return m.renderEmpty(w, h)
	}
	if w < 50 {
		return m.renderCollapsed(w, h)
	}

	// Panes are placed flush (no gap) to avoid unstyled background between
	// them. Their rounded borders act as the visual separator.
	left := w / 4
	if left < 14 {
		left = 14
	}
	right := w - left
	if right < 36 {
		right = 36
		left = w - right
	}

	sidebar := m.renderSidebar(left, h)
	detail := m.renderDetail(right, h)
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, detail)
}

func (m *Model) renderEmpty(w, h int) string {
	st := m.styles
	msg := lipgloss.JoinVertical(lipgloss.Center,
		st.Label.Render("watchlist is empty"),
		st.Label.Render(""),
		st.Base.Render("press ")+st.Title.Render("a")+st.Base.Render(" to add a ticker"),
	)
	box := st.PaneBorder.Width(w - 2).Height(h - 2).Render(centerVert(msg, h-4))
	return box
}

func (m *Model) renderCollapsed(w, h int) string {
	if len(m.tickers) == 0 {
		return m.renderEmpty(w, h)
	}
	st := m.styles
	t := m.tickers[m.selected]
	nav := st.Label.Render(fmt.Sprintf("%d/%d — ↑/↓ to switch", m.selected+1, len(m.tickers)))
	detail := m.detailBody(t, w-4, h-5)
	return st.PaneBorder.Width(w - 2).Height(h - 2).Render(nav + "\n" + detail)
}

// --------- sidebar ---------

func (m *Model) renderSidebar(w, h int) string {
	st := m.styles
	inner := w - 4 // borders + padding
	if inner < 8 {
		inner = 8
	}

	// Render title and separator at exactly `inner` width so JoinVertical does
	// not pad them with unstyled spaces — which would bleed through as the
	// terminal default background.
	title := st.PaneTitle.Width(inner).Render("WATCHLIST")
	sep := st.Subtle.Width(inner).Render(strings.Repeat("─", inner))

	var rows []string
	for i, t := range m.tickers {
		rows = append(rows, m.renderSidebarRow(t, i == m.selected, inner))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		sep,
		strings.Join(rows, "\n"),
	)
	return st.PaneBorder.Width(w - 2).Height(h - 2).Render(content)
}

func (m *Model) renderSidebarRow(t domain.Ticker, selected bool, width int) string {
	st := m.styles
	var marker, sym string
	if selected {
		marker = st.Title.Render("▌")
		sym = st.SidebarSelected.Render(string(t))
	} else {
		marker = st.Base.Render(" ")
		sym = st.SidebarRow.Render(string(t))
	}

	var right string
	if q, ok := m.quotes[t]; ok {
		right = m.colorizeChange(fmt.Sprintf("%+.2f%%", q.ChangePct), q.Change)
	} else if m.loading[t] {
		right = st.Label.Render(m.sp.View())
	} else if err, ok := m.errs[t]; ok && err != nil {
		right = st.Down.Render("!")
	} else {
		right = ""
	}

	leftW := lipgloss.Width(marker) + 1 + lipgloss.Width(sym)
	rightW := lipgloss.Width(right)
	pad := width - leftW - rightW
	if pad < 1 {
		pad = 1
	}
	gap1 := st.Base.Render(" ")
	gap2 := st.Base.Render(strings.Repeat(" ", pad))
	return marker + gap1 + sym + gap2 + right
}

// --------- detail pane ---------

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
		return st.Label.Render(m.sp.View() + " loading " + string(t) + " …")
	}
	if err, ok := m.errs[t]; ok && err != nil {
		return m.renderErrorDetail(t, err, w)
	}
	q, ok := m.quotes[t]
	if !ok {
		return st.Label.Render("no data yet")
	}

	sep := st.Subtle.Render(strings.Repeat("─", w))
	header := m.detailTitleLine(q, w)
	price := m.detailPriceLine(q)
	grid := m.detailStatsGrid(q, w)
	meta := m.detailMetaLine(q, w)

	// Compose fixed-height sections; compute the remaining room for the chart.
	sections := []string{
		header,
		sep,
		"",
		price,
		"",
		grid,
		sep,
		meta,
	}
	fixed := strings.Join(sections, "\n")
	fixedH := strings.Count(fixed, "\n") + 1
	chartH := h - fixedH - 1 // -1 for the blank line before the chart
	if chartH < 3 {
		return fixed
	}
	chart := m.detailChart(t, w, chartH)
	return fixed + "\n\n" + chart
}

func (m *Model) detailTitleLine(q domain.Quote, w int) string {
	st := m.styles
	title := st.Title.Render("▌ " + string(q.Symbol))
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
	p := m.app.Printer

	priceStr := p.Sprintf("%.2f", q.Price)
	cur := q.Currency
	if cur == "" {
		cur = "—"
	}
	price := st.Big.Render(priceStr) + st.Base.Render(" ") + st.Label.Render(cur)

	arrow, changeStr := m.formatChange(q)
	change := m.colorizeChange(arrow+" "+changeStr, q.Change)

	sess := m.sessionLabel(q)

	parts := []string{st.Base.Render("  ") + price, change}
	if sess != "" {
		parts = append(parts, st.Label.Render("· "+sess))
	}
	// Use a styled separator instead of raw spaces between styled segments.
	sep3 := st.Base.Render("   ")
	return joinWith(parts, sep3)
}

// joinWith joins ss using sep without adding sep at the ends.
func joinWith(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	out := ss[0]
	for i := 1; i < len(ss); i++ {
		out += sep + ss[i]
	}
	return out
}

func (m *Model) detailStatsGrid(q domain.Quote, w int) string {
	p := m.app.Printer
	rows := [][2]string{
		{"Prev Close", p.Sprintf("%.2f", q.PrevClose)},
		{"Open", optFloat(p, q.Open)},
		{"Day Range", rangeStr(p, q.DayLow, q.DayHigh)},
		{"52w Range", rangeStr(p, q.Week52Low, q.Week52High)},
		{"Volume", formatVolume(q.Volume)},
		{"Market Cap", formatMarketCap(q.MarketCap, q.Currency)},
		{"P/E", optFloat(p, q.PE)},
		{"EPS", optFloat(p, q.EPS)},
		{"Beta", optFloat(p, q.Beta)},
		{"Div Yield", optPercent(p, q.DivYield)},
	}
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
	parts := []string{}
	if q.Exchange != "" {
		parts = append(parts, st.Label.Render(q.Exchange))
	}
	if q.Industry != "" {
		parts = append(parts, st.Label.Render(q.Industry))
	}
	if q.Country != "" {
		parts = append(parts, st.Label.Render(q.Country))
	}
	if q.IPODate != "" {
		parts = append(parts, st.Label.Render("IPO "+q.IPODate))
	}
	if len(parts) == 0 {
		return ""
	}
	sep := st.Subtle.Render(" · ")
	return st.Base.Render("  ") + joinWith(parts, sep)
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
	if h >= 4 && !m.app.ASCIIOnly {
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

// --------- helpers ---------

func (m *Model) sourceBadge(q domain.Quote) (string, string) {
	st := m.styles
	switch q.Source {
	case domain.SourceCache:
		return st.BadgeCache.Render("[~]"), "cached " + q.FetchedAt.Local().Format("15:04")
	case domain.SourceFinnhub:
		if q.Partial {
			return st.BadgePartial.Render("[!]"), "via finnhub (partial)"
		}
		return st.BadgeFresh.Render("[*]"), "via finnhub"
	}
	return st.BadgeCache.Render("[*]"), "via " + string(q.Source)
}

func (m *Model) sessionLabel(q domain.Quote) string {
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

func (m *Model) formatChange(q domain.Quote) (arrow, body string) {
	p := m.app.Printer
	arrow = "•"
	if q.Change > 0 {
		arrow = "▲"
	} else if q.Change < 0 {
		arrow = "▼"
	}
	if m.app.ASCIIOnly {
		switch arrow {
		case "▲":
			arrow = "^"
		case "▼":
			arrow = "v"
		default:
			arrow = "."
		}
	}
	body = p.Sprintf("%+.2f (%+.2f%%)", q.Change, q.ChangePct)
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

// --- formatting primitives ---

type printer interface {
	Sprintf(string, ...any) string
}

func optFloat(p printer, o domain.Optional[float64]) string {
	if !o.Valid {
		return "—"
	}
	return p.Sprintf("%.2f", o.Value)
}

func optPercent(p printer, o domain.Optional[float64]) string {
	if !o.Valid {
		return "—"
	}
	return p.Sprintf("%.2f%%", o.Value)
}

func rangeStr(p printer, lo, hi domain.Optional[float64]) string {
	if !lo.Valid || !hi.Valid {
		return "—"
	}
	return p.Sprintf("%.2f – %.2f", lo.Value, hi.Value)
}

func formatVolume(v domain.Optional[int64]) string {
	if !v.Valid {
		return "—"
	}
	return humanizeInt(v.Value)
}

func formatMarketCap(m domain.Optional[float64], cur string) string {
	if !m.Valid {
		return "—"
	}
	// Finnhub returns the figure in millions of the reporting currency.
	return humanizeMillions(m.Value) + " " + cur
}

// humanizeMillions turns 1_234_567 (expressed as "millions of units" from the
// API) into a compact string like "1.23T".
func humanizeMillions(v float64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.2fT", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.2fB", v/1_000)
	default:
		return fmt.Sprintf("%.2fM", v)
	}
}

func humanizeInt(n int64) string {
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

func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func centerVert(s string, h int) string {
	lines := strings.Count(s, "\n") + 1
	pad := (h - lines) / 2
	if pad <= 0 {
		return s
	}
	return strings.Repeat("\n", pad) + s
}

func shortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
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
