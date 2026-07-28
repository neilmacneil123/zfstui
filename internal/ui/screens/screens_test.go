package screens

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
