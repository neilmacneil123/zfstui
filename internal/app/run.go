package app

import (
	"context"
	"fmt"
	"time"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/schedule"
	"github.com/zfstui/zfstui/internal/zfs"
)

type Runner struct {
	Z   zfs.Client
	Now func() time.Time
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

func (r *Runner) Snapshot(ctx context.Context, dataset, prefix string, recursive bool) (string, error) {
	snap := schedule.SnapshotName(prefix, r.now())
	full := schedule.FullSnapshot(dataset, snap)
	if err := r.Z.CreateSnapshot(ctx, full, recursive); err != nil {
		return "", err
	}
	return full, nil
}

func (r *Runner) RunSchedule(ctx context.Context, cfg *config.Config, name string) (string, []string, error) {
	s, err := cfg.FindSchedule(name)
	if err != nil {
		return "", nil, err
	}
	if !s.Enabled {
		return "", nil, fmt.Errorf("schedule %q is disabled", name)
	}
	created, err := r.Snapshot(ctx, s.Dataset, s.Prefix, s.Recursive)
	if err != nil {
		return "", nil, err
	}
	pruned, err := r.prune(ctx, s)
	return created, pruned, err
}

func (r *Runner) PruneSchedule(ctx context.Context, cfg *config.Config, name string) ([]string, error) {
	s, err := cfg.FindSchedule(name)
	if err != nil {
		return nil, err
	}
	return r.prune(ctx, s)
}

func (r *Runner) prune(ctx context.Context, s *config.Schedule) ([]string, error) {
	ret := schedule.GFSRetention{
		Daily:   s.Retention.Daily,
		Weekly:  s.Retention.Weekly,
		Monthly: s.Retention.Monthly,
		Yearly:  s.Retention.Yearly,
	}
	if !ret.Enabled() {
		return nil, nil
	}
	list, err := r.Z.ListSnapshots(ctx, s.Dataset)
	if err != nil {
		return nil, err
	}
	var matched []schedule.SnapInfo
	for _, sn := range list {
		if schedule.MatchesPrefix(sn.Name, s.Dataset, s.Prefix) {
			matched = append(matched, schedule.SnapInfo{Name: sn.Name, Created: sn.Creation})
		}
	}
	toDel := schedule.SelectForDeletion(matched, ret, r.now())
	var pruned []string
	for _, d := range toDel {
		if err := r.Z.DestroySnapshot(ctx, d.Name, false); err != nil {
			return pruned, fmt.Errorf("destroy %s: %w", d.Name, err)
		}
		pruned = append(pruned, d.Name)
	}
	return pruned, nil
}
