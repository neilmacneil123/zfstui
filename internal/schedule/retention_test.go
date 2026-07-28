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

func TestKeepLastOnly(t *testing.T) {
	n := 2
	del := schedule.SelectForDeletion(snaps("a", "b", "c", "d"), &n, nil, time.Time{})
	if len(del) != 2 || del[0].Name != "c" || del[1].Name != "d" {
		t.Fatalf("got %+v", del)
	}
}

func TestMaxAgeOnly(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	age := 90 * time.Minute
	// ages: 0h, 1h, 2h, 3h from newest
	del := schedule.SelectForDeletion(snaps("a", "b", "c", "d"), nil, &age, now)
	// created: a=12:00, b=11:00, c=10:00, d=09:00 → c and d older than 90m
	if len(del) != 2 {
		t.Fatalf("got %+v", del)
	}
}

func TestBothRules(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	n := 2
	age := 90 * time.Minute
	// newest 2 protected even if old; among rest delete if older than 90m
	del := schedule.SelectForDeletion(snaps("a", "b", "c", "d"), &n, &age, now)
	// c=2h, d=3h both outside top 2 and older than 90m
	if len(del) != 2 {
		t.Fatalf("got %+v", del)
	}
	// protect top 2 when they would fail max_age alone
	oldTop := []schedule.SnapInfo{
		{Name: "a", Created: now.Add(-3 * time.Hour)},
		{Name: "b", Created: now.Add(-4 * time.Hour)},
		{Name: "c", Created: now.Add(-5 * time.Hour)},
	}
	del2 := schedule.SelectForDeletion(oldTop, &n, &age, now)
	// a,b protected; c deleted
	if len(del2) != 1 || del2[0].Name != "c" {
		t.Fatalf("got %+v", del2)
	}
}

func TestParseMaxAge(t *testing.T) {
	d, err := schedule.ParseMaxAge("7d")
	if err != nil || d != 7*24*time.Hour {
		t.Fatalf("got %v %v", d, err)
	}
	d, err = schedule.ParseMaxAge("12h")
	if err != nil || d != 12*time.Hour {
		t.Fatalf("got %v %v", d, err)
	}
}
