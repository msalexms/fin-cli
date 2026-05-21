package chart

import (
	"github.com/guptarohit/asciigraph"
)

// ASCII renders a Series as a 2D plot using only ASCII characters.
type ASCII struct {
	Caption string
}

// Render returns a multi-line plot fitted into width×height.
func (a ASCII) Render(s Series, width, height int) string {
	if len(s) == 0 {
		return ""
	}
	if height < 3 {
		height = 3
	}
	if width < 10 {
		width = 10
	}
	opts := []asciigraph.Option{
		asciigraph.Height(height),
		asciigraph.Width(width),
	}
	if a.Caption != "" {
		opts = append(opts, asciigraph.Caption(a.Caption))
	}
	return asciigraph.Plot([]float64(s), opts...)
}
