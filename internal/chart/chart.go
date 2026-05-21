// Package chart renders price series as terminal-friendly charts.
//
// Two backends are exposed:
//   - Blocks: compact sparkline using Unicode block elements (▁▂▃▄▅▆▇█).
//     Best for TUI cells and narrow contexts.
//   - ASCII:  a two-dimensional plot drawn with only ASCII, powered by
//     guptarohit/asciigraph. Best for one-shot `quote` output and for
//     environments without Unicode support (LANG=C).
package chart

// Series is the ordered list of close prices.
type Series []float64

// Trend reports whether the series ends above its starting value.
func (s Series) Trend() int {
	if len(s) < 2 {
		return 0
	}
	if s[len(s)-1] > s[0] {
		return 1
	}
	if s[len(s)-1] < s[0] {
		return -1
	}
	return 0
}

// Renderer is implemented by the chart backends.
type Renderer interface {
	// Render returns a multi-line (possibly single-line) string representation
	// of s fitted to width×height characters.
	Render(s Series, width, height int) string
}

// Auto returns the recommended renderer for the current environment.
// When asciiOnly is true (LANG=C or dumb terminal), ASCII is forced.
// Otherwise Blocks is returned for tiny heights (<=1) and ASCII for taller ones.
func Auto(asciiOnly bool, height int) Renderer {
	if asciiOnly {
		return ASCII{}
	}
	if height <= 1 {
		return Blocks{}
	}
	return ASCII{}
}
