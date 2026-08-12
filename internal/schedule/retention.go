package schedule

import (
	"fmt"
	"sort"
	"time"
)

type SnapInfo struct {
	Name    string // full dataset@snap
	Created time.Time
}

type GFSRetention struct {
	Daily   int
	Weekly  int
	Monthly int
	Yearly  int
}

func (r GFSRetention) Enabled() bool {
	return r.Daily > 0 || r.Weekly > 0 || r.Monthly > 0 || r.Yearly > 0
}

// SelectForDeletion returns snapshots that should be destroyed using classic GFS retention.
// snaps must be sorted newest-first by Created. If not sorted, sort first.
func SelectForDeletion(snaps []SnapInfo, retention GFSRetention, now time.Time) []SnapInfo {
	if len(snaps) == 0 || !retention.Enabled() {
		return nil
	}
	items := append([]SnapInfo(nil), snaps...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Created.After(items[j].Created)
	})

	keep := map[string]struct{}{}
	keepTierBuckets(items, keep, retention.Daily, now, bucketDay)
	keepTierBuckets(items, keep, retention.Weekly, now, bucketWeek)
	keepTierBuckets(items, keep, retention.Monthly, now, bucketMonth)
	keepTierBuckets(items, keep, retention.Yearly, now, bucketYear)

	var out []SnapInfo
	for _, s := range items {
		if _, ok := keep[s.Name]; ok {
			continue
		}
		out = append(out, s)
	}
	return out
}

func keepTierBuckets(items []SnapInfo, keep map[string]struct{}, count int, now time.Time, bucketFn func(time.Time) string) {
	if count <= 0 {
		return
	}
	seen := map[string]struct{}{}
	for _, s := range items {
		if s.Created.After(now) {
			// future timestamps should still be eligible, but bucket ordering
			// is driven by the list's sort order, so no special handling here.
		}
		bucket := bucketFn(s.Created.UTC())
		if _, ok := seen[bucket]; ok {
			continue
		}
		seen[bucket] = struct{}{}
		keep[s.Name] = struct{}{}
		if len(seen) >= count {
			return
		}
	}
}

func bucketDay(t time.Time) string {
	y, m, d := t.Date()
	return fmt.Sprintf("%04d-%02d-%02d", y, int(m), d)
}

func bucketWeek(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

func bucketMonth(t time.Time) string {
	y, m, _ := t.Date()
	return fmt.Sprintf("%04d-%02d", y, int(m))
}

func bucketYear(t time.Time) string {
	return fmt.Sprintf("%04d", t.Year())
}
