package zfs

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type CLI struct {
	DryRun     bool
	ZfsPath    string
	ZpoolPath  string
	Runner     func(ctx context.Context, name string, args ...string) (string, error)
	LastDryRun []string // recorded commands in dry-run
}

func NewCLI(dryRun bool) *CLI {
	c := &CLI{DryRun: dryRun, ZfsPath: "zfs", ZpoolPath: "zpool"}
	c.Runner = c.defaultRunner
	return c
}

func (c *CLI) defaultRunner(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if err != nil {
		return buf.String(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, buf.String())
	}
	return buf.String(), nil
}

func (c *CLI) run(ctx context.Context, name string, args ...string) (string, error) {
	if c.DryRun {
		c.LastDryRun = append(c.LastDryRun, name+" "+strings.Join(args, " "))
		return "", nil
	}
	return c.Runner(ctx, name, args...)
}

func (c *CLI) Available(ctx context.Context) error {
	_, err := c.Runner(ctx, c.ZfsPath, "version")
	if err != nil {
		// try -V or just which via empty list
		_, err2 := c.Runner(ctx, c.ZfsPath, "list", "-H")
		if err2 != nil {
			return fmt.Errorf("zfs not available: %v", err)
		}
	}
	return nil
}

func (c *CLI) ListPools(ctx context.Context) ([]Pool, error) {
	out, err := c.Runner(ctx, c.ZpoolPath, "list", "-H", "-o", "name,health,size,allocated")
	if err != nil {
		return nil, err
	}
	var pools []Pool
	for _, line := range splitLines(out) {
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		pools = append(pools, Pool{Name: f[0], Health: f[1], Size: f[2], Alloc: f[3]})
	}
	return pools, nil
}

func (c *CLI) ListDatasets(ctx context.Context, poolOrDataset string) ([]Dataset, error) {
	args := []string{"list", "-H", "-r", "-t", "filesystem,volume", "-o", "name,used,avail,mountpoint,type"}
	if poolOrDataset != "" {
		args = append(args, poolOrDataset)
	}
	out, err := c.Runner(ctx, c.ZfsPath, args...)
	if err != nil {
		return nil, err
	}
	var ds []Dataset
	for _, line := range splitLines(out) {
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			continue
		}
		ds = append(ds, Dataset{Name: f[0], Used: f[1], Avail: f[2], Mountpoint: f[3], Type: f[4]})
	}
	return ds, nil
}

func (c *CLI) ListSnapshots(ctx context.Context, dataset string) ([]Snapshot, error) {
	// Prefer -p so creation is a parseable unix timestamp.
	args := []string{"list", "-H", "-p", "-r", "-t", "snapshot", "-o", "name,used,creation", "-s", "creation"}
	if dataset != "" {
		args = append(args, dataset)
	}
	out, err := c.Runner(ctx, c.ZfsPath, args...)
	if err != nil {
		return nil, err
	}
	var snaps []Snapshot
	for _, line := range splitLines(out) {
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		used, parseErr := strconv.ParseUint(strings.TrimSpace(f[1]), 10, 64)
		if parseErr != nil {
			continue
		}
		cre := parseCreation(f[2])
		if cre.IsZero() {
			continue
		}
		snaps = append(snaps, Snapshot{Name: f[0], Used: f[1], UsedBytes: used, Creation: cre})
	}
	return snaps, nil
}

func (c *CLI) CreateSnapshot(ctx context.Context, fullName string, recursive bool) error {
	args := []string{"snapshot"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, fullName)
	_, err := c.run(ctx, c.ZfsPath, args...)
	return err
}

func (c *CLI) DestroySnapshot(ctx context.Context, fullName string, recursive bool) error {
	args := []string{"destroy"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, fullName)
	_, err := c.run(ctx, c.ZfsPath, args...)
	return err
}

func (c *CLI) Rollback(ctx context.Context, fullName string, recursive bool) error {
	args := []string{"rollback"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, fullName)
	_, err := c.run(ctx, c.ZfsPath, args...)
	return err
}

func (c *CLI) Clone(ctx context.Context, snapshot, newDataset string) error {
	_, err := c.run(ctx, c.ZfsPath, "clone", snapshot, newDataset)
	return err
}

func (c *CLI) Promote(ctx context.Context, dataset string) error {
	_, err := c.run(ctx, c.ZfsPath, "promote", dataset)
	return err
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func parseCreation(s string) time.Time {
	// Prefer numeric unix from zfs list -p
	if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
		return time.Unix(n, 0).UTC()
	}
	// Fallback layouts
	for _, layout := range []string{time.RFC3339, "Mon Jan 2 15:04 2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
