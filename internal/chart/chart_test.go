package chart

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func TestTrend(t *testing.T) {
	if (Series{1, 2}).Trend() != 1 || (Series{2, 1}).Trend() != -1 || (Series{1}).Trend() != 0 {
		t.Fatal("unexpected trend")
	}
}

func TestSpark(t *testing.T) {
	s := Series{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, w := range []int{1, 5, 10, 20} {
		got := Spark(s, w)
		if n := utf8.RuneCountInString(got); n != min(w, len(s)) {
			t.Errorf("Spark width %d produced %d cells", w, n)
		}
	}
	if Spark(Series{3, 3, 3}, 3) != "▄▄▄" {
		t.Error("flat series should render mid blocks")
	}
}

func TestLineFitsWidth(t *testing.T) {
	s := Series{189.5, 191.2, 188.7, 192.4, 195.1, 193.3, 1999.99}
	for _, w := range []int{30, 60, 100} {
		for _, line := range strings.Split(Line(s, w, 6, ""), "\n") {
			if ansi.StringWidth(line) > w {
				t.Errorf("width %d: line too wide (%d): %q", w, ansi.StringWidth(line), line)
			}
		}
	}
}

func TestLineLabelsAreDistinct(t *testing.T) {
	s := Series{1.0912, 1.1003, 1.1187, 1.1045, 1.0951}
	seen := map[string]bool{}
	for _, l := range strings.Split(Line(s, 60, 8, ""), "\n") {
		label := strings.TrimSpace(strings.FieldsFunc(l, func(r rune) bool { return r == '┤' || r == '┼' })[0])
		if seen[label] {
			t.Fatalf("duplicate axis label %q", label)
		}
		seen[label] = true
	}
}
