package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zfstui/zfstui/internal/schedule"
	"github.com/zfstui/zfstui/internal/zfs"
)

// SnapshotsLoadedMsg is returned after async ListSnapshots.
type SnapshotsLoadedMsg struct {
	Dataset   string
	Snapshots []zfs.Snapshot
	Err       error
}

// SnapshotOpDoneMsg is returned after create/destroy completes.
type SnapshotOpDoneMsg struct {
	Op    string // "create" or "destroy"
	Name  string
	Err   error
	Path  string
}

// SnapshotsStatusMsg updates the root status bar.
type SnapshotsStatusMsg struct {
	Text  string
	IsErr bool
	Path  string
}

// SnapshotsBackMsg returns to the datasets list for the pool.
type SnapshotsBackMsg struct {
	Dataset string
}

// CreateSnapshotRequestMsg asks App to run CreateSnapshot with the client.
type CreateSnapshotRequestMsg struct {
	FullName  string
	Recursive bool
}

// DestroySnapshotRequestMsg asks App to run DestroySnapshot with the client.
type DestroySnapshotRequestMsg struct {
	FullName  string
	Recursive bool
}

// Snapshots lists snapshots for one dataset and supports create/destroy.
type Snapshots struct {
	Width            int
	Height           int
	Dataset          string
	Prefix           string // snapshot name prefix from config defaults
	RecursiveDefault bool

	snaps   []zfs.Snapshot
	cursor  int
	loading bool
	err     string
	busy    bool // mutation in flight

	// Confirm overlay (create / destroy).
	Confirm Confirm
	// Restore multi-step wizard.
	Restore Restore
}

// SetBusy marks a mutation in flight (used by App after request msgs).
func (s *Snapshots) SetBusy(v bool) { s.busy = v }

// SetLoading marks a list reload in progress.
func (s *Snapshots) SetLoading(v bool) { s.loading = v }

// NewSnapshots builds a snapshots screen for dataset.
func NewSnapshots(dataset, prefix string, recursiveDefault bool) Snapshots {
	return Snapshots{
		Dataset:          dataset,
		Prefix:           prefix,
		RecursiveDefault: recursiveDefault,
		loading:          true,
	}
}

// Path returns status-bar path (dataset name).
func (s Snapshots) Path() string { return s.Dataset }

// LoadSnapshotsCmd fetches snapshots for dataset.
func LoadSnapshotsCmd(z zfs.Client, dataset string) tea.Cmd {
	return func() tea.Msg {
		list, err := z.ListSnapshots(context.Background(), dataset)
		return SnapshotsLoadedMsg{Dataset: dataset, Snapshots: list, Err: err}
	}
}

// CreateSnapshotCmd creates a named snapshot.
func CreateSnapshotCmd(z zfs.Client, fullName string, recursive bool) tea.Cmd {
	return func() tea.Msg {
		err := z.CreateSnapshot(context.Background(), fullName, recursive)
		return SnapshotOpDoneMsg{Op: "create", Name: fullName, Err: err, Path: schedule.DatasetOf(fullName)}
	}
}

// DestroySnapshotCmd destroys a snapshot.
func DestroySnapshotCmd(z zfs.Client, fullName string, recursive bool) tea.Cmd {
	return func() tea.Msg {
		err := z.DestroySnapshot(context.Background(), fullName, recursive)
		return SnapshotOpDoneMsg{Op: "destroy", Name: fullName, Err: err, Path: schedule.DatasetOf(fullName)}
	}
}

// Init implements tea.Model.
func (s Snapshots) Init() tea.Cmd { return nil }

// Update handles load results, confirm, restore wizard, and keys.
func (s Snapshots) Update(msg tea.Msg) (Snapshots, tea.Cmd) {
	// Confirm modal captures all input while active.
	if s.Confirm.Active {
		var cmd tea.Cmd
		s.Confirm, cmd = s.Confirm.Update(msg)
		return s, cmd
	}
	// Restore wizard captures all input while active.
	if s.Restore.Active {
		var cmd tea.Cmd
		s.Restore, cmd = s.Restore.Update(msg)
		return s, cmd
	}

	switch msg := msg.(type) {
	case SnapshotsLoadedMsg:
		s.loading = false
		s.busy = false
		if msg.Dataset != "" {
			s.Dataset = msg.Dataset
		}
		s.snaps = msg.Snapshots
		if s.cursor >= len(s.snaps) {
			s.cursor = max(0, len(s.snaps)-1)
		}
		if msg.Err != nil {
			s.err = msg.Err.Error()
			return s, snapStatus(msg.Err.Error(), true, s.Dataset)
		}
		s.err = ""
		return s, snapStatus(fmt.Sprintf("%d snapshot(s)", len(s.snaps)), false, s.Dataset)

	case ConfirmResultMsg:
		return s.handleConfirm(msg)

	case tea.KeyMsg:
		return s.updateKeys(msg)
	}
	return s, nil
}

func (s Snapshots) handleConfirm(msg ConfirmResultMsg) (Snapshots, tea.Cmd) {
	if !msg.Confirmed {
		return s, snapStatus("cancelled", false, s.Dataset)
	}
	switch msg.ID {
	case "create", "create-recursive":
		// Value is fullName + NUL + "1"/"0" for recursive.
		full, recursive := splitCreateValue(msg.Value)
		s.busy = true
		return s, func() tea.Msg {
			return CreateSnapshotRequestMsg{FullName: full, Recursive: recursive}
		}
	case "destroy":
		s.busy = true
		return s, func() tea.Msg {
			return DestroySnapshotRequestMsg{FullName: msg.Value, Recursive: false}
		}
	}
	return s, nil
}

func (s Snapshots) updateKeys(msg tea.KeyMsg) (Snapshots, tea.Cmd) {
	if s.busy || s.loading {
		// Only allow back while idle-ish; still block mutations.
		if key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))) {
			return s, func() tea.Msg { return SnapshotsBackMsg{Dataset: s.Dataset} }
		}
		return s, nil
	}

	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if s.cursor > 0 {
			s.cursor--
		} else if len(s.snaps) > 0 {
			s.cursor = len(s.snaps) - 1
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if len(s.snaps) == 0 {
			// nop
		} else if s.cursor < len(s.snaps)-1 {
			s.cursor++
		} else {
			s.cursor = 0
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
		return s, func() tea.Msg { return SnapshotsBackMsg{Dataset: s.Dataset} }
	case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
		return s.beginCreate()
	case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
		return s.beginDestroy()
	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		return s.beginRestore()
	}
	return s, nil
}

func (s Snapshots) beginCreate() (Snapshots, tea.Cmd) {
	prefix := s.Prefix
	if prefix == "" {
		prefix = "zfstui"
	}
	snap := schedule.SnapshotName(prefix, time.Now().UTC())
	full := schedule.FullSnapshot(s.Dataset, snap)
	rec := s.RecursiveDefault
	body := fmt.Sprintf("Create snapshot:\n  %s\n\nRecursive: %v\n(Use default from config; confirm applies as shown.)", full, rec)
	val := packCreateValue(full, rec)
	s.Confirm = OpenYesNo("create", "Create snapshot", body, val, false)
	s.Confirm.Width = minInt(s.Width-4, 64)
	return s, nil
}

func (s Snapshots) beginDestroy() (Snapshots, tea.Cmd) {
	if len(s.snaps) == 0 || s.cursor < 0 || s.cursor >= len(s.snaps) {
		return s, snapStatus("no snapshot selected", true, s.Dataset)
	}
	full := s.snaps[s.cursor].Name
	// Require typing the short snapshot name (after @) for safety.
	short := full
	if i := strings.LastIndex(full, "@"); i >= 0 && i+1 < len(full) {
		short = full[i+1:]
	}
	body := fmt.Sprintf("Destroy snapshot permanently:\n  %s", full)
	s.Confirm = OpenType("destroy", "Destroy snapshot", body, short, full)
	s.Confirm.Width = minInt(s.Width-4, 64)
	return s, nil
}

func (s Snapshots) beginRestore() (Snapshots, tea.Cmd) {
	if len(s.snaps) == 0 || s.cursor < 0 || s.cursor >= len(s.snaps) {
		return s, snapStatus("no snapshot selected", true, s.Dataset)
	}
	full := s.snaps[s.cursor].Name
	s.Restore = OpenRestore(full, s.snaps, s.RecursiveDefault)
	s.Restore.Width = minInt(s.Width-4, 64)
	return s, nil
}

func packCreateValue(full string, recursive bool) string {
	flag := "0"
	if recursive {
		flag = "1"
	}
	return full + "\x00" + flag
}

func splitCreateValue(v string) (full string, recursive bool) {
	parts := strings.SplitN(v, "\x00", 2)
	full = parts[0]
	if len(parts) == 2 && parts[1] == "1" {
		recursive = true
	}
	return full, recursive
}

func snapStatus(text string, isErr bool, path string) tea.Cmd {
	return func() tea.Msg {
		return SnapshotsStatusMsg{Text: text, IsErr: isErr, Path: path}
	}
}

// View renders the snapshot list and optional confirm overlay.
func (s Snapshots) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee"))
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#e879f9"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))

	var b strings.Builder
	b.WriteString(title.Render("◆ Snapshots"))
	b.WriteString(accent.Render("  " + s.Dataset))
	b.WriteString(muted.Render("  · c create · d destroy · r restore · esc back"))
	b.WriteString("\n\n")

	if s.loading {
		b.WriteString(muted.Render("  loading…"))
	} else if s.err != "" && len(s.snaps) == 0 {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))
		b.WriteString(errStyle.Render("  " + s.err))
	} else {
		b.WriteString(s.renderTable())
	}

	if s.busy {
		b.WriteString("\n\n")
		b.WriteString(muted.Render("  working…"))
	}

	body := b.String()
	if s.Confirm.Active {
		// Overlay confirm centered-ish under the list.
		panel := s.Confirm.View()
		return body + "\n\n" + lipgloss.NewStyle().PaddingLeft(2).Render(panel)
	}
	if s.Restore.Active {
		panel := s.Restore.View()
		return body + "\n\n" + lipgloss.NewStyle().PaddingLeft(2).Render(panel)
	}
	return body
}

func (s Snapshots) renderTable() string {
	header := tableHeader([]colSpec{
		{Title: "NAME", Width: 48},
		{Title: "USED", Width: 10},
		{Title: "CREATED", Width: 20},
	})
	if len(s.snaps) == 0 {
		return header + "\n" + mutedLine("  (no snapshots)")
	}
	var lines []string
	lines = append(lines, header)
	for i, sn := range s.snaps {
		created := ""
		if !sn.Creation.IsZero() {
			created = sn.Creation.Local().Format("2006-01-02 15:04")
		}
		line := tableRow(
			[]string{sn.Name, sn.Used, created},
			[]int{48, 10, 20},
			i == s.cursor,
		)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func minInt(a, b int) int {
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
