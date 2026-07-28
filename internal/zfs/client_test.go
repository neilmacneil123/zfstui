package zfs_test

import (
	"context"
	"strings"
	"testing"

	"github.com/zfstui/zfstui/internal/zfs"
)

func TestListPoolsParses(t *testing.T) {
	c := zfs.NewCLI(false)
	c.Runner = func(ctx context.Context, name string, args ...string) (string, error) {
		if name != "zpool" {
			t.Fatalf("cmd %s", name)
		}
		return "tank\tONLINE\t10G\t2G\n", nil
	}
	pools, err := c.ListPools(context.Background())
	if err != nil || len(pools) != 1 || pools[0].Name != "tank" {
		t.Fatalf("%+v %v", pools, err)
	}
}

func TestCreateSnapshotDryRun(t *testing.T) {
	c := zfs.NewCLI(true)
	called := false
	c.Runner = func(ctx context.Context, name string, args ...string) (string, error) {
		called = true
		return "", nil
	}
	if err := c.CreateSnapshot(context.Background(), "tank/home@snap1", false); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("dry-run must not invoke runner")
	}
}

func TestCreateSnapshotArgs(t *testing.T) {
	c := zfs.NewCLI(false)
	var got []string
	c.Runner = func(ctx context.Context, name string, args ...string) (string, error) {
		got = append([]string{name}, args...)
		return "", nil
	}
	_ = c.CreateSnapshot(context.Background(), "tank/home@snap1", true)
	if strings.Join(got, " ") != "zfs snapshot -r tank/home@snap1" {
		t.Fatalf("got %v", got)
	}
}

func TestFakeListAndCreate(t *testing.T) {
	f := zfs.NewFake()
	f.Pools = []zfs.Pool{{Name: "tank", Health: "ONLINE"}}
	f.Datasets = []zfs.Dataset{{Name: "tank/home", Type: "filesystem"}}
	if err := f.CreateSnapshot(context.Background(), "tank/home@a", false); err != nil {
		t.Fatal(err)
	}
	snaps, err := f.ListSnapshots(context.Background(), "tank/home")
	if err != nil || len(snaps) != 1 {
		t.Fatalf("%+v %v", snaps, err)
	}
}
