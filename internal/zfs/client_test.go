package zfs_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/zfstui/zfstui/internal/zfs"
)

func TestListSnapshotsHostWideParsesRawBytesAndSkipsMalformed(t *testing.T) {
	c := zfs.NewCLI(false)
	var got []string
	c.Runner = func(ctx context.Context, name string, args ...string) (string, error) {
		got = append([]string{name}, args...)
		return "tank/a@good\t4096\t1700000000\ntank/a@bad\tnot-a-number\t1700000001\ntank/a@badtime\t1\tbad\n", nil
	}
	s, err := c.ListSnapshots(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 1 || s[0].UsedBytes != 4096 || s[0].Used != "4096" {
		t.Fatalf("snapshots: %#v", s)
	}
	want := []string{"zfs", "list", "-H", "-p", "-r", "-t", "snapshot", "-o", "name,used,creation", "-s", "creation"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%v want %v", got, want)
	}
}

func TestListSnapshotsDatasetKeepsScopeArgument(t *testing.T) {
	c := zfs.NewCLI(false)
	var got []string
	c.Runner = func(ctx context.Context, name string, args ...string) (string, error) { got = args; return "", nil }
	_, _ = c.ListSnapshots(context.Background(), "tank/a")
	if got[len(got)-1] != "tank/a" {
		t.Fatalf("args %v", got)
	}
}

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
