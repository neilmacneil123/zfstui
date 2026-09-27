package zfs

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Fake struct {
	Pools     []Pool
	Datasets  []Dataset
	Snapshots []Snapshot
	Err       error
	Log       []string
}

func NewFake() *Fake { return &Fake{} }

func (f *Fake) Available(ctx context.Context) error { return f.Err }

func (f *Fake) ListPools(ctx context.Context) ([]Pool, error) {
	return append([]Pool(nil), f.Pools...), f.Err
}

func (f *Fake) ListDatasets(ctx context.Context, poolOrDataset string) ([]Dataset, error) {
	var out []Dataset
	for _, d := range f.Datasets {
		if poolOrDataset == "" || d.Name == poolOrDataset || strings.HasPrefix(d.Name, poolOrDataset+"/") {
			out = append(out, d)
		}
	}
	return out, f.Err
}

func (f *Fake) ListSnapshots(ctx context.Context, dataset string) ([]Snapshot, error) {
	var out []Snapshot
	for _, s := range f.Snapshots {
		if dataset == "" || strings.HasPrefix(s.Name, dataset+"@") {
			out = append(out, s)
		}
	}
	return out, f.Err
}

func (f *Fake) CreateSnapshot(ctx context.Context, fullName string, recursive bool) error {
	f.Log = append(f.Log, "create "+fullName)
	if f.Err != nil {
		return f.Err
	}
	f.Snapshots = append(f.Snapshots, Snapshot{Name: fullName, Creation: time.Now().UTC()})
	return nil
}

func (f *Fake) DestroySnapshot(ctx context.Context, fullName string, recursive bool) error {
	f.Log = append(f.Log, "destroy "+fullName)
	if f.Err != nil {
		return f.Err
	}
	var rest []Snapshot
	for _, s := range f.Snapshots {
		if s.Name != fullName {
			rest = append(rest, s)
		}
	}
	f.Snapshots = rest
	return nil
}

func (f *Fake) Rollback(ctx context.Context, fullName string, recursive bool) error {
	if recursive {
		f.Log = append(f.Log, "rollback -r "+fullName)
	} else {
		f.Log = append(f.Log, "rollback "+fullName)
	}
	return f.Err
}

func (f *Fake) Clone(ctx context.Context, snapshot, newDataset string) error {
	f.Log = append(f.Log, fmt.Sprintf("clone %s %s", snapshot, newDataset))
	return f.Err
}

func (f *Fake) Promote(ctx context.Context, dataset string) error {
	f.Log = append(f.Log, "promote "+dataset)
	return f.Err
}
