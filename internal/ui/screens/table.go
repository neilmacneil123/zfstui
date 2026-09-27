package screens

import (
	"github.com/charmbracelet/lipgloss"
	"strings"
)

type colSpec struct {
	Title string
	Width int
}

func tableHeader(cols []colSpec) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Bold(true)
	p := make([]string, len(cols))
	for i, c := range cols {
		p[i] = padTruncate(c.Title, c.Width)
	}
	return "  " + style.Render(strings.Join(p, "  "))
}
func tableRow(cells []string, widths []int, selected bool) string {
	p := make([]string, len(cells))
	for i, v := range cells {
		w := 12
		if i < len(widths) {
			w = widths[i]
		}
		p[i] = padTruncate(v, w)
	}
	content := strings.Join(p, "  ")
	if selected {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee")).Bold(true).Render("▸ ") + lipgloss.NewStyle().Foreground(lipgloss.Color("#0d1117")).Background(lipgloss.Color("#22d3ee")).Bold(true).Render(content)
	}
	return "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3")).Render(content)
}
func padTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if len(s) > w {
		if w == 1 {
			return s[:1]
		}
		return s[:w-1] + "…"
	}
	return s + strings.Repeat(" ", w-len(s))
}
func mutedLine(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Render(s)
}
