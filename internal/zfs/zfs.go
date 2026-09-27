package zfs

import (
	"context"
	"time"
)

type Pool struct {
	Name   string
	Health string
	Size   string
	Alloc  string
}

type Dataset struct {
	Name       string
	Used       string
	Avail      string
	Mountpoint string
	Type       string
}

type Snapshot struct {
	Name string
	Used string
	// UsedBytes is the raw value of the ZFS snapshot "used" property. It is
	// uniquely held snapshot space, not a guarantee of immediately reclaimable
	// space. Used is retained for compatibility with older callers.
	UsedBytes uint64
	Creation  time.Time
}

type Client interface {
	ListPools(ctx context.Context) ([]Pool, error)
	ListDatasets(ctx context.Context, poolOrDataset string) ([]Dataset, error)
	ListSnapshots(ctx context.Context, dataset string) ([]Snapshot, error)
	CreateSnapshot(ctx context.Context, fullName string, recursive bool) error
	DestroySnapshot(ctx context.Context, fullName string, recursive bool) error
	Rollback(ctx context.Context, fullName string, recursive bool) error
	Clone(ctx context.Context, snapshot, newDataset string) error
	Promote(ctx context.Context, dataset string) error
	Available(ctx context.Context) error
}
