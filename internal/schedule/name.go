package schedule

import (
	"fmt"
	"strings"
	"time"
)

func SnapshotName(prefix string, t time.Time) string {
	return fmt.Sprintf("%s-%s", prefix, t.UTC().Format("20060102-150405"))
}

func FullSnapshot(dataset, snapName string) string {
	return dataset + "@" + snapName
}

func MatchesPrefix(fullSnapshot, dataset, prefix string) bool {
	want := dataset + "@" + prefix + "-"
	return strings.HasPrefix(fullSnapshot, want)
}

func DatasetOf(fullSnapshot string) string {
	i := strings.LastIndex(fullSnapshot, "@")
	if i < 0 {
		return fullSnapshot
	}
	return fullSnapshot[:i]
}
