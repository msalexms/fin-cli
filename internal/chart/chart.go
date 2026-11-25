// Package chart renders price series for the terminal.
package chart

import (
	"fmt"
	"math"
	"strings"

	"github.com/guptarohit/asciigraph"
)

// Series is an ordered list of close prices, oldest first.
type Series []float64

// Trend reports +1 if the series ends above where it started, -1 if below,
// and 0 otherwise.
func (s Series) Trend() int {
	if len(s) < 2 {
		return 0
	}
	switch last, first := s[len(s)-1], s[0]; {
	case last > first:
		return 1
	case last < first:
		return -1
	}
	return 0
}

// Line draws a multi-line plot whose total width (axis labels included)
// fits in width columns and whose plot area is height rows.
func Line(s Series, width, height int, caption string) string {
	if len(s) == 0 {
		return ""
	}
	// Enough decimals for the axis labels (one per row) to be distinct.
	prec := uint(2)
	if lo, hi := bounds(s); hi > lo {
		step := (hi - lo) / float64(max(height, 2))
		prec = uint(min(max(2, int(math.Ceil(-math.Log10(step)))+1), 6))
	}
	labelW := labelWidth(s, prec)
	opts := []asciigraph.Option{
		asciigraph.Height(max(height, 2)),
		asciigraph.Width(max(width-labelW-3, 8)),
		asciigraph.Precision(prec),
	}
	if caption != "" {
		opts = append(opts, asciigraph.Caption(caption))
	}
	return asciigraph.Plot(s, opts...)
}

// Spark draws a one-line sparkline of exactly width cells.
func Spark(s Series, width int) string {
	const blocks = "▁▂▃▄▅▆▇█"
	runes := []rune(blocks)
	if len(s) == 0 || width <= 0 {
		return ""
	}
	pts := resample(s, width)
	lo, hi := bounds(pts)
	var b strings.Builder
	for _, v := range pts {
		i := len(runes)/2 - 1 // flat series: a low-mid bar
		if hi > lo {
			i = int((v - lo) / (hi - lo) * float64(len(runes)-1))
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}

func bounds(s Series) (lo, hi float64) {
	lo, hi = s[0], s[0]
	for _, v := range s[1:] {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return lo, hi
}

func labelWidth(s Series, prec uint) int {
	lo, hi := bounds(s)
	f := fmt.Sprintf("%%.%df", prec)
	return max(len(fmt.Sprintf(f, lo)), len(fmt.Sprintf(f, hi)))
}

// resample picks n points from s by nearest index (s is returned as-is when
// it is not longer than n).
func resample(s Series, n int) Series {
	if len(s) <= n {
		return s
	}
	if n == 1 {
		return s[len(s)-1:]
	}
	out := make(Series, n)
	for i := range n {
		out[i] = s[int(math.Round(float64(i)*float64(len(s)-1)/float64(n-1)))]
	}
	return out
}
