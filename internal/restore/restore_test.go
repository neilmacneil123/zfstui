package restore_test

import (
	"context"
	"testing"
	"time"

	"github.com/zfstui/zfstui/internal/restore"
	"github.com/zfstui/zfstui/internal/zfs"
)

func TestClonePromote(t *testing.T) {
	f := zfs.NewFake()
	f.Snapshots = []zfs.Snapshot{{Name: "tank/home@s1", Creation: time.Now()}}
	svc := &restore.Service{Z: f}
	if err := svc.Clone(context.Background(), "tank/home@s1", "tank/home-restored", true); err != nil {
		t.Fatal(err)
	}
	if len(f.Log) != 2 || f.Log[0] != "clone tank/home@s1 tank/home-restored" || f.Log[1] != "promote tank/home-restored" {
		t.Fatalf("%v", f.Log)
	}
}

func TestNewerSnapshots(t *testing.T) {
	base := time.Now()
	all := []zfs.Snapshot{
		{Name: "tank/home@a", Creation: base},
		{Name: "tank/home@b", Creation: base.Add(time.Hour)},
		{Name: "tank/other@c", Creation: base.Add(2 * time.Hour)},
	}
	newer := restore.NewerSnapshots(all, "tank/home@a")
	if len(newer) != 1 || newer[0].Name != "tank/home@b" {
		t.Fatalf("%+v", newer)
	}
}
