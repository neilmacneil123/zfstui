package screens

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/schedule"
)

// schedule form field indices.
const (
	fieldName = iota
	fieldDataset
	fieldCron
	fieldPrefix
	fieldRecursive
	fieldKeepLast
	fieldMaxAge
	fieldCount
)

var formFieldLabels = []string{
	"name",
	"dataset",
	"cron",
	"prefix",
	"recursive",
	"keep_last",
	"max_age",
}

// ScheduleForm is a multi-field editor for one schedule (add or edit).
type ScheduleForm struct {
	Active    bool
	EditIndex int // -1 = add; otherwise index into cfg.Schedules
	Width     int

	fields   [fieldCount]string
	cursor   int
	err      string
	defaults config.Defaults
	// taken names when adding/renaming (exclude current edit name).
	taken map[string]struct{}
}

// NewScheduleFormAdd builds an empty form prefilled with defaults.
func NewScheduleFormAdd(defaults config.Defaults, existing []config.Schedule) ScheduleForm {
	f := ScheduleForm{
		Active:    true,
		EditIndex: -1,
		defaults:  defaults,
		taken:     scheduleNameSet(existing, ""),
	}
	prefix := defaults.SnapshotPrefix
	if prefix == "" {
		prefix = "zfstui"
	}
	f.fields[fieldPrefix] = prefix
	f.fields[fieldRecursive] = boolStr(defaults.Recursive)
	f.fields[fieldCron] = "0 * * * *"
	return f
}

// NewScheduleFormEdit builds a form from an existing schedule.
func NewScheduleFormEdit(idx int, s config.Schedule, defaults config.Defaults, existing []config.Schedule) ScheduleForm {
	f := ScheduleForm{
		Active:    true,
		EditIndex: idx,
		defaults:  defaults,
		taken:     scheduleNameSet(existing, s.Name),
	}
	f.fields[fieldName] = s.Name
	f.fields[fieldDataset] = s.Dataset
	f.fields[fieldCron] = s.Cron
	f.fields[fieldPrefix] = s.Prefix
	if f.fields[fieldPrefix] == "" {
		f.fields[fieldPrefix] = defaults.SnapshotPrefix
	}
	f.fields[fieldRecursive] = boolStr(s.Recursive)
	if s.Retention.KeepLast != nil {
		f.fields[fieldKeepLast] = strconv.Itoa(*s.Retention.KeepLast)
	}
	f.fields[fieldMaxAge] = s.Retention.MaxAge
	return f
}

func scheduleNameSet(existing []config.Schedule, exclude string) map[string]struct{} {
	m := make(map[string]struct{}, len(existing))
	for _, s := range existing {
		if s.Name == exclude {
			continue
		}
		m[s.Name] = struct{}{}
	}
	return m
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// Init implements tea.Model.
func (f ScheduleForm) Init() tea.Cmd { return nil }

// Update handles form input. When inactive, returns unchanged.
func (f ScheduleForm) Update(msg tea.Msg) (ScheduleForm, tea.Cmd) {
	if !f.Active {
		return f, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return f.updateKey(msg)
	}
	return f, nil
}

func (f ScheduleForm) updateKey(msg tea.KeyMsg) (ScheduleForm, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		f.Active = false
		f.err = ""
		return f, func() tea.Msg {
			return ScheduleFormResultMsg{Saved: false}
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+s"))):
		return f.trySave()

	case key.Matches(msg, key.NewBinding(key.WithKeys("tab", "down", "j"))):
		// j only navigates when recursive field is selected (text fields use j as input).
		if msg.String() == "j" && f.cursor != fieldRecursive {
			return f.typeRunes(msg.Runes)
		}
		f.cursor = (f.cursor + 1) % fieldCount
		f.err = ""
		return f, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab", "up", "k"))):
		if msg.String() == "k" && f.cursor != fieldRecursive {
			return f.typeRunes(msg.Runes)
		}
		f.cursor = (f.cursor - 1 + fieldCount) % fieldCount
		f.err = ""
		return f, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		// enter on recursive toggles; otherwise save.
		if f.cursor == fieldRecursive {
			f.toggleRecursive()
			return f, nil
		}
		return f.trySave()

	case key.Matches(msg, key.NewBinding(key.WithKeys(" "))):
		if f.cursor == fieldRecursive {
			f.toggleRecursive()
			return f, nil
		}
		return f.typeRunes([]rune{' '})

	case key.Matches(msg, key.NewBinding(key.WithKeys("backspace"))):
		if f.cursor == fieldRecursive {
			return f, nil
		}
		if v := f.fields[f.cursor]; len(v) > 0 {
			f.fields[f.cursor] = v[:len(v)-1]
		}
		f.err = ""
		return f, nil

	case msg.Type == tea.KeyRunes:
		if f.cursor == fieldRecursive {
			// y/n quick toggle
			r := string(msg.Runes)
			if r == "y" || r == "Y" || r == "t" || r == "T" || r == "1" {
				f.fields[fieldRecursive] = "true"
			} else if r == "n" || r == "N" || r == "f" || r == "F" || r == "0" {
				f.fields[fieldRecursive] = "false"
			}
			return f, nil
		}
		return f.typeRunes(msg.Runes)
	}
	return f, nil
}

func (f ScheduleForm) typeRunes(runes []rune) (ScheduleForm, tea.Cmd) {
	f.fields[f.cursor] += string(runes)
	f.err = ""
	return f, nil
}

func (f *ScheduleForm) toggleRecursive() {
	if f.fields[fieldRecursive] == "true" {
		f.fields[fieldRecursive] = "false"
	} else {
		f.fields[fieldRecursive] = "true"
	}
	f.err = ""
}

func (f ScheduleForm) trySave() (ScheduleForm, tea.Cmd) {
	s, err := f.Validate()
	if err != nil {
		f.err = err.Error()
		return f, nil
	}
	f.Active = false
	f.err = ""
	return f, func() tea.Msg {
		return ScheduleFormResultMsg{Saved: true, EditIndex: f.EditIndex, Schedule: s}
	}
}

// ScheduleFormResultMsg is emitted when the form closes.
type ScheduleFormResultMsg struct {
	Saved     bool
	EditIndex int // -1 = add
	Schedule  config.Schedule
}

// Validate parses and checks form fields. Exported for unit tests.
func (f ScheduleForm) Validate() (config.Schedule, error) {
	name := strings.TrimSpace(f.fields[fieldName])
	dataset := strings.TrimSpace(f.fields[fieldDataset])
	cronExpr := strings.TrimSpace(f.fields[fieldCron])
	prefix := strings.TrimSpace(f.fields[fieldPrefix])
	keepStr := strings.TrimSpace(f.fields[fieldKeepLast])
	maxAge := strings.TrimSpace(f.fields[fieldMaxAge])
	recursive := f.fields[fieldRecursive] == "true"

	if name == "" {
		return config.Schedule{}, fmt.Errorf("name is required")
	}
	if _, dup := f.taken[name]; dup {
		return config.Schedule{}, fmt.Errorf("duplicate schedule name %q", name)
	}
	if dataset == "" {
		return config.Schedule{}, fmt.Errorf("dataset is required")
	}
	if cronExpr == "" {
		return config.Schedule{}, fmt.Errorf("cron is required")
	}
	if err := schedule.ValidateCron(cronExpr); err != nil {
		return config.Schedule{}, err
	}
	if prefix == "" {
		prefix = f.defaults.SnapshotPrefix
	}
	if prefix == "" {
		return config.Schedule{}, fmt.Errorf("prefix is required")
	}

	var ret config.Retention
	if keepStr != "" {
		n, err := strconv.Atoi(keepStr)
		if err != nil {
			return config.Schedule{}, fmt.Errorf("keep_last must be an integer")
		}
		if n < 0 {
			return config.Schedule{}, fmt.Errorf("keep_last must be >= 0")
		}
		ret.KeepLast = &n
	}
	if maxAge != "" {
		if _, err := schedule.ParseMaxAge(maxAge); err != nil {
			return config.Schedule{}, err
		}
		ret.MaxAge = maxAge
	}

	return config.Schedule{
		Name:      name,
		Dataset:   dataset,
		Recursive: recursive,
		Cron:      cronExpr,
		Prefix:    prefix,
		Enabled:   true, // add defaults enabled; edit preserves via caller if needed
		Retention: ret,
	}, nil
}

// View renders the form panel.
func (f ScheduleForm) View() string {
	if !f.Active {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3"))
	selLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee")).Bold(true)
	selValue := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0d1117")).
		Background(lipgloss.Color("#22d3ee")).
		Bold(true)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))

	title := "Add schedule"
	if f.EditIndex >= 0 {
		title = "Edit schedule"
	}

	var lines []string
	lines = append(lines, titleStyle.Render(title))
	lines = append(lines, "")

	for i := 0; i < fieldCount; i++ {
		label := fmt.Sprintf("%-10s", formFieldLabels[i])
		val := f.fields[i]
		if i == fieldRecursive {
			if val == "true" {
				val = "yes"
			} else {
				val = "no"
			}
		}
		if i == f.cursor {
			cursor := "▌"
			if i == fieldRecursive {
				cursor = ""
			}
			lines = append(lines, selLabel.Render("▸ "+label)+" "+selValue.Render(" "+val+cursor+" "))
		} else {
			lines = append(lines, "  "+labelStyle.Render(label)+" "+valueStyle.Render(val))
		}
	}

	lines = append(lines, "")
	if f.err != "" {
		lines = append(lines, errStyle.Render(f.err))
		lines = append(lines, "")
	}
	lines = append(lines, muted.Render("tab/↑↓ fields · enter save · space toggle recursive · esc cancel"))

	w := f.Width
	if w < 40 {
		w = 52
	}
	if w > 72 {
		w = 72
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#e879f9")).
		Padding(1, 2).
		Width(w).
		Render(strings.Join(lines, "\n"))
}
