package screens

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zfstui/zfstui/internal/zfs"
)

// Dataset level within the pools/datasets browser.
type datasetLevel int

const (
	levelPools datasetLevel = iota
	levelDatasets
)

// Messages for the datasets browser.

// PoolsLoadedMsg is returned after async ListPools.
type PoolsLoadedMsg struct {
	Pools []zfs.Pool
	Err   error
}

// DatasetsLoadedMsg is returned after async ListDatasets.
type DatasetsLoadedMsg struct {
	Pool     string
	Datasets []zfs.Dataset
	Err      error
}

// OpenSnapshotsMsg requests navigation to the snapshots screen.
type OpenSnapshotsMsg struct {
	Dataset string
}

// DatasetsStatusMsg updates the root status bar from the datasets screen.
type DatasetsStatusMsg struct {
	Text  string
	IsErr bool
	Path  string // pool or pool/dataset path fragment
}

// DatasetsBackMsg requests return to home (from pools list).
type DatasetsBackMsg struct{}

// OpenPoolMsg requests loading datasets for a pool (handled by App).
type OpenPoolMsg struct {
	Pool string
}

// ReloadPoolsMsg requests re-fetching the pools list (handled by App).
type ReloadPoolsMsg struct{}

// Datasets is the pools → datasets browser.
type Datasets struct {
	Width  int
	Height int

	level  datasetLevel
	pool   string
	filter string
	// filtering is true while the user is typing a filter query after "/".
	filtering bool

	pools    []zfs.Pool
	datasets []zfs.Dataset
	cursor   int
	// offset is the first visible list row index (scroll window).
	offset  int
	loading bool
	err     string
}

// Chrome lines reserved before data rows can paint:
// app status+help (2) + title (1) + blank (1) + table header (1).
// Height is the full terminal height (same value App receives).
const datasetsChromeLines = 5

// IsFiltering reports whether the filter input is active.
func (d Datasets) IsFiltering() bool { return d.filtering }

// NewDatasets constructs an empty browser (call InitLoad after wiring).
func NewDatasets() Datasets {
	return Datasets{level: levelPools, loading: true}
}

// Path returns the current breadcrumb path for the status bar.
func (d Datasets) Path() string {
	if d.level == levelDatasets && d.pool != "" {
		return d.pool
	}
	return ""
}

// LoadPoolsCmd fetches pools via the zfs client.
func LoadPoolsCmd(z zfs.Client) tea.Cmd {
	return func() tea.Msg {
		pools, err := z.ListPools(context.Background())
		return PoolsLoadedMsg{Pools: pools, Err: err}
	}
}

// LoadDatasetsCmd fetches datasets under pool.
func LoadDatasetsCmd(z zfs.Client, pool string) tea.Cmd {
	return func() tea.Msg {
		ds, err := z.ListDatasets(context.Background(), pool)
		return DatasetsLoadedMsg{Pool: pool, Datasets: ds, Err: err}
	}
}

// Init implements tea.Model.
func (d Datasets) Init() tea.Cmd { return nil }

// Update handles keys and load results.
func (d Datasets) Update(msg tea.Msg) (Datasets, tea.Cmd) {
	switch msg := msg.(type) {
	case PoolsLoadedMsg:
		d.loading = false
		d.level = levelPools
		d.pool = ""
		d.pools = msg.Pools
		d.cursor = 0
		d.offset = 0
		d.filter = ""
		d.filtering = false
		if msg.Err != nil {
			d.err = msg.Err.Error()
			return d, statusCmd(msg.Err.Error(), true, "")
		}
		d.err = ""
		return d, statusCmd(fmt.Sprintf("%d pool(s)", len(d.pools)), false, "")

	case DatasetsLoadedMsg:
		d.loading = false
		d.level = levelDatasets
		d.pool = msg.Pool
		d.datasets = msg.Datasets
		d.cursor = 0
		d.offset = 0
		d.filter = ""
		d.filtering = false
		if msg.Err != nil {
			d.err = msg.Err.Error()
			return d, statusCmd(msg.Err.Error(), true, msg.Pool)
		}
		d.err = ""
		return d, statusCmd(fmt.Sprintf("%d dataset(s)", len(d.datasets)), false, msg.Pool)

	case tea.KeyMsg:
		if d.filtering {
			return d.updateFilter(msg)
		}
		return d.updateNav(msg)
	}
	return d, nil
}

func (d Datasets) updateFilter(msg tea.KeyMsg) (Datasets, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		d.filtering = false
		d.filter = ""
		d.cursor = 0
		d.offset = 0
		return d, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		d.filtering = false
		d.cursor = 0
		d.offset = 0
		return d, nil
	case key.Matches(msg, key.NewBinding(key.WithKeys("backspace"))):
		if len(d.filter) > 0 {
			d.filter = d.filter[:len(d.filter)-1]
			d.cursor = 0
			d.offset = 0
		}
	case msg.Type == tea.KeyRunes:
		d.filter += string(msg.Runes)
		d.cursor = 0
		d.offset = 0
	}
	return d, nil
}

func (d Datasets) updateNav(msg tea.KeyMsg) (Datasets, tea.Cmd) {
	rows := d.visibleRows()
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if d.cursor > 0 {
			d.cursor--
		} else if len(rows) > 0 {
			d.cursor = len(rows) - 1
		}
		d = d.ensureCursorVisible()
	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if len(rows) == 0 {
			// nothing
		} else if d.cursor < len(rows)-1 {
			d.cursor++
		} else {
			d.cursor = 0
		}
		d = d.ensureCursorVisible()
	case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
		if d.level == levelDatasets {
			d.filtering = true
		}
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
		if d.level == levelDatasets {
			// Back to pools — parent reloads.
			d.level = levelPools
			d.pool = ""
			d.filter = ""
			d.cursor = 0
			d.offset = 0
			d.loading = true
			return d, func() tea.Msg { return ReloadPoolsMsg{} }
		}
		return d, func() tea.Msg { return DatasetsBackMsg{} }
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if len(rows) == 0 || d.cursor < 0 || d.cursor >= len(rows) {
			return d, nil
		}
		name := rows[d.cursor]
		if d.level == levelPools {
			d.loading = true
			d.err = ""
			return d, func() tea.Msg { return OpenPoolMsg{Pool: name} }
		}
		// Drill into snapshots.
		return d, func() tea.Msg { return OpenSnapshotsMsg{Dataset: name} }
	}
	return d, nil
}

// listPageSize is how many data rows fit in the terminal viewport for n total rows.
// When the list overflows, one line is reserved for the "showing N–M of T" hint.
func (d Datasets) listPageSize(n int) int {
	h := d.Height - datasetsChromeLines
	if h < 1 {
		h = 1
	}
	if n > h && h > 1 {
		h-- // room for scrollHint
	}
	return h
}

// ensureCursorVisible adjusts offset so the cursor row stays on-screen.
func (d Datasets) ensureCursorVisible() Datasets {
	n := len(d.visibleRows())
	page := d.listPageSize(n)
	if n <= 0 {
		d.offset = 0
		d.cursor = 0
		return d
	}
	if d.cursor < 0 {
		d.cursor = 0
	}
	if d.cursor >= n {
		d.cursor = n - 1
	}
	if d.offset < 0 {
		d.offset = 0
	}
	if d.cursor < d.offset {
		d.offset = d.cursor
	}
	if d.cursor >= d.offset+page {
		d.offset = d.cursor - page + 1
	}
	maxOff := n - page
	if maxOff < 0 {
		maxOff = 0
	}
	if d.offset > maxOff {
		d.offset = maxOff
	}
	return d
}

// windowBounds returns the half-open [start, end) range of rows to paint.
func (d Datasets) windowBounds(n int) (start, end int) {
	if n <= 0 {
		return 0, 0
	}
	page := d.listPageSize(n)
	start = d.offset
	if start < 0 {
		start = 0
	}
	if start >= n {
		start = 0
	}
	end = start + page
	if end > n {
		end = n
	}
	return start, end
}

func statusCmd(text string, isErr bool, path string) tea.Cmd {
	return func() tea.Msg {
		return DatasetsStatusMsg{Text: text, IsErr: isErr, Path: path}
	}
}

func (d Datasets) visibleRows() []string {
	switch d.level {
	case levelPools:
		var names []string
		for _, p := range d.pools {
			names = append(names, p.Name)
		}
		return names
	case levelDatasets:
		q := strings.ToLower(d.filter)
		var names []string
		for _, ds := range d.datasets {
			if q != "" && !strings.Contains(strings.ToLower(ds.Name), q) {
				continue
			}
			names = append(names, ds.Name)
		}
		return names
	}
	return nil
}

// View renders the browser table.
func (d Datasets) View() string {
	var b strings.Builder

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#e879f9"))

	switch d.level {
	case levelPools:
		b.WriteString(title.Render("◆ Pools"))
		b.WriteString(muted.Render("  · enter open · esc home"))
	case levelDatasets:
		b.WriteString(title.Render("◆ Datasets"))
		b.WriteString(accent.Render("  " + d.pool))
		if d.filter != "" || d.filtering {
			b.WriteString(muted.Render("  filter: "))
			filtStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24"))
			if d.filtering {
				b.WriteString(filtStyle.Render(d.filter + "▌"))
			} else {
				b.WriteString(filtStyle.Render(d.filter))
			}
		}
	}
	b.WriteString("\n\n")

	if d.loading {
		b.WriteString(muted.Render("  loading…"))
		return b.String()
	}
	if d.err != "" && len(d.visibleRows()) == 0 {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))
		b.WriteString(errStyle.Render("  " + d.err))
		return b.String()
	}

	switch d.level {
	case levelPools:
		b.WriteString(d.renderPools())
	case levelDatasets:
		b.WriteString(d.renderDatasets())
	}
	return b.String()
}

func (d Datasets) renderPools() string {
	header := tableHeader([]colSpec{
		{Title: "NAME", Width: 24},
		{Title: "HEALTH", Width: 10},
		{Title: "SIZE", Width: 10},
		{Title: "ALLOC", Width: 10},
	})
	rows := d.visibleRows()
	if len(rows) == 0 {
		return header + "\n" + mutedLine("  (no pools)")
	}
	// Index pools by name for column data.
	byName := map[string]zfs.Pool{}
	for _, p := range d.pools {
		byName[p.Name] = p
	}
	start, end := d.windowBounds(len(rows))
	var lines []string
	lines = append(lines, header)
	for i := start; i < end; i++ {
		p := byName[rows[i]]
		line := tableRow([]string{p.Name, p.Health, p.Size, p.Alloc}, []int{24, 10, 10, 10}, i == d.cursor)
		lines = append(lines, line)
	}
	if start > 0 || end < len(rows) {
		lines = append(lines, scrollHint(start, end, len(rows)))
	}
	return strings.Join(lines, "\n")
}

func (d Datasets) renderDatasets() string {
	header := tableHeader([]colSpec{
		{Title: "NAME", Width: 36},
		{Title: "USED", Width: 8},
		{Title: "AVAIL", Width: 8},
		{Title: "TYPE", Width: 10},
		{Title: "MOUNTPOINT", Width: 20},
	})
	rows := d.visibleRows()
	if len(rows) == 0 {
		msg := "  (no datasets)"
		if d.filter != "" {
			msg = "  (no match for filter)"
		}
		return header + "\n" + mutedLine(msg)
	}
	byName := map[string]zfs.Dataset{}
	for _, ds := range d.datasets {
		byName[ds.Name] = ds
	}
	start, end := d.windowBounds(len(rows))
	var lines []string
	lines = append(lines, header)
	for i := start; i < end; i++ {
		ds := byName[rows[i]]
		line := tableRow(
			[]string{ds.Name, ds.Used, ds.Avail, ds.Type, ds.Mountpoint},
			[]int{36, 8, 8, 10, 20},
			i == d.cursor,
		)
		lines = append(lines, line)
	}
	if start > 0 || end < len(rows) {
		lines = append(lines, scrollHint(start, end, len(rows)))
	}
	return strings.Join(lines, "\n")
}

func scrollHint(start, end, total int) string {
	// 1-based range for humans: "showing 6–15 of 40"
	return mutedLine(fmt.Sprintf("  showing %d–%d of %d", start+1, end, total))
}

// --- shared table helpers (used by snapshots too) ---

type colSpec struct {
	Title string
	Width int
}

func tableHeader(cols []colSpec) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Bold(true)
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = padTruncate(c.Title, c.Width)
	}
	return "  " + style.Render(strings.Join(parts, "  "))
}

func tableRow(cells []string, widths []int, selected bool) string {
	parts := make([]string, len(cells))
	for i, cell := range cells {
		w := 12
		if i < len(widths) {
			w = widths[i]
		}
		parts[i] = padTruncate(cell, w)
	}
	content := strings.Join(parts, "  ")
	if selected {
		cursor := lipgloss.NewStyle().Foreground(lipgloss.Color("#22d3ee")).Bold(true).Render("▸ ")
		sel := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0d1117")).
			Background(lipgloss.Color("#22d3ee")).
			Bold(true)
		return cursor + sel.Render(content)
	}
	return "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#e6edf3")).Render(content)
}

func padTruncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	// Use rune-aware-ish via lipgloss width when possible; ASCII-heavy ZFS names.
	if len(s) > w {
		if w <= 1 {
			return s[:w]
		}
		return s[:w-1] + "…"
	}
	return s + strings.Repeat(" ", w-len(s))
}

func mutedLine(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Render(s)
}
