package screens

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zfstui/zfstui/internal/config"
	cronpkg "github.com/zfstui/zfstui/internal/cron"
	"github.com/zfstui/zfstui/internal/zfs"
)

func dashboardFixture() Datasets {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{{Name: "root", Dataset: "tank", Enabled: true, Recursive: true}, {Name: "direct", Dataset: "tank/home", Enabled: true}, {Name: "off", Dataset: "tank/home", Enabled: false}}
	d := NewDatasets(&cfg)
	d.Width, d.Height = 120, 30
	d, _ = d.Update(DashboardLoadedMsg{Pools: []zfs.Pool{{Name: "tank"}}, Datasets: []zfs.Dataset{{Name: "tank"}, {Name: "tank/home"}}, Snapshots: []zfs.Snapshot{{Name: "tank/home@a", UsedBytes: 1024, Creation: time.Now().Add(-time.Hour)}, {Name: "tank/home@b", UsedBytes: 2048, Creation: time.Now()}}, Cron: cronpkg.Status{Mode: "system", Installed: true, InSync: true}})
	return d
}

func TestDashboardTreeExpansionFilteringAndAggregation(t *testing.T) {
	d := dashboardFixture()
	if got := len(d.visibleNodes()); got != 1 {
		t.Fatalf("collapsed rows=%d", got)
	}
	d.expanded["pool:tank"] = true
	d.expanded["ds:tank"] = true
	d.expanded["ds:tank/home"] = true
	rows := d.visibleNodes()
	if len(rows) != 5 {
		t.Fatalf("rows=%d: %#v", len(rows), rows)
	}
	var home *treeNode
	for i := range rows {
		if rows[i].key == "ds:tank/home" {
			home = &rows[i]
		}
	}
	if home == nil || home.count != 2 || home.bytes != 3072 || len(home.direct) != 1 || len(home.inherited) != 1 {
		t.Fatalf("home=%#v", home)
	}
	d.filter = "b"
	rows = d.visibleNodes()
	if len(rows) != 4 || rows[len(rows)-1].key != "tank/home@b" {
		t.Fatalf("filtered=%#v", rows)
	}
}

func TestDashboardMarksSurviveCollapseAndExactDeletePhrase(t *testing.T) {
	d := dashboardFixture()
	d.expanded["pool:tank"], d.expanded["ds:tank"], d.expanded["ds:tank/home"] = true, true, true
	d.selectKey("tank/home@a")
	d, _ = d.Update(tea.KeyMsg{Type: tea.KeySpace})
	d.expanded["ds:tank/home"] = false
	if len(d.targets()) != 1 {
		t.Fatalf("targets=%v", d.targets())
	}
	d, _ = d.beginDelete()
	if d.Confirm.Require != "DELETE 1" || !strings.Contains(d.Confirm.Body, "1.0 KiB") {
		t.Fatalf("confirm=%#v", d.Confirm)
	}
}

type deleteClient struct {
	zfs.Fake
	fail string
}

func (c *deleteClient) DestroySnapshot(ctx context.Context, n string, r bool) error {
	c.Log = append(c.Log, "destroy "+n)
	if n == c.fail {
		return errors.New("boom")
	}
	return nil
}
func TestBulkDestroyStableOrderContinuesAfterFailure(t *testing.T) {
	c := &deleteClient{fail: "b@y"}
	snaps := []zfs.Snapshot{{Name: "b@y", Creation: time.Unix(1, 0)}, {Name: "a@z", Creation: time.Unix(3, 0)}, {Name: "a@x", Creation: time.Unix(2, 0)}}
	r := BulkDestroyCmd(c, snaps, false)().(BulkDestroyResultMsg)
	want := []string{"destroy a@x", "destroy a@z", "destroy b@y"}
	if strings.Join(c.Log, "|") != strings.Join(want, "|") || len(r.Succeeded) != 2 || len(r.Failed) != 1 {
		t.Fatalf("log=%v result=%#v", c.Log, r)
	}
}
