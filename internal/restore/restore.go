package restore

import (
	"context"
	"fmt"
	"strings"

	"github.com/zfstui/zfstui/internal/zfs"
)

type Service struct {
	Z zfs.Client
}

func (s *Service) Rollback(ctx context.Context, snapshot string, recursive bool) error {
	if !strings.Contains(snapshot, "@") {
		return fmt.Errorf("invalid snapshot %q", snapshot)
	}
	return s.Z.Rollback(ctx, snapshot, recursive)
}

func (s *Service) Clone(ctx context.Context, snapshot, newDataset string, promote bool) error {
	if err := s.Z.Clone(ctx, snapshot, newDataset); err != nil {
		return err
	}
	if promote {
		return s.Z.Promote(ctx, newDataset)
	}
	return nil
}

func NewerSnapshots(all []zfs.Snapshot, target string) []zfs.Snapshot {
	var targetSnap *zfs.Snapshot
	ds := datasetOf(target)
	for i := range all {
		if all[i].Name == target {
			targetSnap = &all[i]
			break
		}
	}
	if targetSnap == nil {
		return nil
	}
	var out []zfs.Snapshot
	for _, s := range all {
		if datasetOf(s.Name) != ds {
			continue
		}
		if s.Creation.After(targetSnap.Creation) {
			out = append(out, s)
		}
	}
	return out
}

func datasetOf(full string) string {
	i := strings.LastIndex(full, "@")
	if i < 0 {
		return full
	}
	return full[:i]
}
