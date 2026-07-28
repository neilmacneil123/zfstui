package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type SnapInfo struct {
	Name    string // full dataset@snap
	Created time.Time
}

// SelectForDeletion returns snapshots that should be destroyed.
// snaps must be sorted newest-first by Created. If not sorted, sort first.
func SelectForDeletion(snaps []SnapInfo, keepLast *int, maxAge *time.Duration, now time.Time) []SnapInfo {
	if len(snaps) == 0 {
		return nil
	}
	// copy and ensure newest first
	items := append([]SnapInfo(nil), snaps...)
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].Created.After(items[i].Created) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}

	var out []SnapInfo
	for i, s := range items {
		protected := keepLast != nil && i < *keepLast
		if protected {
			continue
		}
		if keepLast != nil && maxAge == nil {
			// only keep_last: everything outside N is deleted
			out = append(out, s)
			continue
		}
		if maxAge != nil {
			if now.Sub(s.Created) > *maxAge {
				out = append(out, s)
			}
			// if only max_age and still young, keep
			// if both and outside N but young, keep
			continue
		}
		// neither rule: never reached if callers check; treat as keep all
	}
	return out
}

func ParseMaxAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty max_age")
	}
	unit := s[len(s)-1]
	numPart := s[:len(s)-1]
	n, err := strconv.Atoi(numPart)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid max_age %q", s)
	}
	switch unit {
	case 'h', 'H':
		return time.Duration(n) * time.Hour, nil
	case 'd', 'D':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'w', 'W':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	case 'm', 'M':
		// minutes — document as m for minutes (not months)
		return time.Duration(n) * time.Minute, nil
	default:
		return 0, fmt.Errorf("invalid max_age unit in %q (use m/h/d/w)", s)
	}
}
