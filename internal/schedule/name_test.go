package schedule_test

import (
	"testing"
	"time"

	"github.com/zfstui/zfstui/internal/schedule"
)

func TestSnapshotName(t *testing.T) {
	ts := time.Date(2026, 7, 28, 15, 4, 5, 0, time.UTC)
	got := schedule.SnapshotName("zfstui", ts)
	if got != "zfstui-20260728-150405" {
		t.Fatalf("got %q", got)
	}
}

func TestMatchesPrefix(t *testing.T) {
	if !schedule.MatchesPrefix("tank/home@zfstui-20260728-150405", "tank/home", "zfstui") {
		t.Fatal("expected match")
	}
	if schedule.MatchesPrefix("tank/home@daily-1", "tank/home", "zfstui") {
		t.Fatal("should not match other prefix")
	}
	if schedule.MatchesPrefix("tank/other@zfstui-1", "tank/home", "zfstui") {
		t.Fatal("wrong dataset")
	}
}
