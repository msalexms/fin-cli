package format

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/msalexms/fin-cli/internal/domain"
)

type plain struct{}

func (plain) Sprintf(f string, a ...any) string { return fmt.Sprintf(f, a...) }

var none = domain.Optional[float64]{}

func TestValues(t *testing.T) {
	p := plain{}
	cases := map[string][2]string{
		"price":         {Price(p, 189.456), "189.46"},
		"price sub-1":   {Price(p, 0.4567), "0.4567"},
		"price fx":      {Price(p, 1.09123), "1.0912"},
		"change fx":     {Change(p, 1.0912, -0.0023, -0.21), "-0.0023 (-0.21%)"},
		"price tiny":    {Price(p, 0.0000123), "0.000012"},
		"opt missing":   {OptFloat(p, none), Dash},
		"opt percent":   {OptPercent(p, domain.Some(2.5)), "2.50%"},
		"range":         {Range(p, domain.Some(10.0), domain.Some(20.0)), "10.00 – 20.00"},
		"range partial": {Range(p, none, domain.Some(20.0)), Dash},
		"volume":        {Volume(domain.Some[int64](5_000_000)), "5.00M"},
		"mcap":          {MarketCap(domain.Some(2_900_000.0), "USD"), "2.90T USD"},
		"mcap no cur":   {MarketCap(domain.Some(500.0), ""), "500.00M"},
		"humanize K":    {HumanizeInt(1_500), "1.50K"},
		"humanize B":    {HumanizeMillions(2_500), "2.50B"},
		"change":        {Change(p, 227.3, -2.5, -1.1), "-2.50 (-1.10%)"},
		"percent":       {Percent(p, 1.234), "+1.23%"},
		"session":       {SessionLabel(domain.SessionPost), "after-hours"},
		"duration s":    {Duration(45 * time.Second), "45s"},
		"duration m":    {Duration(4*time.Minute + 59*time.Second), "4m"},
		"duration h":    {Duration(65 * time.Minute), "1h05m"},
		"duration neg":  {Duration(-time.Second), "0s"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s: got %q, want %q", name, c[0], c[1])
		}
	}
}

func TestChangeArrow(t *testing.T) {
	for _, c := range []struct {
		change float64
		ascii  bool
		want   string
	}{{1, false, "▲"}, {-1, false, "▼"}, {0, false, "•"}, {1, true, "^"}, {-1, true, "v"}, {0, true, "="}} {
		if got := ChangeArrow(c.change, c.ascii); got != c.want {
			t.Errorf("ChangeArrow(%v, %v) = %q, want %q", c.change, c.ascii, got, c.want)
		}
	}
}

func TestProvenance(t *testing.T) {
	if got := Provenance(domain.Quote{Source: "finnhub"}); got != "via finnhub" {
		t.Errorf("fresh: %q", got)
	}
	if got := Provenance(domain.Quote{Source: "yahoo", Partial: true}); got != "via yahoo · partial data" {
		t.Errorf("partial: %q", got)
	}
	if got := Provenance(domain.Quote{Source: "yahoo", Stale: true, FetchedAt: time.Now()}); !strings.HasPrefix(got, "offline · cached ") {
		t.Errorf("stale: %q", got)
	}
}

func TestMetaLine(t *testing.T) {
	q := domain.Quote{Exchange: "NASDAQ", Industry: "Tech", Country: "US", IPODate: "1980-12-12"}
	if got := MetaLine(q); got != "NASDAQ · Tech · US · IPO 1980-12-12" {
		t.Errorf("MetaLine = %q", got)
	}
	if MetaLine(domain.Quote{}) != "" {
		t.Error("empty quote should give empty meta line")
	}
}

func TestStatsRows(t *testing.T) {
	q := domain.Quote{PrevClose: 150, MarketCap: domain.Some(2500.0), Currency: "USD", AvgVolume: domain.Some[int64](7_000_000)}
	rows := StatsRows(q, plain{})
	if len(rows) != 10 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0] != [2]string{"Prev Close", "150.00"} || rows[5] != [2]string{"Market Cap", "2.50B USD"} {
		t.Errorf("unexpected rows: %v", rows)
	}
	if rows[4] != [2]string{"Avg Volume", "7.00M"} {
		t.Errorf("average volume must be labelled as such: %v", rows[4])
	}
}
