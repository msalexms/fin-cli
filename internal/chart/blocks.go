package chart

import "strings"

// Blocks renders a Series as a single-line sparkline using Unicode block elements.
type Blocks struct{}

var blockRunes = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Render ignores height and fills width characters (or len(s) if smaller).
func (Blocks) Render(s Series, width, _ int) string {
	if len(s) == 0 {
		return strings.Repeat(" ", max(width, 1))
	}
	// Resample s to width buckets (nearest-neighbor; adequate for sparkline).
	samples := resample(s, width)

	min, max := samples[0], samples[0]
	for _, v := range samples {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	rng := max - min
	var b strings.Builder
	b.Grow(len(samples) * 3)
	for _, v := range samples {
		var idx int
		if rng == 0 {
			idx = len(blockRunes) / 2
		} else {
			norm := (v - min) / rng
			idx = int(norm * float64(len(blockRunes)-1))
			if idx < 0 {
				idx = 0
			} else if idx >= len(blockRunes) {
				idx = len(blockRunes) - 1
			}
		}
		b.WriteRune(blockRunes[idx])
	}
	return b.String()
}

// resample returns exactly n samples from s using nearest-neighbor indexing.
func resample(s Series, n int) Series {
	if n <= 0 {
		return Series{}
	}
	if len(s) == n {
		return s
	}
	out := make(Series, n)
	if len(s) == 0 {
		return out
	}
	if n == 1 {
		out[0] = s[len(s)-1]
		return out
	}
	for i := 0; i < n; i++ {
		pos := float64(i) * float64(len(s)-1) / float64(n-1)
		out[i] = s[int(pos+0.5)]
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
