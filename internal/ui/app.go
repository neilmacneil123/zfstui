package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	// ZFSErr is set when zfs is unavailable at startup (status banner).
	ZFSErr error
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
	screenDatasets
	screenSnapshots
	screenSchedules
	screenCrontab
)

// clearStatusMsg clears a transient error status after a delay.
type clearStatusMsg struct {
	Token int
}

// App is the root Bubble Tea model: chrome + screen navigation.
type App struct {
	deps   *Deps
	width  int
	height int

	screen     screenKind
	prevScreen screenKind // restored when help closes
	home       screens.Home
	datasets   screens.Datasets
	snapshots  screens.Snapshots
	schedules  screens.Schedules
	crontab    screens.Crontab

	// Placeholder screen for future stubs.
	placeholderTitle string
	placeholderBody  string

	status    string // transient status line
	statusErr bool   // render status in red
	navPath   string // pool/dataset breadcrumb
	statusTok int    // token to cancel delayed clear
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
	d.SetDryRun(dryRun)
	if z != nil {
		d.ZFSErr = z.Available(context.Background())
	}

	m := NewApp(d)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// NewApp constructs the root model at the home screen.
func NewApp(d *Deps) App {
	status := "Ready"
	statusErr := false
	if d != nil && d.ZFSErr != nil {
		status = "zfs unavailable: " + d.ZFSErr.Error()
		statusErr = true
	}
	return App{
		deps:      d,
		home:      screens.NewHome(),
		screen:    screenHome,
		status:    status,
		statusErr: statusErr,
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
		m.datasets.Width = msg.Width
		m.datasets.Height = msg.Height
		m.snapshots.Width = msg.Width
		m.snapshots.Height = msg.Height
		m.schedules.Width = msg.Width
		m.schedules.Height = msg.Height
		m.crontab.Width = msg.Width
		m.crontab.Height = msg.Height
		return m, nil

	case screens.SelectMsg:
		return m.handleSelect(msg.Action)

	case clearStatusMsg:
		if msg.Token == m.statusTok && m.statusErr {
			m.status = "Ready"
			m.statusErr = false
		}
		return m, nil

	case screens.PoolsLoadedMsg, screens.DatasetsLoadedMsg:
		var cmd tea.Cmd
		m.datasets, cmd = m.datasets.Update(msg)
		return m, cmd

	case screens.DatasetsStatusMsg:
		m.setStatus(msg.Text, msg.IsErr)
		if msg.Path != "" || !msg.IsErr {
			m.navPath = msg.Path
		}
		return m, m.maybeClearErrCmd(msg.IsErr)

	case screens.DatasetsBackMsg:
		m.screen = screenHome
		m.navPath = ""
		m.setStatus("Home", false)
		return m, nil

	case screens.OpenPoolMsg:
		m.navPath = msg.Pool
		m.setStatus("Loading datasets…", false)
		return m, screens.LoadDatasetsCmd(m.deps.Z, msg.Pool)

	case screens.ReloadPoolsMsg:
		m.navPath = ""
		m.setStatus("Loading pools…", false)
		return m, screens.LoadPoolsCmd(m.deps.Z)

	case screens.OpenSnapshotsMsg:
		return m.openSnapshots(msg.Dataset)

	case screens.SnapshotsLoadedMsg:
		var cmd tea.Cmd
		m.snapshots, cmd = m.snapshots.Update(msg)
		return m, cmd

	case screens.SnapshotsStatusMsg:
		m.setStatus(msg.Text, msg.IsErr)
		if msg.Path != "" {
			m.navPath = msg.Path
		}
		return m, m.maybeClearErrCmd(msg.IsErr)

	case screens.SnapshotOpDoneMsg:
		return m.handleSnapshotOpDone(msg)

	case screens.SnapshotsBackMsg:
		return m.backFromSnapshots(msg.Dataset)

	case screens.CreateSnapshotRequestMsg:
		return m, screens.CreateSnapshotCmd(m.deps.Z, msg.FullName, msg.Recursive)

	case screens.DestroySnapshotRequestMsg:
		return m, screens.DestroySnapshotCmd(m.deps.Z, msg.FullName, msg.Recursive)

	case screens.RestoreRequestMsg:
		m.snapshots.SetBusy(true)
		m.setStatus(fmt.Sprintf("%s…", msg.Op), false)
		return m, screens.RestoreCmd(m.deps.Z, msg)

	case screens.RestoreOpDoneMsg:
		return m.handleRestoreOpDone(msg)

	case screens.RestoreStatusMsg:
		m.setStatus(msg.Text, msg.IsErr)
		if msg.Path != "" {
			m.navPath = msg.Path
		}
		return m, m.maybeClearErrCmd(msg.IsErr)

	case screens.ConfirmResultMsg:
		switch m.screen {
		case screenSnapshots:
			var cmd tea.Cmd
			m.snapshots, cmd = m.snapshots.Update(msg)
			return m, cmd
		case screenSchedules:
			var cmd tea.Cmd
			m.schedules, cmd = m.schedules.Update(msg)
			return m, cmd
		case screenCrontab:
			var cmd tea.Cmd
			m.crontab, cmd = m.crontab.Update(msg)
			return m, cmd
		}
		return m, nil

	case screens.ScheduleFormResultMsg:
		var cmd tea.Cmd
		m.schedules, cmd = m.schedules.Update(msg)
		return m, cmd

	case screens.ScheduleDatasetsLoadedMsg:
		var cmd tea.Cmd
		m.schedules, cmd = m.schedules.Update(msg)
		return m, cmd

	case screens.SchedulesStatusMsg:
		m.setStatus(msg.Text, msg.IsErr)
		return m, m.maybeClearErrCmd(msg.IsErr)

	case screens.SchedulesBackMsg:
		m.screen = screenHome
		m.navPath = ""
		m.setStatus("Home", false)
		return m, nil

	case screens.CrontabStatusMsg:
		m.setStatus(msg.Text, msg.IsErr)
		return m, m.maybeClearErrCmd(msg.IsErr)

	case screens.CrontabBackMsg:
		m.screen = screenHome
		m.navPath = ""
		m.setStatus("Home", false)
		return m, nil

	case screens.CrontabOpDoneMsg:
		var cmd tea.Cmd
		m.crontab, cmd = m.crontab.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if m.shouldGlobalQuit(msg) {
			return m, tea.Quit
		}

		if m.screen == screenHelp {
			if key.Matches(msg, Keys.Help) || key.Matches(msg, Keys.Back) {
				m.screen = m.prevScreen
			}
			return m, nil
		}

		if !m.inputCaptured() {
			switch {
			case key.Matches(msg, Keys.Help):
				m.prevScreen = m.screen
				m.screen = screenHelp
				return m, nil

			case key.Matches(msg, Keys.DryRun):
				m.deps.SetDryRun(!m.deps.DryRun)
				if m.deps.DryRun {
					m.setStatus("Dry-run enabled — mutations are simulated", false)
				} else {
					m.setStatus("Dry-run disabled — live mode", false)
				}
				return m, nil
			}
		}

		// Any key dismisses red error styling (text remains until replaced).
		if m.statusErr {
			m.statusErr = false
		}

		switch m.screen {
		case screenHome:
			var cmd tea.Cmd
			m.home, cmd = m.home.Update(msg)
			return m, cmd
		case screenDatasets:
			var cmd tea.Cmd
			m.datasets, cmd = m.datasets.Update(msg)
			return m, cmd
		case screenSnapshots:
			var cmd tea.Cmd
			m.snapshots, cmd = m.snapshots.Update(msg)
			return m, cmd
		case screenSchedules:
			var cmd tea.Cmd
			m.schedules, cmd = m.schedules.Update(msg)
			return m, cmd
		case screenCrontab:
			// Keep dry-run flag in sync with session toggle.
			m.crontab.SetDryRun(m.deps.DryRun)
			var cmd tea.Cmd
			m.crontab, cmd = m.crontab.Update(msg)
			return m, cmd
		case screenPlaceholder:
			if key.Matches(msg, Keys.Back) {
				m.screen = screenHome
				m.navPath = ""
				m.setStatus("Home", false)
			}
			return m, nil
		}
	}

	return m, nil
}

func (m App) shouldGlobalQuit(msg tea.KeyMsg) bool {
	if m.inputCaptured() {
		return key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c")))
	}
	return key.Matches(msg, Keys.Quit)
}

func (m App) inputCaptured() bool {
	if m.screen == screenDatasets && m.datasets.IsFiltering() {
		return true
	}
	if m.screen == screenSnapshots && (m.snapshots.Confirm.Active || m.snapshots.Restore.Active) {
		return true
	}
	if m.screen == screenSchedules && m.schedules.InputCaptured() {
		return true
	}
	if m.screen == screenCrontab && m.crontab.InputCaptured() {
		return true
	}
	return false
}

func (m *App) setStatus(text string, isErr bool) {
	m.status = text
	m.statusErr = isErr
	if isErr {
		m.statusTok++
	}
}

func (m App) maybeClearErrCmd(isErr bool) tea.Cmd {
	if !isErr {
		return nil
	}
	tok := m.statusTok
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return clearStatusMsg{Token: tok}
	})
}

func (m App) handleSelect(action screens.MenuAction) (tea.Model, tea.Cmd) {
	switch action {
	case screens.ActionPools:
		m.screen = screenDatasets
		m.datasets = screens.NewDatasets()
		m.datasets.Width = m.width
		m.datasets.Height = m.height
		m.navPath = ""
		m.setStatus("Loading pools…", false)
		return m, screens.LoadPoolsCmd(m.deps.Z)
	case screens.ActionSchedules:
		m.schedules = screens.NewSchedules(m.deps.Cfg, m.deps.CfgPath, m.deps.Z)
		m.schedules.Width = m.width
		m.schedules.Height = m.height
		m.screen = screenSchedules
		m.navPath = ""
		n := 0
		if m.deps.Cfg != nil {
			n = len(m.deps.Cfg.Schedules)
		}
		m.setStatus(fmt.Sprintf("%d schedule(s)", n), false)
	case screens.ActionCrontab:
		m.crontab = screens.NewCrontab(m.deps.Cfg, m.deps.CfgPath, m.deps.Binary, m.deps.DryRun)
		m.crontab.Width = m.width
		m.crontab.Height = m.height
		m.screen = screenCrontab
		m.navPath = m.crontab.ModeLabel()
		m.setStatus("Crontab · "+m.crontab.ModeLabel(), false)
	case screens.ActionToggleDryRun:
		m.deps.SetDryRun(!m.deps.DryRun)
		if m.deps.DryRun {
			m.setStatus("Dry-run enabled — mutations are simulated", false)
		} else {
			m.setStatus("Dry-run disabled — live mode", false)
		}
	case screens.ActionQuit:
		return m, tea.Quit
	}
	return m, nil
}

func (m App) openSnapshots(dataset string) (tea.Model, tea.Cmd) {
	prefix := "zfstui"
	rec := false
	if m.deps.Cfg != nil {
		if m.deps.Cfg.Defaults.SnapshotPrefix != "" {
			prefix = m.deps.Cfg.Defaults.SnapshotPrefix
		}
		rec = m.deps.Cfg.Defaults.Recursive
	}
	m.snapshots = screens.NewSnapshots(dataset, prefix, rec)
	m.snapshots.Width = m.width
	m.snapshots.Height = m.height
	m.screen = screenSnapshots
	m.navPath = dataset
	m.setStatus("Loading snapshots…", false)
	return m, screens.LoadSnapshotsCmd(m.deps.Z, dataset)
}

func (m App) backFromSnapshots(dataset string) (tea.Model, tea.Cmd) {
	pool := dataset
	if i := strings.Index(dataset, "/"); i > 0 {
		pool = dataset[:i]
	}
	m.screen = screenDatasets
	m.datasets = screens.NewDatasets()
	m.datasets.Width = m.width
	m.datasets.Height = m.height
	m.navPath = pool
	m.setStatus("Loading datasets…", false)
	return m, screens.LoadDatasetsCmd(m.deps.Z, pool)
}

func (m App) handleSnapshotOpDone(msg screens.SnapshotOpDoneMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.snapshots.SetBusy(false)
		m.setStatus(msg.Err.Error(), true)
		return m, m.maybeClearErrCmd(true)
	}
	op := msg.Op
	if op == "" {
		op = "op"
	}
	m.setStatus(fmt.Sprintf("%s ok: %s", op, msg.Name), false)
	m.snapshots.SetBusy(true)
	m.snapshots.SetLoading(true)
	return m, screens.LoadSnapshotsCmd(m.deps.Z, m.snapshots.Dataset)
}

func (m App) handleRestoreOpDone(msg screens.RestoreOpDoneMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.snapshots.SetBusy(false)
		m.setStatus(msg.Err.Error(), true)
		return m, m.maybeClearErrCmd(true)
	}
	op := msg.Op
	if op == "" {
		op = "restore"
	}
	detail := msg.Target
	if detail == "" {
		detail = msg.Snapshot
	}
	m.setStatus(fmt.Sprintf("%s ok: %s", op, detail), false)
	m.snapshots.SetBusy(true)
	m.snapshots.SetLoading(true)
	ds := m.snapshots.Dataset
	if msg.Path != "" {
		ds = msg.Path
	}
	return m, screens.LoadSnapshotsCmd(m.deps.Z, ds)
}

// View implements tea.Model.
func (m App) View() string {
	if m.width == 0 {
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
	case screenDatasets:
		body = m.datasets.View()
	case screenSnapshots:
		body = m.snapshots.View()
	case screenSchedules:
		body = m.schedules.View()
	case screenCrontab:
		body = m.crontab.View()
	}

	status := m.viewStatusBar()
	help := m.viewHelpBar()

	chrome := 2
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
		helpRow("q", "Quit"),
		"",
		MutedStyle.Render("Pools & datasets:"),
		helpRow("/", "Filter datasets"),
		helpRow("c", "Create snapshot"),
		helpRow("d", "Destroy (typed confirm)"),
		"",
		MutedStyle.Render("Snapshots:"),
		helpRow("c / d", "Create / destroy"),
		helpRow("r", "Restore wizard (rollback / clone)"),
		"",
		MutedStyle.Render("Schedules:"),
		helpRow("a / e / x", "Add / edit / delete"),
		helpRow("space", "Toggle enabled"),
		"",
		MutedStyle.Render("Crontab:"),
		helpRow("i / u", "Install / uninstall"),
		helpRow("r", "Refresh preview"),
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
		cfg = shortenPath(cfg, max(12, m.width/4))
	}

	pathPart := ""
	if m.navPath != "" {
		pathPart = " " + SubtitleStyle.Render(m.navPath) + " " + MutedStyle.Render("│")
	}

	left := fmt.Sprintf(" %s  %s%s ", mode, MutedStyle.Render("│"), pathPart)
	mid := MutedStyle.Render(" " + cfg + " ")

	var right string
	if m.statusErr {
		right = DangerStyle.Render(" " + m.status + " ")
	} else {
		right = MutedStyle.Render(" " + m.status + " ")
	}

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
	case screenDatasets:
		if m.datasets.IsFiltering() {
			hints = []string{
				HelpHint("type", "filter"),
				HelpHint("enter", "apply"),
				HelpHint("esc", "clear"),
			}
		} else {
			hints = []string{
				HelpHint("j/k", "move"),
				HelpHint("enter", "open"),
				HelpHint("/", "filter"),
				HelpHint("esc", "back"),
				HelpHint("q", "quit"),
			}
		}
	case screenSnapshots:
		if m.snapshots.Confirm.Active {
			hints = []string{
				HelpHint("y/n|type", "confirm"),
				HelpHint("esc", "cancel"),
			}
		} else if m.snapshots.Restore.Active {
			hints = []string{
				HelpHint("step", "wizard"),
				HelpHint("esc", "cancel"),
			}
		} else {
			hints = []string{
				HelpHint("j/k", "move"),
				HelpHint("c", "create"),
				HelpHint("d", "destroy"),
				HelpHint("r", "restore"),
				HelpHint("esc", "back"),
				HelpHint("q", "quit"),
			}
		}
	case screenSchedules:
		if m.schedules.Form.Active {
			hints = []string{
				HelpHint("tab", "fields"),
				HelpHint("enter", "save"),
				HelpHint("esc", "cancel"),
			}
		} else if m.schedules.Confirm.Active {
			hints = []string{
				HelpHint("y/n", "confirm"),
				HelpHint("esc", "cancel"),
			}
		} else {
			hints = []string{
				HelpHint("j/k", "move"),
				HelpHint("a", "add"),
				HelpHint("e", "edit"),
				HelpHint("x", "delete"),
				HelpHint("space", "toggle"),
				HelpHint("esc", "back"),
			}
		}
	case screenCrontab:
		if m.crontab.Confirm.Active {
			hints = []string{
				HelpHint("y/n", "confirm"),
				HelpHint("esc", "cancel"),
			}
		} else {
			hints = []string{
				HelpHint("i", "install"),
				HelpHint("u", "uninstall"),
				HelpHint("r", "refresh"),
				HelpHint("esc", "back"),
				HelpHint("q", "quit"),
			}
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
