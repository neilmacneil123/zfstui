package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/zfstui/zfstui/internal/app"
	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/zfs"
)

func TestRunScheduleCreatesAndPrunes(t *testing.T) {
	f := zfs.NewFake()
	f.Datasets = []zfs.Dataset{{Name: "tank/home"}}
	now := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	// seed old snaps
	for i := 0; i < 5; i++ {
		name := "tank/home@zfstui-" + now.Add(-time.Duration(i+1)*time.Hour).Format("20060102-150405")
		f.Snapshots = append(f.Snapshots, zfs.Snapshot{Name: name, Creation: now.Add(-time.Duration(i+1) * time.Hour)})
	}
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{{
		Name: "hourly", Dataset: "tank/home", Cron: "0 * * * *", Prefix: "zfstui",
		Enabled: true, Retention: config.Retention{Daily: 2},
	}}
	r := &app.Runner{Z: f, Now: func() time.Time { return now }}
	created, pruned, err := r.RunSchedule(context.Background(), &cfg, "hourly")
	if err != nil {
		t.Fatal(err)
	}
	if created == "" || !containsPrefix(created, "tank/home@zfstui-") {
		t.Fatalf("created %q", created)
	}
	// after create there are 6; keep 2 → prune 4
	if len(pruned) < 1 {
		t.Fatalf("expected prunes, got %v (snaps=%d)", pruned, len(f.Snapshots))
	}
}

func containsPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
