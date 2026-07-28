package screens

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/zfs"
)

func TestConfirmYesNo(t *testing.T) {
	c := OpenYesNo("create", "Create", "body", "val", false)
	if !c.Active || !c.yesSelected {
		t.Fatalf("expected active yes-default confirm")
	}
	c, cmd := c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if c.Active {
		t.Fatal("expected closed after y")
	}
	msg := cmd()
	res, ok := msg.(ConfirmResultMsg)
	if !ok || !res.Confirmed || res.ID != "create" || res.Value != "val" {
		t.Fatalf("unexpected result: %#v", msg)
	}
}

func TestConfirmType(t *testing.T) {
	c := OpenType("destroy", "Destroy", "body", "snap1", "pool/ds@snap1")
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("snap")})
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	c, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if c.Active {
		t.Fatal("expected closed")
	}
	res := cmd().(ConfirmResultMsg)
	if !res.Confirmed || res.Value != "pool/ds@snap1" {
		t.Fatalf("got %#v", res)
	}
}

func TestConfirmTypeMismatch(t *testing.T) {
	c := OpenType("destroy", "Destroy", "body", "snap1", "full")
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("nope")})
	c, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	res := cmd().(ConfirmResultMsg)
	if res.Confirmed {
		t.Fatal("mismatch must not confirm")
	}
}

func TestLoadPoolsAndDatasets(t *testing.T) {
	f := zfs.NewFake()
	f.Pools = []zfs.Pool{{Name: "tank", Health: "ONLINE", Size: "1T", Alloc: "100G"}}
	f.Datasets = []zfs.Dataset{
		{Name: "tank", Used: "1G", Avail: "9G", Type: "filesystem", Mountpoint: "/tank"},
		{Name: "tank/home", Used: "2G", Avail: "8G", Type: "filesystem", Mountpoint: "/home"},
	}

	msg := LoadPoolsCmd(f)().(PoolsLoadedMsg)
	if msg.Err != nil || len(msg.Pools) != 1 {
		t.Fatalf("pools: %#v", msg)
	}

	d := NewDatasets()
	d, _ = d.Update(msg)
	if d.level != levelPools || len(d.pools) != 1 {
		t.Fatalf("datasets after pools: level=%v pools=%d", d.level, len(d.pools))
	}

	dmsg := LoadDatasetsCmd(f, "tank")().(DatasetsLoadedMsg)
	d, _ = d.Update(dmsg)
	if d.level != levelDatasets || len(d.datasets) != 2 {
		t.Fatalf("datasets: level=%v n=%d", d.level, len(d.datasets))
	}

	// Filter
	d.filtering = true
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("home")})
	rows := d.visibleRows()
	if len(rows) != 1 || rows[0] != "tank/home" {
		t.Fatalf("filter rows: %v", rows)
	}
}

func TestSnapshotsCreateDestroy(t *testing.T) {
	f := zfs.NewFake()
	f.Snapshots = []zfs.Snapshot{
		{Name: "tank/home@old", Used: "1M", Creation: time.Unix(1, 0).UTC()},
	}
	s := NewSnapshots("tank/home", "zfstui", false)
	msg := LoadSnapshotsCmd(f, "tank/home")().(SnapshotsLoadedMsg)
	s, _ = s.Update(msg)
	if len(s.snaps) != 1 {
		t.Fatalf("snaps: %d", len(s.snaps))
	}

	// Create request path via confirm
	s, _ = s.beginCreate()
	if !s.Confirm.Active {
		t.Fatal("expected create confirm")
	}
	s, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	req := cmd().(ConfirmResultMsg)
	s, cmd = s.Update(req)
	creq := cmd().(CreateSnapshotRequestMsg)
	if creq.FullName == "" || creq.Recursive {
		t.Fatalf("create req: %#v", creq)
	}
	done := CreateSnapshotCmd(f, creq.FullName, creq.Recursive)().(SnapshotOpDoneMsg)
	if done.Err != nil {
		t.Fatal(done.Err)
	}
	if len(f.Snapshots) != 2 {
		t.Fatalf("expected 2 snaps, got %d", len(f.Snapshots))
	}

	// Destroy typed confirm
	s.snaps = f.Snapshots
	s.cursor = 0
	s, _ = s.beginDestroy()
	short := s.Confirm.Require
	for _, r := range short {
		s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	s, cmd = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	dres := cmd().(ConfirmResultMsg)
	s, cmd = s.Update(dres)
	dreq := cmd().(DestroySnapshotRequestMsg)
	done = DestroySnapshotCmd(f, dreq.FullName, false)().(SnapshotOpDoneMsg)
	if done.Err != nil {
		t.Fatal(done.Err)
	}
}

func TestScheduleFormValidate(t *testing.T) {
	defaults := config.Defaults{SnapshotPrefix: "zfstui", Recursive: false}
	existing := []config.Schedule{{Name: "hourly", Dataset: "tank/a", Cron: "0 * * * *", Prefix: "zfstui", Enabled: true}}

	f := NewScheduleFormAdd(defaults, existing)
	f.fields[fieldName] = "hourly"
	f.fields[fieldDataset] = "tank/b"
	f.fields[fieldCron] = "0 * * * *"
	if _, err := f.Validate(); err == nil {
		t.Fatal("expected duplicate name error")
	}

	f.fields[fieldName] = "daily"
	f.fields[fieldCron] = "not-a-cron"
	if _, err := f.Validate(); err == nil {
		t.Fatal("expected bad cron error")
	}

	f.fields[fieldCron] = "30 2 * * *"
	f.fields[fieldKeepLast] = "abc"
	if _, err := f.Validate(); err == nil {
		t.Fatal("expected keep_last parse error")
	}

	f.fields[fieldKeepLast] = "7"
	f.fields[fieldMaxAge] = "nope"
	if _, err := f.Validate(); err == nil {
		t.Fatal("expected max_age error")
	}

	f.fields[fieldMaxAge] = "7d"
	f.fields[fieldRecursive] = "true"
	s, err := f.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "daily" || s.Dataset != "tank/b" || !s.Recursive || s.Retention.KeepLast == nil || *s.Retention.KeepLast != 7 || s.Retention.MaxAge != "7d" {
		t.Fatalf("unexpected schedule: %#v", s)
	}
}

func TestRetentionSummary(t *testing.T) {
	keep := 24
	s := config.Schedule{Retention: config.Retention{KeepLast: &keep, MaxAge: "7d"}}
	if got := RetentionSummary(s); got != "keep 24, max 7d" {
		t.Fatalf("got %q", got)
	}
	if got := RetentionSummary(config.Schedule{}); got != "—" {
		t.Fatalf("empty: %q", got)
	}
}

func TestSchedulesToggleAndSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	keep := 3
	cfg.Schedules = []config.Schedule{{
		Name: "s1", Dataset: "tank/home", Cron: "0 * * * *", Prefix: "zfstui",
		Enabled: true, Retention: config.Retention{KeepLast: &keep},
	}}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	sc := NewSchedules(&cfg, path)
	sc, cmd := sc.toggleEnabled()
	if cmd == nil {
		t.Fatal("expected status cmd")
	}
	msg := cmd().(SchedulesStatusMsg)
	if msg.IsErr || !strings.Contains(msg.Text, "disabled") {
		t.Fatalf("status: %#v", msg)
	}
	if cfg.Schedules[0].Enabled {
		t.Fatal("expected disabled")
	}
	// reload from disk
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Schedules[0].Enabled {
		t.Fatal("disk still enabled")
	}
}

func TestSchedulesAddViaForm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	sc := NewSchedules(&cfg, path)
	sc, _ = sc.beginAdd()
	if !sc.Form.Active {
		t.Fatal("form should be active")
	}
	// fill required fields
	sc.Form.fields[fieldName] = "nightly"
	sc.Form.fields[fieldDataset] = "tank/data"
	sc.Form.fields[fieldCron] = "0 3 * * *"
	sc.Form.fields[fieldPrefix] = "zfstui"
	form, cmd := sc.Form.trySave()
	sc.Form = form
	if sc.Form.Active {
		t.Fatalf("form should close: err=%s", sc.Form.err)
	}
	res := cmd().(ScheduleFormResultMsg)
	sc, cmd = sc.Update(res)
	if cmd == nil {
		t.Fatal("expected status")
	}
	st := cmd().(SchedulesStatusMsg)
	if st.IsErr {
		t.Fatal(st.Text)
	}
	if len(cfg.Schedules) != 1 || cfg.Schedules[0].Name != "nightly" {
		t.Fatalf("schedules: %#v", cfg.Schedules)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestCrontabDryRunInstall(t *testing.T) {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{{
		Name: "s1", Dataset: "tank/home", Cron: "0 * * * *", Prefix: "zfstui", Enabled: true,
	}}
	c := NewCrontab(&cfg, "/tmp/cfg.yaml", "/usr/bin/zfstui", true)
	if c.preview == "" && c.previewErr != "" {
		t.Fatalf("preview err: %s", c.previewErr)
	}
	c, cmd := c.beginInstall()
	msg := cmd().(CrontabStatusMsg)
	if msg.IsErr || !strings.Contains(msg.Text, "dry-run") {
		t.Fatalf("expected dry-run status, got %#v", msg)
	}
	if c.busy {
		t.Fatal("dry-run must not set busy")
	}
}
