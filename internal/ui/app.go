package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/ui/screens"
	"github.com/zfstui/zfstui/internal/zfs"
)

// Deps holds shared application dependencies for all screens.
type Deps struct {
	CfgPath string
	Cfg     *config.Config
	Z       zfs.Client
	DryRun  bool
	Binary  string
}

// SetDryRun updates the session dry-run flag and propagates to *zfs.CLI when present.
func (d *Deps) SetDryRun(on bool) {
	d.DryRun = on
	if cli, ok := d.Z.(*zfs.CLI); ok {
		cli.DryRun = on
	}
}

// screenKind is the top-level navigation target.
type screenKind int

const (
	screenHome screenKind = iota
	screenPlaceholder
	screenHelp
)

// App is the root Bubble Tea model: chrome + screen navigation.
type App struct {
	deps   *Deps
	width  int
	height int

	screen screenKind
	home   screens.Home

	// Placeholder screen (Tasks 10–11 will replace these).
	placeholderTitle string
	placeholderBody  string

	status string // transient status line
}

// Run launches the TUI. This is the entry point wired to cli.StartTUI.
func Run(cfgPath string, cfg *config.Config, z zfs.Client, dryRun bool) error {
	d := &Deps{
		CfgPath: cfgPath,
		Cfg:     cfg,
		Z:       z,
		DryRun:  dryRun,
		Binary:  binaryPath(),
	}
	// Keep CLI dry-run in sync with the session flag.
	d.SetDryRun(dryRun)

	m := NewApp(d)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// NewApp constructs the root model at the home screen.
func NewApp(d *Deps) App {
	return App{
		deps:   d,
		home:   screens.NewHome(),
		screen: screenHome,
		status: "Ready",
	}
}

// Init implements tea.Model.
func (m App) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.home.Width = msg.Width
		m.home.Height = msg.Height
		return m, nil

	case screens.SelectMsg:
		return m.handleSelect(msg.Action)

	case tea.KeyMsg:
		// Always-on quit (ctrl+c and q).
		if key.Matches(msg, Keys.Quit) {
			return m, tea.Quit
		}

		// Help overlay: ? / esc closes; other keys ignored.
		if m.screen == screenHelp {
			if key.Matches(msg, Keys.Help) || key.Matches(msg, Keys.Back) {
				m.screen = screenHome
			}
			return m, nil
		}

		// Global keys available on every screen.
		switch {
		case key.Matches(msg, Keys.Help):
			m.screen = screenHelp
			return m, nil

		case key.Matches(msg, Keys.DryRun):
			m.deps.SetDryRun(!m.deps.DryRun)
			if m.deps.DryRun {
				m.status = "Dry-run enabled — mutations are simulated"
			} else {
				m.status = "Dry-run disabled — live mode"
			}
			return m, nil

		case key.Matches(msg, Keys.Back):
			if m.screen != screenHome {
				m.screen = screenHome
				m.status = "Home"
				return m, nil
			}
		}

		// Screen-specific input.
		switch m.screen {
		case screenHome:
			var cmd tea.Cmd
			m.home, cmd = m.home.Update(msg)
			return m, cmd
		case screenPlaceholder:
			// Only navigation/global keys matter; already handled.
			return m, nil
		}
	}

	return m, nil
}

func (m App) handleSelect(action screens.MenuAction) (tea.Model, tea.Cmd) {
	switch action {
	case screens.ActionPools:
		m.screen = screenPlaceholder
		m.placeholderTitle = "Pools & Datasets"
		m.placeholderBody = "Coming soon — browse pools, datasets, and snapshots.\n(Task 10 will wire this screen.)"
		m.status = "Pools & Datasets"
	case screens.ActionSchedules:
		m.screen = screenPlaceholder
		m.placeholderTitle = "Schedules"
		m.placeholderBody = "Coming soon — manage snapshot schedules and retention.\n(Task 11 will wire this screen.)"
		m.status = "Schedules"
	case screens.ActionCrontab:
		m.screen = screenPlaceholder
		m.placeholderTitle = "Crontab"
		m.placeholderBody = "Coming soon — install, preview, and sync cron entries.\n(Task 11 will wire this screen.)"
		m.status = "Crontab"
	case screens.ActionToggleDryRun:
		m.deps.SetDryRun(!m.deps.DryRun)
		if m.deps.DryRun {
			m.status = "Dry-run enabled — mutations are simulated"
		} else {
			m.status = "Dry-run disabled — live mode"
		}
	case screens.ActionQuit:
		return m, tea.Quit
	}
	return m, nil
}

// View implements tea.Model.
func (m App) View() string {
	if m.width == 0 {
		// Before first WindowSizeMsg.
		return MutedStyle.Render("loading…")
	}

	var body string
	switch m.screen {
	case screenHome:
		body = m.home.View()
	case screenPlaceholder:
		body = m.viewPlaceholder()
	case screenHelp:
		body = m.viewHelp()
	}

	status := m.viewStatusBar()
	help := m.viewHelpBar()

	// Layout: body fills remaining height above status + help bars.
	chrome := 2 // status + help
	bodyHeight := m.height - chrome
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	bodyBox := lipgloss.NewStyle().
		Width(m.width).
		Height(bodyHeight).
		Align(lipgloss.Left, lipgloss.Top).
		Render(body)

	return lipgloss.JoinVertical(lipgloss.Left, bodyBox, status, help)
}

func (m App) viewPlaceholder() string {
	title := PlaceholderTitleStyle.Render("◆ " + m.placeholderTitle)
	body := PlaceholderBodyStyle.Render(m.placeholderBody)
	hint := MutedStyle.Render("\n\nPress esc to return home.")
	return "\n  " + title + "\n\n  " + body + hint
}

func (m App) viewHelp() string {
	rows := []string{
		TitleStyle.Render("Keyboard shortcuts"),
		"",
		helpRow("↑/k  ↓/j", "Move selection"),
		helpRow("enter", "Select / open"),
		helpRow("esc", "Back / close help"),
		helpRow("D", "Toggle dry-run (session)"),
		helpRow("?", "Toggle this help"),
		helpRow("q", "Quit (or back when nested)"),
		"",
		MutedStyle.Render("Context keys (later screens):"),
		helpRow("c", "Create snapshot"),
		helpRow("d", "Destroy (confirm)"),
		helpRow("r", "Restore wizard"),
		helpRow("s", "Jump to schedules"),
		helpRow("/", "Filter lists"),
		"",
		MutedStyle.Render("Press esc or ? to close."),
	}
	panel := HelpPanelStyle.Width(min(m.width-4, 56)).Render(strings.Join(rows, "\n"))
	return "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(panel)
}

func helpRow(k, desc string) string {
	return fmt.Sprintf("  %s  %s", HelpKeyStyle.Render(fmt.Sprintf("%-8s", k)), HelpDescStyle.Render(desc))
}

func (m App) viewStatusBar() string {
	mode := StatusDryRun(m.deps.DryRun)
	cfg := m.deps.CfgPath
	if cfg == "" {
		cfg = "(no config)"
	} else {
		cfg = shortenPath(cfg, max(12, m.width/3))
	}

	left := fmt.Sprintf(" %s  %s ", mode, MutedStyle.Render("│"))
	mid := MutedStyle.Render(" " + cfg + " ")
	right := MutedStyle.Render(" " + m.status + " ")

	// Build a full-width bar.
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	bar := left + mid + strings.Repeat(" ", gap) + right
	return StatusBarStyle.Width(m.width).Render(bar)
}

func (m App) viewHelpBar() string {
	var hints []string
	switch m.screen {
	case screenHome:
		hints = []string{
			HelpHint("j/k", "move"),
			HelpHint("enter", "select"),
			HelpHint("D", "dry-run"),
			HelpHint("?", "help"),
			HelpHint("q", "quit"),
		}
	case screenPlaceholder:
		hints = []string{
			HelpHint("esc", "back"),
			HelpHint("D", "dry-run"),
			HelpHint("?", "help"),
			HelpHint("q", "quit"),
		}
	case screenHelp:
		hints = []string{
			HelpHint("esc/?", "close"),
			HelpHint("q", "quit"),
		}
	}
	return HelpBarStyle.Width(m.width).Render(strings.Join(hints, MutedStyle.Render("  ·  ")))
}

func binaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "zfstui"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

func shortenPath(p string, maxLen int) string {
	if maxLen < 8 {
		maxLen = 8
	}
	if len(p) <= maxLen {
		return p
	}
	// Keep basename with ellipsis prefix.
	base := filepath.Base(p)
	if len(base)+3 >= maxLen {
		return "…" + base
	}
	return "…" + p[len(p)-(maxLen-1):]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
