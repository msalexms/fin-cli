package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// thorPixels is the AMSTech Thor mark (the head with the >_ prompt) on a
// 12×12 grid: '#' head, 'p' prompt, '.' empty. Two pixel rows share one text
// row through half blocks, so it renders as 12 cells × 6 rows.
var thorPixels = []string{
	"..#......#..",
	"..##....##..",
	".####..####.",
	"############",
	"############",
	"###p########",
	"####p#######",
	"#####p######",
	"####p#######",
	"###p##pppp##",
	"############",
	".##########.",
}

// Logo renders the Thor mark with half-block characters.
func Logo(r *lipgloss.Renderer) string {
	if r == nil {
		r = lipgloss.DefaultRenderer()
	}
	color := func(px byte) string {
		switch px {
		case '#':
			return Thor
		case 'p':
			return Ink
		}
		return ""
	}
	var rows []string
	for y := 0; y+1 < len(thorPixels); y += 2 {
		var sb strings.Builder
		for x := range len(thorPixels[y]) {
			top, bottom := color(thorPixels[y][x]), color(thorPixels[y+1][x])
			st := r.NewStyle()
			switch {
			case top == "" && bottom == "":
				sb.WriteString(" ")
			case top == bottom:
				sb.WriteString(st.Foreground(lipgloss.Color(top)).Render("█"))
			case top == "":
				sb.WriteString(st.Foreground(lipgloss.Color(bottom)).Render("▄"))
			case bottom == "":
				sb.WriteString(st.Foreground(lipgloss.Color(top)).Render("▀"))
			default:
				sb.WriteString(st.Foreground(lipgloss.Color(top)).Background(lipgloss.Color(bottom)).Render("▀"))
			}
		}
		rows = append(rows, sb.String())
	}
	return strings.Join(rows, "\n")
}
