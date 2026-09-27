package screens

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MenuAction identifies a home-menu selection.
type MenuAction int

const (
	ActionNone MenuAction = iota
	ActionPools
	ActionSchedules
	ActionCrontab
	ActionToggleDryRun
	ActionQuit
)

// SelectMsg is emitted when the user activates a home menu item.
type SelectMsg struct {
	Action MenuAction
}

type menuItem struct {
	Label  string
	Desc   string
	Action MenuAction
	Icon   string
}

// Home is the main menu screen.
type Home struct {
	Cursor int
	Width  int
	Height int
	items  []menuItem
}

// NewHome builds the default home menu.
func NewHome() Home {
	return Home{
		items: []menuItem{
			{
				Icon:   "◈",
				Label:  "Snapshot Dashboard",
				Desc:   "Host-wide pools, datasets, snapshots, and schedules",
				Action: ActionPools,
			},
			{
				Icon:   "◷",
				Label:  "Schedules",
				Desc:   "Manage snapshot schedules and retention",
				Action: ActionSchedules,
			},
			{
				Icon:   "⚙",
				Label:  "Crontab",
				Desc:   "Install, preview, and sync cron entries",
				Action: ActionCrontab,
			},
			{
				Icon:   "⟳",
				Label:  "Toggle Dry-Run",
				Desc:   "Toggle session dry-run (no real mutations)",
				Action: ActionToggleDryRun,
			},
			{
				Icon:   "✕",
				Label:  "Quit",
				Desc:   "Exit zfstui",
				Action: ActionQuit,
			},
		},
	}
}

// Init implements tea.Model.
func (h Home) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (h Home) Update(msg tea.Msg) (Home, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if h.Cursor > 0 {
				h.Cursor--
			} else {
				h.Cursor = len(h.items) - 1
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if h.Cursor < len(h.items)-1 {
				h.Cursor++
			} else {
				h.Cursor = 0
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", " "))):
			item := h.items[h.Cursor]
			return h, func() tea.Msg {
				return SelectMsg{Action: item.Action}
			}
		case msg.String() >= "1" && msg.String() <= "5":
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(h.items) {
				h.Cursor = idx
				item := h.items[h.Cursor]
				return h, func() tea.Msg {
					return SelectMsg{Action: item.Action}
				}
			}
		}
	}
	return h, nil
}

// View renders the banner and menu.
func (h Home) View() string {
	var b strings.Builder
	b.WriteString(renderBanner())
	b.WriteString("\n\n")
	b.WriteString(h.renderMenu())
	return b.String()
}

func renderBanner() string {
	// Soft cyan/magenta dual-tone box banner.
	border := lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee")).Bold(true)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee"))
	sub := lipgloss.NewStyle().Foreground(lipgloss.Color("#e879f9"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#334155"))

	const width = 42
	top := border.Render("╔" + strings.Repeat("═", width) + "╗")
	bot := border.Render("╚" + strings.Repeat("═", width) + "╝")
	empty := func(s string) string {
		// Center s within width cells (approx; ASCII/Unicode mix).
		pad := width - lipgloss.Width(s)
		if pad < 0 {
			pad = 0
		}
		left := pad / 2
		right := pad - left
		return border.Render("║") + strings.Repeat(" ", left) + s + strings.Repeat(" ", right) + border.Render("║")
	}

	line1 := empty(title.Render("zfstui"))
	line2 := empty(sub.Render("ZFS Snapshot Manager"))
	line3 := empty(dim.Render("· · ·"))

	return strings.Join([]string{top, line1, line2, line3, bot}, "\n")
}

func (h Home) renderMenu() string {
	selected := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0d1117")).
		Background(lipgloss.Color("#22d3ee")).
		Bold(true).
		Padding(0, 1)
	normal := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#e6edf3")).
		PaddingLeft(2)
	cursor := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#22d3ee")).
		Bold(true)
	desc := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6b7280")).
		PaddingLeft(6)
	num := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6b7280"))

	var lines []string
	for i, item := range h.items {
		n := num.Render(fmt.Sprintf("%d.", i+1))
		label := fmt.Sprintf("%s  %s  %s", n, item.Icon, item.Label)
		if i == h.Cursor {
			lines = append(lines, cursor.Render("▸ ")+selected.Render(label))
			lines = append(lines, desc.Render(item.Desc))
		} else {
			lines = append(lines, "  "+normal.Render(label))
			lines = append(lines, desc.Render(item.Desc))
		}
		if i < len(h.items)-1 {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}
