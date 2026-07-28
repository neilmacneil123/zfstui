package screens

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zfstui/zfstui/internal/restore"
	"github.com/zfstui/zfstui/internal/schedule"
	"github.com/zfstui/zfstui/internal/zfs"
)

// Restore step indices for the multi-step wizard.
const (
	restoreStepMode = iota // choose rollback vs clone
	restoreStepRollback    // list newer snaps + type dataset name
	restoreStepClonePath   // enter new dataset path
	restoreStepClonePromo  // promote y/n
	restoreStepCloneFinal  // final yes/no confirm
)

// RestoreRequestMsg asks App to run restore.Service.
type RestoreRequestMsg struct {
	Op         string // "rollback" or "clone"
	Snapshot   string
	NewDataset string // clone only
	Promote    bool   // clone only
	Recursive  bool   // rollback only
}

// RestoreOpDoneMsg is returned after a restore operation completes.
type RestoreOpDoneMsg struct {
	Op       string
	Snapshot string
	Target   string // dataset affected / clone path
	Err      error
	Path     string // breadcrumb / reload dataset
}

// RestoreStatusMsg updates the root status bar from the wizard.
type RestoreStatusMsg struct {
	Text  string
	IsErr bool
	Path  string
}

// Restore is a multi-step wizard: Rollback vs Clone, then path/confirm.
type Restore struct {
	Active    bool
	Width     int
	Snapshot  string // full snapshot name (pool/ds@snap)
	Dataset   string // dataset of snapshot
	AllSnaps  []zfs.Snapshot
	Recursive bool

	step  int
	mode  string // "rollback" or "clone"
	newer []zfs.Snapshot

	// rollback typed confirm
	typeInput string

	// clone fields
	pathInput string
	promote   bool

	// yes/no selection (promote + final confirm)
	yesSelected bool

	err string
}

// OpenRestore starts the wizard for the given snapshot.
func OpenRestore(snapshot string, all []zfs.Snapshot, recursive bool) Restore {
	ds := schedule.DatasetOf(snapshot)
	return Restore{
		Active:    true,
		Snapshot:  snapshot,
		Dataset:   ds,
		AllSnaps:  all,
		Recursive: recursive,
		step:      restoreStepMode,
		newer:     restore.NewerSnapshots(all, snapshot),
		pathInput: ds + "-restored",
	}
}

// Init implements tea.Model.
func (r Restore) Init() tea.Cmd { return nil }

// Update handles wizard keys. When inactive, returns unchanged.
func (r Restore) Update(msg tea.Msg) (Restore, tea.Cmd) {
	if !r.Active {
		return r, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return r.updateKey(msg)
	}
	return r, nil
}

func (r Restore) updateKey(msg tea.KeyMsg) (Restore, tea.Cmd) {
	// Global cancel for all steps.
	if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
		r.Active = false
		return r, restoreStatus("restore cancelled", false, r.Dataset)
	}

	switch r.step {
	case restoreStepMode:
		return r.updateMode(msg)
	case restoreStepRollback:
		return r.updateRollback(msg)
	case restoreStepClonePath:
		return r.updateClonePath(msg)
	case restoreStepClonePromo:
		return r.updateYesNo(msg, true)
	case restoreStepCloneFinal:
		return r.updateYesNo(msg, false)
	}
	return r, nil
}

func (r Restore) updateMode(msg tea.KeyMsg) (Restore, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("r", "R", "1"))):
		r.mode = "rollback"
		r.step = restoreStepRollback
		r.typeInput = ""
		r.err = ""
		return r, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("c", "C", "2"))):
		r.mode = "clone"
		r.step = restoreStepClonePath
		r.err = ""
		return r, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k", "down", "j", "tab"))):
		// cosmetic: nothing to toggle between two fixed choices via selection UI
		return r, nil
	}
	return r, nil
}

func (r Restore) updateRollback(msg tea.KeyMsg) (Restore, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if r.typeInput != r.Dataset {
			r.err = fmt.Sprintf("type exact dataset name %q to confirm", r.Dataset)
			return r, nil
		}
		r.Active = false
		snap := r.Snapshot
		rec := r.Recursive
		return r, func() tea.Msg {
			return RestoreRequestMsg{
				Op:        "rollback",
				Snapshot:  snap,
				Recursive: rec,
			}
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("backspace"))):
		if len(r.typeInput) > 0 {
			r.typeInput = r.typeInput[:len(r.typeInput)-1]
		}
		r.err = ""
	case msg.Type == tea.KeyRunes:
		r.typeInput += string(msg.Runes)
		r.err = ""
	}
	return r, nil
}

func (r Restore) updateClonePath(msg tea.KeyMsg) (Restore, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		path := strings.TrimSpace(r.pathInput)
		if path == "" {
			r.err = "new dataset path is required"
			return r, nil
		}
		if path == r.Dataset {
			r.err = "clone path must differ from source dataset"
			return r, nil
		}
		if strings.Contains(path, "@") {
			r.err = "path must be a dataset, not a snapshot"
			return r, nil
		}
		r.pathInput = path
		r.step = restoreStepClonePromo
		r.yesSelected = false // default: do not promote
		r.promote = false
		r.err = ""
		return r, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("backspace"))):
		if len(r.pathInput) > 0 {
			r.pathInput = r.pathInput[:len(r.pathInput)-1]
		}
		r.err = ""
	case msg.Type == tea.KeyRunes:
		r.pathInput += string(msg.Runes)
		r.err = ""
	}
	return r, nil
}

// updateYesNo handles promote (isPromote=true) or final clone confirm.
func (r Restore) updateYesNo(msg tea.KeyMsg, isPromote bool) (Restore, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h", "right", "l", "tab"))):
		r.yesSelected = !r.yesSelected
	case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
		r.yesSelected = true
		return r.advanceYesNo(isPromote, true)
	case key.Matches(msg, key.NewBinding(key.WithKeys("n", "N"))):
		r.yesSelected = false
		return r.advanceYesNo(isPromote, false)
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter", " "))):
		return r.advanceYesNo(isPromote, r.yesSelected)
	}
	return r, nil
}

func (r Restore) advanceYesNo(isPromote bool, yes bool) (Restore, tea.Cmd) {
	if isPromote {
		r.promote = yes
		r.step = restoreStepCloneFinal
		r.yesSelected = false // default No on destructive-ish final confirm until reviewed
		// Final confirm defaults to Yes when not promote-only — still cautious default No.
		r.err = ""
		return r, nil
	}
	// Final confirm
	if !yes {
		r.Active = false
		return r, restoreStatus("restore cancelled", false, r.Dataset)
	}
	r.Active = false
	snap := r.Snapshot
	path := r.pathInput
	promote := r.promote
	return r, func() tea.Msg {
		return RestoreRequestMsg{
			Op:         "clone",
			Snapshot:   snap,
			NewDataset: path,
			Promote:    promote,
		}
	}
}

func restoreStatus(text string, isErr bool, path string) tea.Cmd {
	return func() tea.Msg {
		return RestoreStatusMsg{Text: text, IsErr: isErr, Path: path}
	}
}

// RestoreCmd executes restore.Service via the zfs client (respects dry-run on *zfs.CLI).
func RestoreCmd(z zfs.Client, req RestoreRequestMsg) tea.Cmd {
	return func() tea.Msg {
		svc := &restore.Service{Z: z}
		var err error
		target := schedule.DatasetOf(req.Snapshot)
		switch req.Op {
		case "rollback":
			err = svc.Rollback(context.Background(), req.Snapshot, req.Recursive)
		case "clone":
			target = req.NewDataset
			err = svc.Clone(context.Background(), req.Snapshot, req.NewDataset, req.Promote)
		default:
			err = fmt.Errorf("unknown restore op %q", req.Op)
		}
		return RestoreOpDoneMsg{
			Op:       req.Op,
			Snapshot: req.Snapshot,
			Target:   target,
			Err:      err,
			Path:     schedule.DatasetOf(req.Snapshot),
		}
	}
}

// View renders the wizard panel.
func (r Restore) View() string {
	if !r.Active {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee"))
	dangerTitle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f87171"))
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#e879f9"))
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))

	var lines []string
	switch r.step {
	case restoreStepMode:
		lines = append(lines, titleStyle.Render("Restore wizard"))
		lines = append(lines, "")
		lines = append(lines, bodyStyle.Render("Snapshot:"))
		lines = append(lines, accent.Render("  "+r.Snapshot))
		lines = append(lines, "")
		lines = append(lines, bodyStyle.Render("Choose restore mode:"))
		lines = append(lines, "")
		lines = append(lines, "  "+warn.Render("[r]")+"  Rollback  — restore in place (destroys newer snaps)")
		lines = append(lines, "  "+warn.Render("[c]")+"  Clone     — new dataset from this snapshot")
		lines = append(lines, "")
		lines = append(lines, muted.Render("r/c select · esc cancel"))

	case restoreStepRollback:
		lines = append(lines, dangerTitle.Render("Rollback — destructive"))
		lines = append(lines, "")
		lines = append(lines, bodyStyle.Render("Will roll back to:"))
		lines = append(lines, accent.Render("  "+r.Snapshot))
		if r.Recursive {
			lines = append(lines, muted.Render("  (recursive)"))
		}
		lines = append(lines, "")
		if len(r.newer) == 0 {
			lines = append(lines, muted.Render("No newer snapshots will be destroyed."))
		} else {
			lines = append(lines, warn.Render(fmt.Sprintf("%d newer snapshot(s) will be destroyed:", len(r.newer))))
			maxShow := 8
			for i, sn := range r.newer {
				if i >= maxShow {
					lines = append(lines, muted.Render(fmt.Sprintf("  … and %d more", len(r.newer)-maxShow)))
					break
				}
				lines = append(lines, bodyStyle.Render("  · "+sn.Name))
			}
		}
		lines = append(lines, "")
		prompt := muted.Render("Type ") +
			warn.Bold(true).Render(r.Dataset) +
			muted.Render(" to confirm:")
		lines = append(lines, prompt)
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e6edf3")).
			Background(lipgloss.Color("#161b22")).
			Padding(0, 1)
		cursor := lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee")).Render("▌")
		lines = append(lines, inputStyle.Render(r.typeInput)+cursor)
		if r.err != "" {
			lines = append(lines, errStyle.Render(r.err))
		}
		lines = append(lines, "")
		lines = append(lines, muted.Render("enter confirm · esc cancel"))

	case restoreStepClonePath:
		lines = append(lines, titleStyle.Render("Clone to new dataset"))
		lines = append(lines, "")
		lines = append(lines, bodyStyle.Render("Source:"))
		lines = append(lines, accent.Render("  "+r.Snapshot))
		lines = append(lines, "")
		lines = append(lines, muted.Render("New dataset path:"))
		inputStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e6edf3")).
			Background(lipgloss.Color("#161b22")).
			Padding(0, 1)
		cursor := lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee")).Render("▌")
		lines = append(lines, inputStyle.Render(r.pathInput)+cursor)
		if r.err != "" {
			lines = append(lines, errStyle.Render(r.err))
		}
		lines = append(lines, "")
		lines = append(lines, muted.Render("enter next · esc cancel"))

	case restoreStepClonePromo:
		lines = append(lines, titleStyle.Render("Promote clone?"))
		lines = append(lines, "")
		lines = append(lines, bodyStyle.Render("Clone to: "+accent.Render(r.pathInput)))
		lines = append(lines, "")
		lines = append(lines, muted.Render("Promote makes the clone independent of the origin snapshot."))
		lines = append(lines, "")
		lines = append(lines, r.renderYesNo())
		lines = append(lines, "")
		lines = append(lines, muted.Render("y/n · enter · esc cancel"))

	case restoreStepCloneFinal:
		lines = append(lines, titleStyle.Render("Confirm clone"))
		lines = append(lines, "")
		lines = append(lines, bodyStyle.Render("Snapshot: ")+accent.Render(r.Snapshot))
		lines = append(lines, bodyStyle.Render("New dataset: ")+accent.Render(r.pathInput))
		promo := "no"
		if r.promote {
			promo = "yes"
		}
		lines = append(lines, bodyStyle.Render("Promote: ")+accent.Render(promo))
		lines = append(lines, "")
		lines = append(lines, r.renderYesNo())
		lines = append(lines, "")
		lines = append(lines, muted.Render("y/n · enter · esc cancel"))
	}

	w := r.Width
	if w < 40 {
		w = 52
	}
	if w > 72 {
		w = 72
	}
	border := lipgloss.Color("#e879f9")
	if r.step == restoreStepRollback {
		border = lipgloss.Color("#f87171")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(1, 2).
		Width(w).
		Render(strings.Join(lines, "\n"))
}

func (r Restore) renderYesNo() string {
	yes := " Yes "
	no := " No "
	sel := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0d1117")).
		Background(lipgloss.Color("#22d3ee")).
		Bold(true)
	unsel := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Padding(0, 1)
	if r.yesSelected {
		yes = sel.Render(yes)
		no = unsel.Render(no)
	} else {
		yes = unsel.Render(yes)
		no = sel.Render(no)
	}
	return yes + "  " + no
}
