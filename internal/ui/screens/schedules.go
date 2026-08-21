package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/zfs"
)

// SchedulesStatusMsg updates the root status bar from the schedules screen.
type SchedulesStatusMsg struct {
	Text  string
	IsErr bool
}

// SchedulesBackMsg requests return to home.
type SchedulesBackMsg struct{}

// Schedules lists and edits config schedules.
type Schedules struct {
	Width   int
	Height  int
	Cfg     *config.Config
	CfgPath string
	Z       zfs.Client

	cursor  int
	Form    ScheduleForm
	Confirm Confirm
	err     string
}

// NewSchedules constructs the schedules browser bound to cfg.
func NewSchedules(cfg *config.Config, cfgPath string, clients ...zfs.Client) Schedules {
	var client zfs.Client
	if len(clients) > 0 {
		client = clients[0]
	}
	return Schedules{Cfg: cfg, CfgPath: cfgPath, Z: client}
}

// InputCaptured reports whether keys should not be handled globally (form/confirm).
func (s Schedules) InputCaptured() bool {
	return s.Form.Active || s.Confirm.Active
}

// Init implements tea.Model.
func (s Schedules) Init() tea.Cmd { return nil }

// Update handles keys, form, and confirm results.
func (s Schedules) Update(msg tea.Msg) (Schedules, tea.Cmd) {
	if s.Confirm.Active {
		var cmd tea.Cmd
		s.Confirm, cmd = s.Confirm.Update(msg)
		return s, cmd
	}
	if s.Form.Active {
		var cmd tea.Cmd
		s.Form, cmd = s.Form.Update(msg)
		return s, cmd
	}

	switch msg := msg.(type) {
	case ConfirmResultMsg:
		return s.handleConfirm(msg)
	case ScheduleFormResultMsg:
		return s.handleFormResult(msg)
	case tea.KeyMsg:
		return s.updateKeys(msg)
	}
	return s, nil
}

func (s Schedules) handleConfirm(msg ConfirmResultMsg) (Schedules, tea.Cmd) {
	if !msg.Confirmed {
		return s, schedStatus("cancelled", false)
	}
	if msg.ID != "delete" {
		return s, nil
	}
	idx, err := strconv.Atoi(msg.Value)
	if err != nil || s.Cfg == nil || idx < 0 || idx >= len(s.Cfg.Schedules) {
		return s, schedStatus("invalid delete target", true)
	}
	name := s.Cfg.Schedules[idx].Name
	prev := append([]config.Schedule(nil), s.Cfg.Schedules...)
	prevCursor := s.cursor
	s.Cfg.Schedules = append(s.Cfg.Schedules[:idx], s.Cfg.Schedules[idx+1:]...)
	if s.cursor >= len(s.Cfg.Schedules) && s.cursor > 0 {
		s.cursor = len(s.Cfg.Schedules) - 1
	}
	if err := s.save(); err != nil {
		s.Cfg.Schedules = prev
		s.cursor = prevCursor
		return s, schedStatus(err.Error(), true)
	}
	return s, schedStatus(fmt.Sprintf("deleted %s", name), false)
}

func (s Schedules) handleFormResult(msg ScheduleFormResultMsg) (Schedules, tea.Cmd) {
	if !msg.Saved {
		return s, schedStatus("cancelled", false)
	}
	if s.Cfg == nil {
		return s, schedStatus("no config loaded", true)
	}

	prev := append([]config.Schedule(nil), s.Cfg.Schedules...)
	prevCursor := s.cursor
	entries := msg.Schedules
	if len(entries) == 0 && msg.Schedule.Name != "" {
		entries = []config.Schedule{msg.Schedule}
	}
	if len(entries) == 0 {
		return s, schedStatus("no schedules to save", true)
	}
	if msg.EditIndex >= 0 && msg.EditIndex < len(s.Cfg.Schedules) {
		// Preserve enabled flag from existing entry.
		entries[0].Enabled = s.Cfg.Schedules[msg.EditIndex].Enabled
		s.Cfg.Schedules[msg.EditIndex] = entries[0]
		s.cursor = msg.EditIndex
	} else {
		s.Cfg.Schedules = append(s.Cfg.Schedules, entries...)
		s.cursor = len(s.Cfg.Schedules) - 1
	}
	if err := s.Cfg.Validate(); err != nil {
		s.Cfg.Schedules = prev
		s.cursor = prevCursor
		return s, schedStatus(err.Error(), true)
	}
	if err := s.save(); err != nil {
		s.Cfg.Schedules = prev
		s.cursor = prevCursor
		return s, schedStatus(err.Error(), true)
	}
	action := "added"
	if msg.EditIndex >= 0 {
		action = "updated"
	}
	if len(entries) == 1 {
		return s, schedStatus(fmt.Sprintf("%s schedule %s", action, entries[0].Name), false)
	}
	return s, schedStatus(fmt.Sprintf("%s %d schedules", action, len(entries)), false)
}

func (s Schedules) updateKeys(msg tea.KeyMsg) (Schedules, tea.Cmd) {
	n := 0
	if s.Cfg != nil {
		n = len(s.Cfg.Schedules)
	}

	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if n == 0 {
			return s, nil
		}
		if s.cursor > 0 {
			s.cursor--
		} else {
			s.cursor = n - 1
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if n == 0 {
			return s, nil
		}
		if s.cursor < n-1 {
			s.cursor++
		} else {
			s.cursor = 0
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
		return s, func() tea.Msg { return SchedulesBackMsg{} }
	case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
		return s.beginAdd()
	case key.Matches(msg, key.NewBinding(key.WithKeys("e", "enter"))):
		return s.beginEdit()
	case key.Matches(msg, key.NewBinding(key.WithKeys("x", "d"))):
		return s.beginDelete()
	case key.Matches(msg, key.NewBinding(key.WithKeys(" "))):
		return s.toggleEnabled()
	}
	return s, nil
}

func (s Schedules) beginAdd() (Schedules, tea.Cmd) {
	if s.Cfg == nil {
		return s, schedStatus("no config loaded", true)
	}
	s.Form = NewScheduleFormAdd(s.Cfg.Defaults, s.Cfg.Schedules)
	s.Form.Width = minInt(s.Width-4, 64)
	if s.Z != nil {
		s.Form.startDatasetLoad()
		return s, LoadScheduleDatasetsCmd(s.Z)
	}
	return s, nil
}

func (s Schedules) beginEdit() (Schedules, tea.Cmd) {
	if s.Cfg == nil || len(s.Cfg.Schedules) == 0 {
		return s, schedStatus("no schedule selected", true)
	}
	if s.cursor < 0 || s.cursor >= len(s.Cfg.Schedules) {
		return s, schedStatus("no schedule selected", true)
	}
	cur := s.Cfg.Schedules[s.cursor]
	s.Form = NewScheduleFormEdit(s.cursor, cur, s.Cfg.Defaults, s.Cfg.Schedules)
	s.Form.Width = minInt(s.Width-4, 64)
	if s.Z != nil {
		s.Form.startDatasetLoad()
		return s, LoadScheduleDatasetsCmd(s.Z)
	}
	return s, nil
}

func (s Schedules) beginDelete() (Schedules, tea.Cmd) {
	if s.Cfg == nil || len(s.Cfg.Schedules) == 0 {
		return s, schedStatus("no schedule selected", true)
	}
	if s.cursor < 0 || s.cursor >= len(s.Cfg.Schedules) {
		return s, schedStatus("no schedule selected", true)
	}
	name := s.Cfg.Schedules[s.cursor].Name
	body := fmt.Sprintf("Delete schedule permanently:\n  %s", name)
	s.Confirm = OpenYesNo("delete", "Delete schedule", body, strconv.Itoa(s.cursor), true)
	s.Confirm.Width = minInt(s.Width-4, 64)
	return s, nil
}

func (s Schedules) toggleEnabled() (Schedules, tea.Cmd) {
	if s.Cfg == nil || len(s.Cfg.Schedules) == 0 {
		return s, schedStatus("no schedule selected", true)
	}
	if s.cursor < 0 || s.cursor >= len(s.Cfg.Schedules) {
		return s, schedStatus("no schedule selected", true)
	}
	s.Cfg.Schedules[s.cursor].Enabled = !s.Cfg.Schedules[s.cursor].Enabled
	name := s.Cfg.Schedules[s.cursor].Name
	en := "disabled"
	if s.Cfg.Schedules[s.cursor].Enabled {
		en = "enabled"
	}
	if err := s.save(); err != nil {
		// revert
		s.Cfg.Schedules[s.cursor].Enabled = !s.Cfg.Schedules[s.cursor].Enabled
		return s, schedStatus(err.Error(), true)
	}
	return s, schedStatus(fmt.Sprintf("%s %s", name, en), false)
}

func (s Schedules) save() error {
	if s.Cfg == nil {
		return fmt.Errorf("no config")
	}
	if s.CfgPath == "" {
		return fmt.Errorf("no config path")
	}
	return s.Cfg.Save(s.CfgPath)
}

func schedStatus(text string, isErr bool) tea.Cmd {
	return func() tea.Msg {
		return SchedulesStatusMsg{Text: text, IsErr: isErr}
	}
}

// View renders the schedules table and optional overlays.
func (s Schedules) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))

	var b strings.Builder
	b.WriteString(title.Render("◆ Schedules"))
	b.WriteString(muted.Render("  · a add · e edit · x delete · space toggle · esc back"))
	b.WriteString("\n\n")

	if s.Cfg == nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))
		b.WriteString(errStyle.Render("  no config loaded"))
	} else {
		b.WriteString(s.renderTable())
	}

	body := b.String()
	if s.Form.Active {
		return body + "\n\n" + lipgloss.NewStyle().PaddingLeft(2).Render(s.Form.View())
	}
	if s.Confirm.Active {
		return body + "\n\n" + lipgloss.NewStyle().PaddingLeft(2).Render(s.Confirm.View())
	}
	return body
}

func (s Schedules) renderTable() string {
	header := tableHeader([]colSpec{
		{Title: "NAME", Width: 18},
		{Title: "DATASET", Width: 20},
		{Title: "CRON", Width: 14},
		{Title: "ON", Width: 4},
		{Title: "RETENTION", Width: 22},
	})
	if len(s.Cfg.Schedules) == 0 {
		return header + "\n" + mutedLine("  (no schedules — press a to add)")
	}
	var lines []string
	lines = append(lines, header)
	for i, sch := range s.Cfg.Schedules {
		on := "no"
		if sch.Enabled {
			on = "yes"
		}
		line := tableRow(
			[]string{sch.Name, sch.Dataset, sch.Cron, on, retentionSummary(sch)},
			[]int{18, 20, 14, 4, 22},
			i == s.cursor,
		)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// RetentionSummary formats GFS retention for the table. Exported for tests.
func RetentionSummary(s config.Schedule) string {
	return retentionSummary(s)
}

func retentionSummary(s config.Schedule) string {
	var parts []string
	if s.Retention.Hourly > 0 {
		parts = append(parts, fmt.Sprintf("H %d", s.Retention.Hourly))
	}
	if s.Retention.Daily > 0 {
		parts = append(parts, fmt.Sprintf("D %d", s.Retention.Daily))
	}
	if s.Retention.Weekly > 0 {
		parts = append(parts, fmt.Sprintf("W %d", s.Retention.Weekly))
	}
	if s.Retention.Monthly > 0 {
		parts = append(parts, fmt.Sprintf("M %d", s.Retention.Monthly))
	}
	if s.Retention.Yearly > 0 {
		parts = append(parts, fmt.Sprintf("Y %d", s.Retention.Yearly))
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}
