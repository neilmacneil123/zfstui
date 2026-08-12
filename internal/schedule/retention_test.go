package schedule_test

import (
	"testing"
	"time"

	"github.com/zfstui/zfstui/internal/schedule"
)

func snaps(names ...string) []schedule.SnapInfo {
	base := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	out := make([]schedule.SnapInfo, len(names))
	for i, n := range names {
		// index 0 = newest
		out[i] = schedule.SnapInfo{Name: n, Created: base.Add(-time.Duration(i) * time.Hour)}
	}
	return out
}

func TestGFSRetentionDailyWeeklyMonthlyYearly(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	snaps := []schedule.SnapInfo{
		{Name: "newest", Created: now},
		{Name: "same-day-late", Created: now.Add(-2 * time.Hour)},
		{Name: "same-week-late", Created: now.Add(-2 * 24 * time.Hour)},
		{Name: "same-month-late", Created: now.Add(-20 * 24 * time.Hour)},
		{Name: "same-year-late", Created: now.Add(-120 * 24 * time.Hour)},
		{Name: "older-year", Created: now.Add(-500 * 24 * time.Hour)},
	}
	ret := schedule.GFSRetention{Daily: 1, Weekly: 1, Monthly: 1, Yearly: 1}
	del := schedule.SelectForDeletion(snaps, ret, now)
	if len(del) != 5 {
		t.Fatalf("got %+v", del)
	}
	if del[0].Name != "same-day-late" || del[4].Name != "older-year" {
		t.Fatalf("unexpected delete set: %+v", del)
	}
}

func TestGFSRetentionMultipleBuckets(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	snaps := []schedule.SnapInfo{
		{Name: "d1", Created: now},
		{Name: "d2", Created: now.Add(-24 * time.Hour)},
		{Name: "d3", Created: now.Add(-48 * time.Hour)},
		{Name: "d4", Created: now.Add(-72 * time.Hour)},
		{Name: "d5", Created: now.Add(-96 * time.Hour)},
		{Name: "d6", Created: now.Add(-120 * time.Hour)},
	}
	ret := schedule.GFSRetention{Daily: 2}
	del := schedule.SelectForDeletion(snaps, ret, now)
	if len(del) != 4 {
		t.Fatalf("got %+v", del)
	}
	if del[0].Name != "d3" || del[3].Name != "d6" {
		t.Fatalf("unexpected delete order: %+v", del)
	}
}

func TestGFSRetentionDisabled(t *testing.T) {
	now := time.Now()
	del := schedule.SelectForDeletion(snaps("a", "b"), schedule.GFSRetention{}, now)
	if len(del) != 0 {
		t.Fatalf("expected no deletes, got %+v", del)
	}
}
