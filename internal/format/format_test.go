package format

import (
	"fmt"
	"testing"
	"time"

	"fin-cli/internal/domain"
)

// testPrinter satisfies Printer using fmt.Sprintf (no locale formatting).
type testPrinter struct{}

func (testPrinter) Sprintf(f string, a ...any) string { return fmt.Sprintf(f, a...) }

func TestOptFloat(t *testing.T) {
	p := testPrinter{}
	tests := []struct {
		name string
		in   domain.Optional[float64]
		want string
	}{
		{"valid", domain.Some(123.456), "123.46"},
		{"zero valid", domain.Some(0.0), "0.00"},
		{"invalid", domain.None[float64](), "\u2014"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OptFloat(p, tt.in)
			if got != tt.want {
				t.Errorf("OptFloat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOptPercent(t *testing.T) {
	p := testPrinter{}
	tests := []struct {
		name string
		in   domain.Optional[float64]
		want string
	}{
		{"valid", domain.Some(2.5), "2.50%"},
		{"invalid", domain.None[float64](), "\u2014"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OptPercent(p, tt.in)
			if got != tt.want {
				t.Errorf("OptPercent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRangeStr(t *testing.T) {
	p := testPrinter{}
	tests := []struct {
		name     string
		lo, hi   domain.Optional[float64]
		want     string
	}{
		{"both valid", domain.Some(10.0), domain.Some(20.0), "10.00 \u2013 20.00"},
		{"lo missing", domain.None[float64](), domain.Some(20.0), "\u2014"},
		{"hi missing", domain.Some(10.0), domain.None[float64](), "\u2014"},
		{"both missing", domain.None[float64](), domain.None[float64](), "\u2014"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RangeStr(p, tt.lo, tt.hi)
			if got != tt.want {
				t.Errorf("RangeStr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVolume(t *testing.T) {
	tests := []struct {
		name string
		in   domain.Optional[int64]
		want string
	}{
		{"invalid", domain.None[int64](), "\u2014"},
		{"small", domain.Some[int64](500), "500"},
		{"thousands", domain.Some[int64](1500), "1.50K"},
		{"millions", domain.Some[int64](2_500_000), "2.50M"},
		{"billions", domain.Some[int64](3_500_000_000), "3.50B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Volume(tt.in)
			if got != tt.want {
				t.Errorf("Volume() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarketCap(t *testing.T) {
	tests := []struct {
		name string
		m    domain.Optional[float64]
		cur  string
		want string
	}{
		{"invalid", domain.None[float64](), "USD", "\u2014"},
		{"millions", domain.Some(500.0), "USD", "500.00M USD"},
		{"billions", domain.Some(1500.0), "EUR", "1.50B EUR"},
		{"trillions", domain.Some(2_500_000.0), "USD", "2.50T USD"},
		{"no currency", domain.Some(100.0), "", "100.00M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarketCap(tt.m, tt.cur)
			if got != tt.want {
				t.Errorf("MarketCap() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHumanizeInt(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.00K"},
		{1_000_000, "1.00M"},
		{1_000_000_000, "1.00B"},
		{2_345_678_901, "2.35B"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := HumanizeInt(tt.in)
			if got != tt.want {
				t.Errorf("HumanizeInt(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHumanizeMillions(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0.00M"},
		{500, "500.00M"},
		{1000, "1.00B"},
		{1_500_000, "1.50T"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := HumanizeMillions(tt.in)
			if got != tt.want {
				t.Errorf("HumanizeMillions(%f) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestChangeArrow(t *testing.T) {
	tests := []struct {
		change    float64
		ascii     bool
		wantArrow string
	}{
		{1.5, false, "\u25B2"},
		{-0.5, false, "\u25BC"},
		{0, false, "\u2022"},
		{1.5, true, "^"},
		{-0.5, true, "v"},
		{0, true, "."},
	}
	for _, tt := range tests {
		t.Run(tt.wantArrow, func(t *testing.T) {
			got := ChangeArrow(tt.change, tt.ascii)
			if got != tt.wantArrow {
				t.Errorf("ChangeArrow(%f, %v) = %q, want %q", tt.change, tt.ascii, got, tt.wantArrow)
			}
		})
	}
}

func TestChangeBody(t *testing.T) {
	p := testPrinter{}
	got := ChangeBody(p, 1.23, 0.45)
	want := "+1.23 (+0.45%)"
	if got != want {
		t.Errorf("ChangeBody() = %q, want %q", got, want)
	}
	got = ChangeBody(p, -2.50, -1.10)
	want = "-2.50 (-1.10%)"
	if got != want {
		t.Errorf("ChangeBody() = %q, want %q", got, want)
	}
}

func TestSessionLabel(t *testing.T) {
	tests := []struct {
		s    domain.MarketSession
		want string
	}{
		{domain.SessionPre, "pre-market"},
		{domain.SessionRegular, "regular"},
		{domain.SessionPost, "after-hours"},
		{domain.SessionClosed, "closed"},
		{domain.SessionUnknown, ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.s), func(t *testing.T) {
			got := SessionLabel(tt.s)
			if got != tt.want {
				t.Errorf("SessionLabel(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestSourceBadge(t *testing.T) {
	tests := []struct {
		name        string
		q           domain.Quote
		wantBadge   string
		wantPartial bool // provider string contains "partial"
	}{
		{
			name:      "fresh full",
			q:         domain.Quote{Source: domain.SourceFinnhub, Partial: false},
			wantBadge: "[*]",
		},
		{
			name:        "fresh partial",
			q:           domain.Quote{Source: domain.SourceFinnhub, Partial: true},
			wantBadge:   "[!]",
			wantPartial: true,
		},
		{
			name:      "cache",
			q:         domain.Quote{Source: domain.SourceCache, FetchedAt: time.Now()},
			wantBadge: "[~]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			badge, prov := SourceBadge(tt.q)
			if badge != tt.wantBadge {
				t.Errorf("SourceBadge() badge = %q, want %q", badge, tt.wantBadge)
			}
			if tt.wantPartial {
				if !contains(prov, "partial") {
					t.Errorf("SourceBadge() provider = %q, want to contain 'partial'", prov)
				}
			}
		})
	}
}

func TestMetaLine(t *testing.T) {
	tests := []struct {
		name string
		q    domain.Quote
		want string
	}{
		{
			name: "all fields",
			q:    domain.Quote{Exchange: "NASDAQ", Industry: "Tech", Country: "US", IPODate: "1980-12-12"},
			want: "NASDAQ \u00B7 Tech \u00B7 US \u00B7 IPO 1980-12-12",
		},
		{
			name: "partial",
			q:    domain.Quote{Exchange: "NYSE", Country: "US"},
			want: "NYSE \u00B7 US",
		},
		{
			name: "empty",
			q:    domain.Quote{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MetaLine(tt.q)
			if got != tt.want {
				t.Errorf("MetaLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatsRows(t *testing.T) {
	p := testPrinter{}
	q := domain.Quote{
		PrevClose: 150.0,
		Open:      domain.Some(151.0),
		DayLow:    domain.Some(149.0),
		DayHigh:   domain.Some(152.0),
		Week52Low: domain.Some(120.0),
		Week52High: domain.Some(180.0),
		Volume:    domain.Some[int64](5_000_000),
		MarketCap: domain.Some(2500.0),
		Currency:  "USD",
		PE:        domain.Some(25.5),
		EPS:       domain.Some(6.10),
		Beta:      domain.Some(1.2),
		DivYield:  domain.Some(0.55),
	}
	rows := StatsRows(q, p)
	if len(rows) != 10 {
		t.Fatalf("StatsRows() returned %d rows, want 10", len(rows))
	}
	// Spot-check some rows.
	if rows[0][0] != "Prev Close" || rows[0][1] != "150.00" {
		t.Errorf("row 0: %v", rows[0])
	}
	if rows[4][0] != "Volume" || rows[4][1] != "5.00M" {
		t.Errorf("row 4: %v", rows[4])
	}
	if rows[5][0] != "Market Cap" || rows[5][1] != "2.50B USD" {
		t.Errorf("row 5: %v", rows[5])
	}
}

func TestShortDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m"},
		{5 * time.Minute, "5m"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := ShortDuration(tt.d)
			if got != tt.want {
				t.Errorf("ShortDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestVisibleLen(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"hello", 5},
		{"\x1b[31mred\x1b[0m", 3},
		{"\x1b[38;2;78;154;6mgreen\x1b[0m text", 10},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := VisibleLen(tt.in)
			if got != tt.want {
				t.Errorf("VisibleLen(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestPadRightANSI(t *testing.T) {
	got := PadRightANSI("hi", 5)
	if got != "hi   " {
		t.Errorf("PadRightANSI(\"hi\", 5) = %q, want %q", got, "hi   ")
	}
	// Already wider
	got = PadRightANSI("hello world", 5)
	if got != "hello world" {
		t.Errorf("PadRightANSI(\"hello world\", 5) = %q, want original", got)
	}
}

func TestRepeat(t *testing.T) {
	if Repeat("ab", 3) != "ababab" {
		t.Error("Repeat positive")
	}
	if Repeat("x", 0) != "" {
		t.Error("Repeat zero")
	}
	if Repeat("x", -1) != "" {
		t.Error("Repeat negative")
	}
}

// contains is a test helper.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
