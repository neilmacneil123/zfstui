package screens

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/schedule"
	"github.com/zfstui/zfstui/internal/zfs"
)

// schedule form field indices.
const (
	fieldName = iota
	fieldDataset
	fieldCron
	fieldPrefix
	fieldRecursive
	fieldDaily
	fieldWeekly
	fieldMonthly
	fieldYearly
	fieldCount
)

var formFieldLabels = []string{
	"name",
	"dataset",
	"cron",
	"prefix",
	"recursive",
	"daily",
	"weekly",
	"monthly",
	"yearly",
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

	datasets       []string
	datasetLoading bool
	datasetPicker  bool
	datasetCursor  int
	cronPicker     cronPicker
}

// ScheduleDatasetsLoadedMsg supplies the schedule form's live dataset choices.
type ScheduleDatasetsLoadedMsg struct {
	Datasets []string
	Err      error
}

// LoadScheduleDatasetsCmd loads datasets from every pool for the schedule picker.
func LoadScheduleDatasetsCmd(z zfs.Client) tea.Cmd {
	return func() tea.Msg {
		if z == nil {
			return ScheduleDatasetsLoadedMsg{Err: fmt.Errorf("zfs client unavailable")}
		}
		pools, err := z.ListPools(context.Background())
		if err != nil {
			return ScheduleDatasetsLoadedMsg{Err: err}
		}
		seen := make(map[string]struct{})
		var names []string
		for _, pool := range pools {
			datasets, err := z.ListDatasets(context.Background(), pool.Name)
			if err != nil {
				return ScheduleDatasetsLoadedMsg{Err: err}
			}
			for _, dataset := range datasets {
				if dataset.Name == "" {
					continue
				}
				if _, ok := seen[dataset.Name]; ok {
					continue
				}
				seen[dataset.Name] = struct{}{}
				names = append(names, dataset.Name)
			}
		}
		sort.Strings(names)
		return ScheduleDatasetsLoadedMsg{Datasets: names}
	}
}

type cronFrequency int

const (
	cronHourly cronFrequency = iota
	cronDaily
	cronWeekly
	cronMonthly
)

var cronFrequencyNames = []string{"Hourly", "Daily", "Weekly", "Monthly"}
var weekdayNames = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

type cronPicker struct {
	Active    bool
	Frequency cronFrequency
	Cursor    int
	Minute    int
	Hour      int
	Weekday   int
	MonthDay  int
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
	if s.Retention.Daily > 0 {
		f.fields[fieldDaily] = strconv.Itoa(s.Retention.Daily)
	}
	if s.Retention.Weekly > 0 {
		f.fields[fieldWeekly] = strconv.Itoa(s.Retention.Weekly)
	}
	if s.Retention.Monthly > 0 {
		f.fields[fieldMonthly] = strconv.Itoa(s.Retention.Monthly)
	}
	if s.Retention.Yearly > 0 {
		f.fields[fieldYearly] = strconv.Itoa(s.Retention.Yearly)
	}
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
	case ScheduleDatasetsLoadedMsg:
		f.datasetLoading = false
		if msg.Err != nil {
			f.err = "load datasets: " + msg.Err.Error()
			return f, nil
		}
		f.datasets = append([]string(nil), msg.Datasets...)
		f.syncDatasetCursor()
		return f, nil
	case tea.KeyMsg:
		if f.datasetPicker {
			return f.updateDatasetPicker(msg)
		}
		if f.cronPicker.Active {
			return f.updateCronPicker(msg)
		}
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
		if f.cursor == fieldDataset {
			return f.openDatasetPicker()
		}
		if f.cursor == fieldCron {
			f.openCronPicker()
			return f, nil
		}
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

func (f *ScheduleForm) startDatasetLoad() {
	f.datasetLoading = true
}

func (f *ScheduleForm) syncDatasetCursor() {
	f.datasetCursor = 0
	current := strings.TrimSpace(f.fields[fieldDataset])
	for i, name := range f.datasets {
		if name == current {
			f.datasetCursor = i
			return
		}
	}
}

func (f ScheduleForm) openDatasetPicker() (ScheduleForm, tea.Cmd) {
	if f.datasetLoading {
		f.err = "datasets are still loading"
		return f, nil
	}
	if len(f.datasets) == 0 {
		f.err = "no live datasets found; type a dataset path manually"
		return f, nil
	}
	f.syncDatasetCursor()
	f.datasetPicker = true
	f.err = ""
	return f, nil
}

func (f ScheduleForm) updateDatasetPicker(msg tea.KeyMsg) (ScheduleForm, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		f.datasetPicker = false
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if f.datasetCursor > 0 {
			f.datasetCursor--
		} else {
			f.datasetCursor = len(f.datasets) - 1
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if f.datasetCursor < len(f.datasets)-1 {
			f.datasetCursor++
		} else {
			f.datasetCursor = 0
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if f.datasetCursor >= 0 && f.datasetCursor < len(f.datasets) {
			f.fields[fieldDataset] = f.datasets[f.datasetCursor]
		}
		f.datasetPicker = false
		f.err = ""
	}
	return f, nil
}

func (f *ScheduleForm) openCronPicker() {
	f.cronPicker = cronPickerFromExpression(f.fields[fieldCron])
	f.cronPicker.Active = true
	f.err = ""
}

func cronPickerFromExpression(expr string) cronPicker {
	p := cronPicker{Frequency: cronDaily, Minute: 0, Hour: 2, MonthDay: 1}
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return p
	}
	minute, minuteErr := strconv.Atoi(parts[0])
	if minuteErr == nil && minute >= 0 && minute <= 59 {
		p.Minute = minute
	}
	if parts[1] == "*" && parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
		p.Frequency = cronHourly
		return p
	}
	hour, hourErr := strconv.Atoi(parts[1])
	if hourErr == nil && hour >= 0 && hour <= 23 {
		p.Hour = hour
	}
	if day, err := strconv.Atoi(parts[2]); err == nil && day >= 1 && day <= 31 && parts[3] == "*" && parts[4] == "*" {
		p.Frequency = cronMonthly
		p.MonthDay = day
		return p
	}
	if day, err := strconv.Atoi(parts[4]); err == nil && day >= 0 && day <= 6 && parts[2] == "*" && parts[3] == "*" {
		p.Frequency = cronWeekly
		p.Weekday = day
		return p
	}
	if parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
		p.Frequency = cronDaily
	}
	return p
}

func (p cronPicker) settingCount() int {
	if p.Frequency == cronHourly {
		return 2
	}
	if p.Frequency == cronDaily {
		return 3
	}
	return 4
}

func (p cronPicker) expression() string {
	switch p.Frequency {
	case cronHourly:
		return fmt.Sprintf("%d * * * *", p.Minute)
	case cronWeekly:
		return fmt.Sprintf("%d %d * * %d", p.Minute, p.Hour, p.Weekday)
	case cronMonthly:
		return fmt.Sprintf("%d %d %d * *", p.Minute, p.Hour, p.MonthDay)
	default:
		return fmt.Sprintf("%d %d * * *", p.Minute, p.Hour)
	}
}

func (f ScheduleForm) updateCronPicker(msg tea.KeyMsg) (ScheduleForm, tea.Cmd) {
	p := &f.cronPicker
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "c"))):
		p.Active = false
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		f.fields[fieldCron] = p.expression()
		p.Active = false
		f.err = ""
	case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
		p.Cursor = (p.Cursor + 1) % p.settingCount()
	case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
		p.Cursor = (p.Cursor - 1 + p.settingCount()) % p.settingCount()
	case key.Matches(msg, key.NewBinding(key.WithKeys("right", "up", "l", "k"))):
		adjustCronPicker(p, 1)
	case key.Matches(msg, key.NewBinding(key.WithKeys("left", "down", "h", "j"))):
		adjustCronPicker(p, -1)
	}
	return f, nil
}

func adjustCronPicker(p *cronPicker, delta int) {
	wrap := func(value, size int) int { return (value + delta + size) % size }
	switch p.Cursor {
	case 0:
		p.Frequency = cronFrequency(wrap(int(p.Frequency), len(cronFrequencyNames)))
		if p.Cursor >= p.settingCount() {
			p.Cursor = p.settingCount() - 1
		}
	case 1:
		p.Minute = wrap(p.Minute, 60)
	case 2:
		p.Hour = wrap(p.Hour, 24)
	case 3:
		if p.Frequency == cronWeekly {
			p.Weekday = wrap(p.Weekday, 7)
		} else {
			p.MonthDay = wrap(p.MonthDay-1, 31) + 1
		}
	}
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
	recursive := f.fields[fieldRecursive] == "true"

	if name == "" {
		return config.Schedule{}, fmt.Errorf("name is required")
	}
	for _, r := range name {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if !ok {
			return config.Schedule{}, fmt.Errorf("name must match [A-Za-z0-9._-]+")
		}
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

	parseCount := func(label, raw string) (int, error) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("%s must be an integer", label)
		}
		if n < 0 {
			return 0, fmt.Errorf("%s must be >= 0", label)
		}
		return n, nil
	}
	var ret config.Retention
	var err error
	if ret.Daily, err = parseCount("daily", f.fields[fieldDaily]); err != nil {
		return config.Schedule{}, err
	}
	if ret.Weekly, err = parseCount("weekly", f.fields[fieldWeekly]); err != nil {
		return config.Schedule{}, err
	}
	if ret.Monthly, err = parseCount("monthly", f.fields[fieldMonthly]); err != nil {
		return config.Schedule{}, err
	}
	if ret.Yearly, err = parseCount("yearly", f.fields[fieldYearly]); err != nil {
		return config.Schedule{}, err
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
	if f.datasetPicker {
		lines = append(lines, f.renderDatasetPicker(), "")
		lines = append(lines, muted.Render("↑↓ choose dataset · enter select · esc close"))
		return f.renderBox(lines)
	}
	if f.cronPicker.Active {
		lines = append(lines, f.renderCronPicker(), "")
		lines = append(lines, muted.Render("tab field · ←→ adjust · enter apply · c custom · esc close"))
		return f.renderBox(lines)
	}

	for i := 0; i < fieldCount; i++ {
		label := fmt.Sprintf("%-10s", formFieldLabels[i])
		val := f.fields[i]
		if i == fieldDataset && val == "" && f.datasetLoading {
			val = "loading datasets…"
		}
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
	lines = append(lines, muted.Render("tab/↑↓ fields · enter choose/save · ctrl+s save · esc cancel"))
	lines = append(lines, muted.Render("dataset/cron: enter opens picker · raw values remain editable"))
	return f.renderBox(lines)
}

func (f ScheduleForm) renderBox(lines []string) string {
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

func (f ScheduleForm) renderDatasetPicker() string {
	const visible = 8
	start := f.datasetCursor - visible/2
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(f.datasets) {
		end = len(f.datasets)
		start = end - visible
		if start < 0 {
			start = 0
		}
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e879f9"))
	selected := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0d1117")).Background(lipgloss.Color("#22d3ee"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	lines := []string{title.Render(fmt.Sprintf("Dataset (%d)", len(f.datasets)))}
	if start > 0 {
		lines = append(lines, muted.Render("  …"))
	}
	for i := start; i < end; i++ {
		line := "  " + f.datasets[i]
		if i == f.datasetCursor {
			line = selected.Render("▸ " + f.datasets[i] + " ")
		}
		lines = append(lines, line)
	}
	if end < len(f.datasets) {
		lines = append(lines, muted.Render("  …"))
	}
	return strings.Join(lines, "\n")
}

func (f ScheduleForm) renderCronPicker() string {
	p := f.cronPicker
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e879f9"))
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	selected := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0d1117")).Background(lipgloss.Color("#22d3ee"))
	value := lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3"))
	items := []struct{ name, value string }{
		{"frequency", cronFrequencyNames[p.Frequency]},
		{"minute", fmt.Sprintf("%02d", p.Minute)},
	}
	if p.Frequency != cronHourly {
		items = append(items, struct{ name, value string }{"hour", fmt.Sprintf("%02d", p.Hour)})
	}
	if p.Frequency == cronWeekly {
		items = append(items, struct{ name, value string }{"weekday", weekdayNames[p.Weekday]})
	} else if p.Frequency == cronMonthly {
		items = append(items, struct{ name, value string }{"month day", strconv.Itoa(p.MonthDay)})
	}
	lines := []string{title.Render("Schedule builder")}
	for i, item := range items {
		v := value.Render(item.value)
		if i == p.Cursor {
			v = selected.Render(" " + item.value + " ")
		}
		lines = append(lines, fmt.Sprintf("  %s %s", label.Render(fmt.Sprintf("%-10s", item.name)), v))
	}
	lines = append(lines, "", label.Render("  cron preview  ")+value.Render(p.expression()))
	lines = append(lines, label.Render("  Runs in the host's local timezone."))
	return strings.Join(lines, "\n")
}
