package screens

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zfstui/zfstui/internal/config"
	cronpkg "github.com/zfstui/zfstui/internal/cron"
	"github.com/zfstui/zfstui/internal/zfs"
)

// Compatibility messages retained for integrations using the old browser.
type PoolsLoadedMsg struct {
	Pools []zfs.Pool
	Err   error
}
type DatasetsLoadedMsg struct {
	Pool     string
	Datasets []zfs.Dataset
	Err      error
}
type OpenPoolMsg struct{ Pool string }
type ReloadPoolsMsg struct{}
type OpenSnapshotsMsg struct{ Dataset string }
type DatasetsBackMsg struct{}
type DatasetsStatusMsg struct {
	Text  string
	IsErr bool
	Path  string
}

type DashboardLoadedMsg struct {
	Pools                               []zfs.Pool
	Datasets                            []zfs.Dataset
	Snapshots                           []zfs.Snapshot
	Cron                                cronpkg.Status
	PoolsErr, DatasetsErr, SnapshotsErr error
}
type BulkDestroyRequestMsg struct{ Snapshots []zfs.Snapshot }
type BulkDestroyResultMsg struct {
	Succeeded, Failed, Simulated []string
	Errors                       map[string]error
}

type nodeKind int

const (
	nodePool nodeKind = iota
	nodeDataset
	nodeSnapshot
)

type datasetLevel int

const (
	levelPools datasetLevel = iota
	levelDatasets
)

type treeNode struct {
	kind                       nodeKind
	key, name, parent, dataset string
	depth, count               int
	bytes                      uint64
	newest, creation           time.Time
	direct, inherited          []config.Schedule
}

// Datasets is the host-wide pool → dataset → snapshot dashboard.
type Datasets struct {
	Width, Height                 int
	pools                         []zfs.Pool
	datasets                      []zfs.Dataset
	snapshots                     []zfs.Snapshot
	cfg                           *config.Config
	cron                          cronpkg.Status
	errors                        []string
	expanded, marked, failedMarks map[string]bool
	cursor, offset                int
	filter                        string
	filtering, loading, busy      bool
	Confirm                       Confirm
	level                         datasetLevel
	pool                          string
	legacy                        bool
}

const datasetsChromeLines = 7

func NewDatasets(cfg ...*config.Config) Datasets {
	var c *config.Config
	if len(cfg) > 0 {
		c = cfg[0]
	}
	return Datasets{cfg: c, loading: true, expanded: map[string]bool{}, marked: map[string]bool{}, failedMarks: map[string]bool{}}
}
func (d Datasets) Init() tea.Cmd       { return nil }
func (d Datasets) IsFiltering() bool   { return d.filtering }
func (d Datasets) InputCaptured() bool { return d.filtering || d.Confirm.Active }
func (d Datasets) Path() string {
	if n := d.selected(); n != nil {
		return n.dataset
	}
	return ""
}

func LoadPoolsCmd(z zfs.Client) tea.Cmd {
	return func() tea.Msg { p, e := z.ListPools(context.Background()); return PoolsLoadedMsg{p, e} }
}
func LoadDatasetsCmd(z zfs.Client, pool string) tea.Cmd {
	return func() tea.Msg {
		ds, e := z.ListDatasets(context.Background(), pool)
		return DatasetsLoadedMsg{pool, ds, e}
	}
}
func LoadDashboardCmd(z zfs.Client, cfg *config.Config, binary, cfgPath string) tea.Cmd {
	return func() tea.Msg {
		type poolsResult struct {
			v   []zfs.Pool
			err error
		}
		type datasetsResult struct {
			v   []zfs.Dataset
			err error
		}
		type snapshotsResult struct {
			v   []zfs.Snapshot
			err error
		}
		pc := make(chan poolsResult, 1)
		dc := make(chan datasetsResult, 1)
		sc := make(chan snapshotsResult, 1)
		cc := make(chan cronpkg.Status, 1)
		go func() { v, e := z.ListPools(context.Background()); pc <- poolsResult{v, e} }()
		go func() { v, e := z.ListDatasets(context.Background(), ""); dc <- datasetsResult{v, e} }()
		go func() { v, e := z.ListSnapshots(context.Background(), ""); sc <- snapshotsResult{v, e} }()
		go func() {
			if cfg == nil {
				cc <- cronpkg.Status{Mode: "none", Err: fmt.Errorf("no config loaded")}
				return
			}
			bin := binary
			if bin == "" {
				bin = "zfstui"
			}
			cc <- cronpkg.Inspect(cfg, bin, cfgPath, os.Geteuid() == 0)
		}()
		pr, dr, sr := <-pc, <-dc, <-sc
		m := DashboardLoadedMsg{}
		m.Pools, m.PoolsErr = pr.v, pr.err
		m.Datasets, m.DatasetsErr = dr.v, dr.err
		m.Snapshots, m.SnapshotsErr = sr.v, sr.err
		m.Cron = <-cc
		return m
	}
}
func BulkDestroyCmd(z zfs.Client, snaps []zfs.Snapshot, dryRun bool) tea.Cmd {
	list := append([]zfs.Snapshot(nil), snaps...)
	sort.SliceStable(list, func(i, j int) bool {
		di, dj := snapshotDataset(list[i].Name), snapshotDataset(list[j].Name)
		if di != dj {
			return di < dj
		}
		if !list[i].Creation.Equal(list[j].Creation) {
			return list[i].Creation.Before(list[j].Creation)
		}
		return list[i].Name < list[j].Name
	})
	return func() tea.Msg {
		r := BulkDestroyResultMsg{Errors: map[string]error{}}
		for _, s := range list {
			if e := z.DestroySnapshot(context.Background(), s.Name, false); e != nil {
				r.Failed = append(r.Failed, s.Name)
				r.Errors[s.Name] = e
			} else if dryRun {
				r.Simulated = append(r.Simulated, s.Name)
			} else {
				r.Succeeded = append(r.Succeeded, s.Name)
			}
		}
		return r
	}
}

func (d Datasets) Update(msg tea.Msg) (Datasets, tea.Cmd) {
	if d.Confirm.Active {
		var c tea.Cmd
		d.Confirm, c = d.Confirm.Update(msg)
		return d, c
	}
	switch m := msg.(type) {
	case DashboardLoadedMsg:
		selectedKey := ""
		if n := d.selected(); n != nil {
			selectedKey = n.key
		}
		d.loading, d.busy = false, false
		d.pools, d.datasets, d.snapshots, d.cron = m.Pools, m.Datasets, m.Snapshots, m.Cron
		d.errors = nil
		for label, e := range map[string]error{"pools": m.PoolsErr, "datasets": m.DatasetsErr, "snapshots": m.SnapshotsErr} {
			if e != nil {
				d.errors = append(d.errors, label+": "+e.Error())
			}
		}
		sort.Strings(d.errors)
		d.retainMarks()
		d.clamp()
		if selectedKey != "" {
			d.selectKey(selectedKey)
			d = d.ensureCursorVisible()
		}
		return d, statusCmd(d.summaryStatus(), len(d.errors) > 0, "")
	case PoolsLoadedMsg:
		d.level = levelPools
		d.legacy = true
		return d.Update(DashboardLoadedMsg{Pools: m.Pools, PoolsErr: m.Err})
	case DatasetsLoadedMsg:
		d.level, d.pool = levelDatasets, m.Pool
		d.legacy = true
		d.datasets = m.Datasets
		if m.Pool != "" {
			found := false
			for _, p := range d.pools {
				if p.Name == m.Pool {
					found = true
				}
			}
			if !found {
				d.pools = append(d.pools, zfs.Pool{Name: m.Pool})
			}
			d.expanded["pool:"+m.Pool] = true
		}
		d.loading = false
		if m.Err != nil {
			d.errors = []string{"datasets: " + m.Err.Error()}
		}
		d.clamp()
		return d, nil
	case ConfirmResultMsg:
		if m.ID == "bulk-destroy" && m.Confirmed {
			t := d.targets()
			d.busy = true
			return d, func() tea.Msg { return BulkDestroyRequestMsg{Snapshots: t} }
		}
		return d, nil
	case tea.KeyMsg:
		if d.filtering {
			return d.updateFilter(m)
		}
		return d.updateNav(m)
	}
	return d, nil
}

func (d Datasets) updateFilter(m tea.KeyMsg) (Datasets, tea.Cmd) {
	switch {
	case key.Matches(m, key.NewBinding(key.WithKeys("esc"))):
		d.filtering = false
		d.filter = ""
	case key.Matches(m, key.NewBinding(key.WithKeys("enter"))):
		d.filtering = false
	case key.Matches(m, key.NewBinding(key.WithKeys("backspace"))):
		if len(d.filter) > 0 {
			d.filter = d.filter[:len(d.filter)-1]
		}
	case m.Type == tea.KeyRunes:
		d.filter += string(m.Runes)
	}
	d.cursor, d.offset = 0, 0
	return d, nil
}
func (d Datasets) updateNav(m tea.KeyMsg) (Datasets, tea.Cmd) {
	rowCount := len(d.visibleRows())
	switch {
	case key.Matches(m, key.NewBinding(key.WithKeys("up", "k"))):
		if rowCount > 0 {
			d.cursor = (d.cursor - 1 + rowCount) % rowCount
		}
	case key.Matches(m, key.NewBinding(key.WithKeys("down", "j"))):
		if rowCount > 0 {
			d.cursor = (d.cursor + 1) % rowCount
		}
	case key.Matches(m, key.NewBinding(key.WithKeys("/"))):
		d.filtering = true
	case key.Matches(m, key.NewBinding(key.WithKeys("R"))):
		d.loading = true
		return d, func() tea.Msg { return ReloadPoolsMsg{} }
	case key.Matches(m, key.NewBinding(key.WithKeys("esc", "backspace"))):
		return d, func() tea.Msg { return DatasetsBackMsg{} }
	case key.Matches(m, key.NewBinding(key.WithKeys("enter", "right", "l"))):
		if n := d.selected(); n != nil && n.kind != nodeSnapshot {
			d.expanded[n.key] = true
		}
	case key.Matches(m, key.NewBinding(key.WithKeys("left", "h"))):
		if n := d.selected(); n != nil {
			if d.expanded[n.key] {
				d.expanded[n.key] = false
			} else if n.parent != "" {
				d.selectKey(n.parent)
			}
		}
	case key.Matches(m, key.NewBinding(key.WithKeys(" "))):
		if n := d.selected(); n != nil && n.kind == nodeSnapshot {
			d.marked[n.key] = !d.marked[n.key]
			delete(d.failedMarks, n.key)
		}
	case key.Matches(m, key.NewBinding(key.WithKeys("d"))):
		return d.beginDelete()
	case key.Matches(m, key.NewBinding(key.WithKeys("o"))):
		if n := d.selected(); n != nil && n.dataset != "" {
			return d, func() tea.Msg { return OpenSnapshotsMsg{Dataset: n.dataset} }
		}
	}
	d = d.ensureCursorVisible()
	return d, nil
}

func (d Datasets) beginDelete() (Datasets, tea.Cmd) {
	t := d.targets()
	if len(t) == 0 {
		return d, statusCmd("select a snapshot to delete", true, d.Path())
	}
	sets := map[string]bool{}
	var total uint64
	var preview []string
	for _, s := range t {
		sets[snapshotDataset(s.Name)] = true
		total += s.UsedBytes
		if len(preview) < 6 {
			preview = append(preview, "  "+s.Name)
		}
	}
	if len(t) > len(preview) {
		preview = append(preview, fmt.Sprintf("  … and %d more", len(t)-len(preview)))
	}
	body := fmt.Sprintf("%d snapshot(s) across %d dataset(s)\n%s snapshot unique used\n%s", len(t), len(sets), formatBytes(total), strings.Join(preview, "\n"))
	d.Confirm = OpenType("bulk-destroy", "Delete snapshots", body, fmt.Sprintf("DELETE %d", len(t)), "")
	d.Confirm.Width = minInt(d.Width-4, 72)
	return d, nil
}
func (d Datasets) targets() []zfs.Snapshot {
	marks := d.marked
	if lenTrue(marks) == 0 {
		marks = map[string]bool{}
		if n := d.selected(); n != nil && n.kind == nodeSnapshot {
			marks[n.key] = true
		}
	}
	var out []zfs.Snapshot
	for _, s := range d.snapshots {
		if marks[s.Name] {
			out = append(out, s)
		}
	}
	return out
}
func (d *Datasets) ApplyBulkResult(r BulkDestroyResultMsg) {
	d.busy = false
	d.failedMarks = map[string]bool{}
	d.marked = map[string]bool{}
	for _, n := range r.Failed {
		d.marked[n] = true
		d.failedMarks[n] = true
	}
}

func (d Datasets) tree() []treeNode {
	poolMap := map[string]*treeNode{}
	dsMap := map[string]*treeNode{}
	for _, p := range d.pools {
		poolMap[p.Name] = &treeNode{kind: nodePool, key: "pool:" + p.Name, name: p.Name}
	}
	for _, ds := range d.datasets {
		pool := strings.SplitN(ds.Name, "/", 2)[0]
		if poolMap[pool] == nil {
			poolMap[pool] = &treeNode{kind: nodePool, key: "pool:" + pool, name: pool}
		}
		parent := "pool:" + pool
		depth := 1
		if i := strings.LastIndex(ds.Name, "/"); i >= 0 {
			parent = "ds:" + ds.Name[:i]
			depth = strings.Count(ds.Name, "/") + 1
		}
		dsMap[ds.Name] = &treeNode{kind: nodeDataset, key: "ds:" + ds.Name, name: ds.Name, parent: parent, dataset: ds.Name, depth: depth}
	}
	// Snapshot inventory remains useful even when dataset listing failed or a
	// dataset disappeared between the independent host-wide reads.
	for _, s := range d.snapshots {
		ds := snapshotDataset(s.Name)
		if ds == "" || dsMap[ds] != nil {
			continue
		}
		pool := strings.SplitN(ds, "/", 2)[0]
		if poolMap[pool] == nil {
			poolMap[pool] = &treeNode{kind: nodePool, key: "pool:" + pool, name: pool}
		}
		parent, depth := "pool:"+pool, 1
		if i := strings.LastIndex(ds, "/"); i >= 0 {
			parent = "ds:" + ds[:i]
			depth = strings.Count(ds, "/") + 1
		}
		dsMap[ds] = &treeNode{kind: nodeDataset, key: "ds:" + ds, name: ds, parent: parent, dataset: ds, depth: depth}
	}
	for _, n := range dsMap {
		if strings.HasPrefix(n.parent, "ds:") && dsMap[strings.TrimPrefix(n.parent, "ds:")] == nil {
			n.parent = "pool:" + strings.SplitN(n.dataset, "/", 2)[0]
			n.depth = 1
		}
	}
	for _, s := range d.snapshots {
		ds := snapshotDataset(s.Name)
		n := dsMap[ds]
		if n == nil {
			continue
		}
		n.count++
		n.bytes += s.UsedBytes
		if s.Creation.After(n.newest) {
			n.newest = s.Creation
		}
	}
	for _, n := range dsMap {
		n.direct, n.inherited = d.schedulesFor(n.dataset)
		p := poolMap[strings.SplitN(n.dataset, "/", 2)[0]]
		p.count += n.count
		p.bytes += n.bytes
	}
	var all []treeNode
	for _, n := range poolMap {
		all = append(all, *n)
	}
	for _, n := range dsMap {
		all = append(all, *n)
	}
	for _, s := range d.snapshots {
		ds := snapshotDataset(s.Name)
		dn := dsMap[ds]
		if dn == nil {
			continue
		}
		short := s.Name
		if i := strings.Index(short, "@"); i >= 0 {
			short = short[i+1:]
		}
		all = append(all, treeNode{kind: nodeSnapshot, key: s.Name, name: short, parent: "ds:" + ds, dataset: ds, depth: dn.depth + 1, bytes: s.UsedBytes, creation: s.Creation})
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		pa, pb := treePath(a), treePath(b)
		if pa != pb {
			return pa < pb
		}
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		if a.kind == nodeSnapshot && !a.creation.Equal(b.creation) {
			return a.creation.Before(b.creation)
		}
		return a.key < b.key
	})
	return all
}
func treePath(n treeNode) string {
	if n.kind == nodePool {
		return n.name
	}
	if n.kind == nodeDataset {
		return n.dataset + "!"
	}
	return n.dataset + "@" + n.name
}
func (d Datasets) schedulesFor(dataset string) (direct, inherited []config.Schedule) {
	if d.cfg == nil {
		return
	}
	for _, s := range d.cfg.Schedules {
		if !s.Enabled {
			continue
		}
		if s.Dataset == dataset {
			direct = append(direct, s)
		} else if s.Recursive && strings.HasPrefix(dataset, s.Dataset+"/") {
			inherited = append(inherited, s)
		}
	}
	return
}
func (d Datasets) visibleNodes() []treeNode {
	all := d.tree()
	q := strings.ToLower(d.filter)
	matches := map[string]bool{}
	if q != "" {
		by := map[string]treeNode{}
		for _, n := range all {
			by[n.key] = n
		}
		for _, n := range all {
			if strings.Contains(strings.ToLower(n.name), q) || strings.Contains(strings.ToLower(n.dataset), q) {
				for cur := n.key; cur != ""; {
					matches[cur] = true
					cur = by[cur].parent
				}
			}
		}
	}
	var out []treeNode
	visible := map[string]bool{}
	for _, n := range all {
		if q != "" && !matches[n.key] {
			continue
		}
		if n.parent != "" {
			if q == "" && !visible[n.parent] {
				continue
			}
			if q != "" && !matches[n.parent] {
				continue
			}
		}
		out = append(out, n)
		visible[n.key] = q != "" || n.kind == nodeSnapshot || d.expanded[n.key]
	}
	return out
}
func (d Datasets) visibleRows() []string {
	if d.legacy && d.level == levelDatasets {
		q := strings.ToLower(d.filter)
		var out []string
		for _, ds := range d.datasets {
			if q == "" || strings.Contains(strings.ToLower(ds.Name), q) {
				out = append(out, ds.Name)
			}
		}
		return out
	}
	ns := d.visibleNodes()
	out := make([]string, len(ns))
	for i := range ns {
		out[i] = ns[i].name
	}
	return out
}
func (d Datasets) selected() *treeNode {
	r := d.visibleNodes()
	if d.cursor < 0 || d.cursor >= len(r) {
		return nil
	}
	n := r[d.cursor]
	return &n
}
func (d *Datasets) selectKey(k string) {
	for i, n := range d.visibleNodes() {
		if n.key == k {
			d.cursor = i
			return
		}
	}
}
func (d *Datasets) retainMarks() {
	exists := map[string]bool{}
	for _, s := range d.snapshots {
		exists[s.Name] = true
	}
	for n := range d.marked {
		if !exists[n] && !d.failedMarks[n] {
			delete(d.marked, n)
		}
	}
}
func (d *Datasets) clamp() {
	n := len(d.visibleRows())
	if n == 0 {
		d.cursor, d.offset = 0, 0
	} else if d.cursor >= n {
		d.cursor = n - 1
	}
	*d = d.ensureCursorVisible()
}
func (d Datasets) summaryStatus() string {
	return fmt.Sprintf("%d snapshots · %s snapshot unique used", len(d.snapshots), formatBytes(sumSnapshots(d.snapshots)))
}
func (d Datasets) listPageSize(n int) int {
	h := d.Height - datasetsChromeLines
	if h < 1 {
		h = 1
	}
	if n > h && h > 1 {
		h--
	}
	return h
}
func (d Datasets) ensureCursorVisible() Datasets {
	n := len(d.visibleRows())
	p := d.listPageSize(n)
	if n == 0 {
		d.cursor, d.offset = 0, 0
		return d
	}
	if d.cursor < 0 {
		d.cursor = 0
	}
	if d.cursor >= n {
		d.cursor = n - 1
	}
	if d.cursor < d.offset {
		d.offset = d.cursor
	}
	if d.cursor >= d.offset+p {
		d.offset = d.cursor - p + 1
	}
	max := n - p
	if max < 0 {
		max = 0
	}
	if d.offset > max {
		d.offset = max
	}
	return d
}
func (d Datasets) windowBounds(n int) (int, int) {
	if n == 0 {
		return 0, 0
	}
	p := d.listPageSize(n)
	s := d.offset
	if s < 0 || s >= n {
		s = 0
	}
	e := s + p
	if e > n {
		e = n
	}
	return s, e
}
func statusCmd(text string, isErr bool, path string) tea.Cmd {
	return func() tea.Msg { return DatasetsStatusMsg{text, isErr, path} }
}

func (d Datasets) View() string {
	if d.legacy && d.level == levelDatasets {
		return d.legacyDatasetsView()
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee"))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24"))
	var b strings.Builder
	b.WriteString(title.Render("◆ Snapshot Dashboard"))
	b.WriteString(muted.Render("  · enter/right expand · space mark · d delete · o open · R reload"))
	b.WriteString("\n")
	state := cronState(d.cron)
	scheduled := 0
	for _, ds := range d.datasets {
		a, c := d.schedulesFor(ds.Name)
		if len(a)+len(c) > 0 {
			scheduled++
		}
	}
	summary := fmt.Sprintf("  %d snapshots  %s snapshot unique used  %d scheduled datasets  cron: %s", len(d.snapshots), formatBytes(sumSnapshots(d.snapshots)), scheduled, state)
	if state != "in sync" {
		b.WriteString(warn.Render(summary))
	} else {
		b.WriteString(muted.Render(summary))
	}
	b.WriteString("\n")
	if d.filter != "" || d.filtering {
		b.WriteString(muted.Render("  filter: "))
		b.WriteString(warn.Render(d.filter + map[bool]string{true: "▌"}[d.filtering]))
		b.WriteString("\n")
	}
	for _, e := range d.errors {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171")).Render("  " + e))
		b.WriteString("\n")
	}
	if d.cron.Err != nil {
		b.WriteString(warn.Render("  cron: " + d.cron.Err.Error()))
		b.WriteString("\n")
	}
	if d.loading {
		b.WriteString("\n" + muted.Render("  loading host inventory…"))
		return b.String()
	}
	rows := d.visibleNodes()
	if len(rows) == 0 {
		b.WriteString("\n" + muted.Render("  (no matching pools, datasets, or snapshots)"))
		return b.String()
	}
	b.WriteString("\n")
	s, e := d.windowBounds(len(rows))
	for i := s; i < e; i++ {
		b.WriteString(d.renderNode(rows[i], i == d.cursor))
		b.WriteString("\n")
	}
	if s > 0 || e < len(rows) {
		b.WriteString(scrollHint(s, e, len(rows)) + "\n")
	}
	if d.Width < 90 {
		if n := d.selected(); n != nil {
			b.WriteString(muted.Render("  "+d.metadata(*n)) + "\n")
		}
	}
	if d.busy {
		b.WriteString(muted.Render("  deleting snapshots…") + "\n")
	}
	if d.Confirm.Active {
		return b.String() + "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(d.Confirm.View())
	}
	return strings.TrimRight(b.String(), "\n")
}

func (d Datasets) legacyDatasetsView() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#22d3ee")).Render("◆ Datasets"))
	b.WriteString("\n\n")
	header := tableHeader([]colSpec{{"NAME", 36}, {"USED", 8}, {"AVAIL", 8}, {"TYPE", 10}, {"MOUNTPOINT", 20}})
	b.WriteString(header + "\n")
	rows := d.visibleRows()
	if len(rows) == 0 {
		return b.String() + mutedLine("  (no datasets)")
	}
	by := map[string]zfs.Dataset{}
	for _, v := range d.datasets {
		by[v.Name] = v
	}
	s, e := d.windowBounds(len(rows))
	for i := s; i < e; i++ {
		v := by[rows[i]]
		b.WriteString(tableRow([]string{v.Name, v.Used, v.Avail, v.Type, v.Mountpoint}, []int{36, 8, 8, 10, 20}, i == d.cursor) + "\n")
	}
	if s > 0 || e < len(rows) {
		b.WriteString(scrollHint(s, e, len(rows)))
	}
	return strings.TrimRight(b.String(), "\n")
}
func (d Datasets) renderNode(n treeNode, selected bool) string {
	indent := strings.Repeat("  ", n.depth)
	glyph := "•"
	if n.kind != nodeSnapshot {
		glyph = "▸"
		if d.expanded[n.key] || d.filter != "" {
			glyph = "▾"
		}
	}
	mark := " "
	if d.marked[n.key] {
		mark = "✓"
	}
	line := fmt.Sprintf("%s%s [%s] %s", indent, glyph, mark, n.name)
	if d.Width >= 90 {
		line += "  " + d.metadata(n)
	}
	if selected {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#0d1117")).Background(lipgloss.Color("#22d3ee")).Bold(true).Render("▸ " + line)
	}
	return "  " + line
}
func (d Datasets) metadata(n treeNode) string {
	switch n.kind {
	case nodePool:
		return fmt.Sprintf("%d snapshots · %s", n.count, formatBytes(n.bytes))
	case nodeDataset:
		badges := scheduleBadges(n.direct, n.inherited, d.cron)
		newest := "never"
		if !n.newest.IsZero() {
			newest = n.newest.Local().Format("2006-01-02 15:04")
		}
		return fmt.Sprintf("%d direct · %s · newest %s%s", n.count, formatBytes(n.bytes), newest, badges)
	default:
		return fmt.Sprintf("%s · %s ago · %s", n.creation.Local().Format("2006-01-02 15:04:05"), relativeAge(n.creation), formatBytes(n.bytes))
	}
}
func scheduleBadges(a, b []config.Schedule, st cronpkg.Status) string {
	var x []string
	for _, s := range a {
		x = append(x, "[schedule:"+s.Name+"]")
	}
	for _, s := range b {
		x = append(x, "[inherited:"+s.Name+"]")
	}
	if len(x) == 0 {
		return ""
	}
	if st.Err != nil || !st.Installed || !st.InSync {
		x = append(x, "[cron warning]")
	}
	return " · " + strings.Join(x, " ")
}
func cronState(s cronpkg.Status) string {
	if s.Err != nil {
		return "unavailable"
	}
	if !s.Installed {
		return "not installed"
	}
	if s.InSync {
		return "in sync"
	}
	return "out of sync"
}
func snapshotDataset(n string) string {
	if i := strings.Index(n, "@"); i >= 0 {
		return n[:i]
	}
	return n
}
func sumSnapshots(s []zfs.Snapshot) uint64 {
	var n uint64
	for _, v := range s {
		n += v.UsedBytes
	}
	return n
}
func formatBytes(n uint64) string {
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	for _, x := range []string{"KiB", "MiB", "GiB", "TiB", "PiB"} {
		v /= u
		if v < u {
			return fmt.Sprintf("%.1f %s", v, x)
		}
	}
	return fmt.Sprintf("%.1f EiB", v/u)
}
func relativeAge(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	v := time.Since(t)
	if v < 0 {
		v = 0
	}
	if v < time.Minute {
		return "<1m"
	}
	if v < time.Hour {
		return fmt.Sprintf("%dm", int(v.Minutes()))
	}
	if v < 24*time.Hour {
		return fmt.Sprintf("%dh", int(v.Hours()))
	}
	return fmt.Sprintf("%dd", int(v.Hours()/24))
}
func lenTrue(m map[string]bool) int {
	n := 0
	for _, v := range m {
		if v {
			n++
		}
	}
	return n
}
func scrollHint(s, e, n int) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Render(fmt.Sprintf("  showing %d–%d of %d", s+1, e, n))
}
