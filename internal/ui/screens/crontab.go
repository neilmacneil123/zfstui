package screens

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/cron"
)

// CrontabStatusMsg updates the root status bar from the crontab screen.
type CrontabStatusMsg struct {
	Text  string
	IsErr bool
}

// CrontabBackMsg requests return to home.
type CrontabBackMsg struct{}

// CrontabOpDoneMsg is returned after install/uninstall.
type CrontabOpDoneMsg struct {
	Op  string // "install" | "uninstall"
	Err error
}

// Crontab shows managed cron preview and install/uninstall actions.
type Crontab struct {
	Width   int
	Height  int
	Cfg     *config.Config
	CfgPath string
	Binary  string
	DryRun  bool

	// PreferSystem is true when running as root (euid == 0).
	PreferSystem bool
	// SystemPath is the system cron.d path.
	SystemPath string

	Confirm Confirm

	preview    string
	previewErr string
	busy       bool
	lastOp     string
}

// NewCrontab builds the crontab screen and computes preview.
func NewCrontab(cfg *config.Config, cfgPath, binary string, dryRun bool) Crontab {
	c := Crontab{
		Cfg:          cfg,
		CfgPath:      cfgPath,
		Binary:       binary,
		DryRun:       dryRun,
		PreferSystem: isRoot(),
		SystemPath:   cron.SystemPath,
	}
	c.refreshPreview()
	return c
}

// SetDryRun updates session dry-run for install decisions.
func (c *Crontab) SetDryRun(on bool) {
	c.DryRun = on
}

// InputCaptured reports whether a modal is capturing keys (confirm).
func (c Crontab) InputCaptured() bool {
	return c.Confirm.Active
}

func isRoot() bool {
	// Geteuid is -1 on Windows; only treat real root as system-capable.
	return os.Geteuid() == 0
}

func (c *Crontab) refreshPreview() {
	c.preview = ""
	c.previewErr = ""
	if c.Cfg == nil {
		c.previewErr = "no config loaded"
		return
	}
	bin := c.Binary
	if bin == "" {
		bin = "zfstui"
	}
	body, err := cron.Preview(c.Cfg, bin, c.CfgPath, c.PreferSystem)
	if err != nil {
		c.previewErr = err.Error()
		return
	}
	c.preview = body
}

// ModeLabel returns "system" or "user".
func (c Crontab) ModeLabel() string {
	if c.PreferSystem {
		return "system"
	}
	return "user"
}

// Init implements tea.Model.
func (c Crontab) Init() tea.Cmd { return nil }

// Update handles keys, confirm results, and op results.
func (c Crontab) Update(msg tea.Msg) (Crontab, tea.Cmd) {
	if c.Confirm.Active {
		var cmd tea.Cmd
		c.Confirm, cmd = c.Confirm.Update(msg)
		return c, cmd
	}

	switch msg := msg.(type) {
	case ConfirmResultMsg:
		return c.handleConfirm(msg)
	case CrontabOpDoneMsg:
		c.busy = false
		if msg.Err != nil {
			return c, cronStatus(msg.Err.Error(), true)
		}
		c.refreshPreview()
		return c, cronStatus(fmt.Sprintf("%s ok (%s)", msg.Op, c.ModeLabel()), false)
	case tea.KeyMsg:
		return c.updateKeys(msg)
	}
	return c, nil
}

func (c Crontab) handleConfirm(msg ConfirmResultMsg) (Crontab, tea.Cmd) {
	if !msg.Confirmed {
		return c, cronStatus("cancelled", false)
	}
	switch msg.ID {
	case "install":
		return c.doInstall()
	case "uninstall":
		return c.doUninstall()
	}
	return c, nil
}

func (c Crontab) updateKeys(msg tea.KeyMsg) (Crontab, tea.Cmd) {
	if c.busy {
		if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
			return c, func() tea.Msg { return CrontabBackMsg{} }
		}
		return c, nil
	}

	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
		return c, func() tea.Msg { return CrontabBackMsg{} }
	case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
		return c.beginInstall()
	case key.Matches(msg, key.NewBinding(key.WithKeys("u"))):
		return c.beginUninstall()
	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		c.refreshPreview()
		return c, cronStatus("preview refreshed", false)
	}
	return c, nil
}

func (c Crontab) beginInstall() (Crontab, tea.Cmd) {
	if c.Cfg == nil {
		return c, cronStatus("no config loaded", true)
	}
	c.refreshPreview()
	if c.previewErr != "" {
		return c, cronStatus(c.previewErr, true)
	}
	if c.DryRun {
		mode := c.ModeLabel()
		return c, cronStatus(fmt.Sprintf("dry-run: would install %s crontab (preview only)", mode), false)
	}
	mode := c.ModeLabel()
	target := "user crontab (managed block)"
	if c.PreferSystem {
		target = c.SystemPath
	}
	body := fmt.Sprintf("Install managed %s crontab?\n  %s", mode, target)
	c.Confirm = OpenYesNo("install", "Install crontab", body, "", true)
	c.Confirm.Width = minInt(c.Width-4, 64)
	return c, nil
}

func (c Crontab) beginUninstall() (Crontab, tea.Cmd) {
	if c.DryRun {
		mode := c.ModeLabel()
		return c, cronStatus(fmt.Sprintf("dry-run: would uninstall %s crontab (no write)", mode), false)
	}
	mode := c.ModeLabel()
	target := "user crontab managed block"
	if c.PreferSystem {
		target = c.SystemPath
	}
	body := fmt.Sprintf("Remove managed %s crontab?\n  %s", mode, target)
	c.Confirm = OpenYesNo("uninstall", "Uninstall crontab", body, "", true)
	c.Confirm.Width = minInt(c.Width-4, 64)
	return c, nil
}

func (c Crontab) doInstall() (Crontab, tea.Cmd) {
	c.busy = true
	c.lastOp = "install"
	cfg, path, bin, system, dest := c.Cfg, c.CfgPath, c.Binary, c.PreferSystem, c.SystemPath
	return c, func() tea.Msg {
		var err error
		if system {
			err = cron.InstallSystemFile(cfg, bin, path, dest)
		} else {
			err = cron.InstallUser(cfg, bin, path)
		}
		return CrontabOpDoneMsg{Op: "install", Err: err}
	}
}

func (c Crontab) doUninstall() (Crontab, tea.Cmd) {
	c.busy = true
	c.lastOp = "uninstall"
	system, dest := c.PreferSystem, c.SystemPath
	return c, func() tea.Msg {
		var err error
		if system {
			err = cron.UninstallSystemFile(dest)
		} else {
			err = cron.UninstallUser()
		}
		return CrontabOpDoneMsg{Op: "uninstall", Err: err}
	}
}

func cronStatus(text string, isErr bool) tea.Cmd {
	return func() tea.Msg {
		return CrontabStatusMsg{Text: text, IsErr: isErr}
	}
}

// View renders preview and status hints.
func (c Crontab) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee"))
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#e879f9"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))
	code := lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3"))

	var b strings.Builder
	b.WriteString(title.Render("◆ Crontab"))
	b.WriteString(muted.Render("  · i install · u uninstall · r refresh · esc back"))
	b.WriteString("\n\n")

	mode := c.ModeLabel()
	target := "user crontab (managed block)"
	if c.PreferSystem {
		target = c.SystemPath
	}
	b.WriteString(muted.Render("  Mode: "))
	b.WriteString(accent.Render(mode))
	b.WriteString(muted.Render("  →  "))
	b.WriteString(code.Render(target))
	if c.DryRun {
		b.WriteString(muted.Render("  "))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24")).Render("[dry-run]"))
	}
	b.WriteString("\n\n")

	b.WriteString(muted.Render("  Preview:"))
	b.WriteString("\n")

	if c.previewErr != "" {
		b.WriteString(errStyle.Render("  " + c.previewErr))
	} else if strings.TrimSpace(c.preview) == "" {
		b.WriteString(muted.Render("  (empty — enable schedules to generate lines)"))
	} else {
		for _, line := range strings.Split(strings.TrimRight(c.preview, "\n"), "\n") {
			b.WriteString(code.Render("  " + line))
			b.WriteString("\n")
		}
	}

	if c.busy {
		b.WriteString("\n")
		b.WriteString(muted.Render("  working…"))
	}

	enabled := 0
	if c.Cfg != nil {
		for _, s := range c.Cfg.Schedules {
			if s.Enabled {
				enabled++
			}
		}
		b.WriteString("\n")
		b.WriteString(muted.Render(fmt.Sprintf("  %d enabled schedule(s) of %d", enabled, len(c.Cfg.Schedules))))
	}

	body := b.String()
	if c.Confirm.Active {
		return body + "\n\n" + lipgloss.NewStyle().PaddingLeft(2).Render(c.Confirm.View())
	}
	return body
}
