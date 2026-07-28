# zfstui Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a pretty Go + Bubble Tea Linux utility that manages ZFS snapshots, config-driven schedules installed as managed crontab, retention prune, and restore via rollback or clone.

**Architecture:** Layered Go packages (`config`, `schedule`, `zfs`, `restore`, `cron`) with a Bubble Tea TUI client and cobra-style (or stdlib `flag` + subcommands) CLI for cron automation. Config YAML is source of truth; crontab is derived. ZFS operations go through an interface so unit tests use fakes.

**Tech Stack:** Go 1.22+, `gopkg.in/yaml.v3`, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles` (list, textinput, viewport, help), OpenZFS CLI (`zfs`, `zpool`), system crontab.

**Spec:** `docs/superpowers/specs/2026-07-28-zfstui-design.md`

## Global Constraints

- Binary name: `zfstui`; CGO disabled preferred (`CGO_ENABLED=0`)
- Module path: `github.com/zfstui/zfstui` (use this unless repo remote dictates otherwise)
- Config paths: `/etc/zfstui/config.yaml` (system), `~/.config/zfstui/config.yaml` (user), `--config` override
- Snapshot name format: `{prefix}-{YYYYMMDD-HHMMSS}` → full `dataset@prefix-YYYYMMDD-HHMMSS`
- Cron entrypoint: `zfstui run --schedule NAME [--config PATH] [--dry-run]`
- Retention match: snapshots named `dataset@prefix-*` on that dataset
- Retention both rules: never delete newest N; among the rest delete older than max_age
- Managed system cron file: `/etc/cron.d/zfstui` with header `# Managed by zfstui — do not edit by hand`
- Managed user crontab markers: `# BEGIN zfstui managed` / `# END zfstui managed`
- Dry-run must not execute mutating `zfs` or crontab writes
- Default `go test ./...` must pass without real ZFS or root
- Target OS for runtime: Linux (build may be cross-compiled from Windows)

---

## File Structure

```
zfstui/
  go.mod
  go.sum
  Makefile
  README.md
  cmd/zfstui/main.go
  internal/version/version.go
  internal/config/config.go
  internal/config/config_test.go
  internal/config/paths.go
  internal/schedule/cron.go
  internal/schedule/cron_test.go
  internal/schedule/retention.go
  internal/schedule/retention_test.go
  internal/schedule/name.go
  internal/schedule/name_test.go
  internal/zfs/zfs.go
  internal/zfs/client.go
  internal/zfs/fake.go
  internal/zfs/client_test.go
  internal/restore/restore.go
  internal/restore/restore_test.go
  internal/cron/render.go
  internal/cron/render_test.go
  internal/cron/install.go
  internal/cron/install_test.go
  internal/app/run.go
  internal/app/run_test.go
  internal/cli/root.go
  internal/ui/styles.go
  internal/ui/app.go
  internal/ui/keys.go
  internal/ui/screens/home.go
  internal/ui/screens/datasets.go
  internal/ui/screens/snapshots.go
  internal/ui/screens/schedules.go
  internal/ui/screens/schedule_form.go
  internal/ui/screens/crontab.go
  internal/ui/screens/restore.go
  internal/ui/screens/confirm.go
  docs/superpowers/specs/2026-07-28-zfstui-design.md
  docs/superpowers/plans/2026-07-28-zfstui-implementation.md
```

---

### Task 1: Scaffold module and version

**Files:**
- Create: `go.mod`
- Create: `internal/version/version.go`
- Create: `cmd/zfstui/main.go`
- Create: `Makefile`

**Interfaces:**
- Produces: `version.Version` string constant (default `"0.1.0-dev"`); `main` prints version when args are `version`

- [ ] **Step 1: Initialize module**

```bash
cd "$(git rev-parse --show-toplevel)"
go mod init github.com/zfstui/zfstui
```

Expected: `go.mod` created with Go 1.22+

- [ ] **Step 2: Write version package and minimal main**

`internal/version/version.go`:

```go
package version

// Version is set at build time via -ldflags when packaging.
var Version = "0.1.0-dev"
```

`cmd/zfstui/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/zfstui/zfstui/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version.Version)
		return
	}
	fmt.Fprintln(os.Stderr, "zfstui: not implemented yet")
	os.Exit(1)
}
```

`Makefile`:

```makefile
BINARY=zfstui
VERSION?=0.1.0-dev
LDFLAGS=-s -w -X github.com/zfstui/zfstui/internal/version.Version=$(VERSION)

.PHONY: build test install clean

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/zfstui

test:
	go test ./...

install: build
	install -m 755 bin/$(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -rf bin
```

- [ ] **Step 3: Verify build and version**

```bash
go build -o bin/zfstui ./cmd/zfstui
./bin/zfstui version
```

Expected: prints `0.1.0-dev` (on Windows: `go build -o bin/zfstui.exe ./cmd/zfstui` then `.\bin\zfstui.exe version`)

- [ ] **Step 4: Commit**

```bash
git add go.mod cmd/zfstui/main.go internal/version/version.go Makefile
git commit -m "chore: scaffold zfstui module and version command"
```

---

### Task 2: Config load, save, validate

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/paths.go`
- Create: `internal/config/config_test.go`

**Interfaces:**
- Produces:
  - `type Retention struct { KeepLast *int; MaxAge string }`
  - `type Schedule struct { Name, Dataset, Cron, Prefix string; Recursive, Enabled bool; Retention Retention }`
  - `type Defaults struct { SnapshotPrefix string; Recursive bool }`
  - `type UI struct { Theme string }`
  - `type Config struct { Version int; Defaults Defaults; Schedules []Schedule; UI UI }`
  - `func Default() Config`
  - `func Load(path string) (*Config, error)`
  - `func (c *Config) Save(path string) error`
  - `func (c *Config) Validate() error`
  - `func (c *Config) FindSchedule(name string) (*Schedule, error)`
  - `func ResolvePath(explicit string) (string, error)` — explicit, else root→`/etc/zfstui/config.yaml` if exists, else `~/.config/zfstui/config.yaml`

- [ ] **Step 1: Write failing tests**

`internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zfstui/zfstui/internal/config"
)

func TestLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	keep := 24
	cfg := config.Config{
		Version: 1,
		Defaults: config.Defaults{SnapshotPrefix: "zfstui", Recursive: false},
		Schedules: []config.Schedule{{
			Name: "tank-home-hourly", Dataset: "tank/home", Cron: "0 * * * *",
			Prefix: "zfstui", Enabled: true, Retention: config.Retention{KeepLast: &keep, MaxAge: "7d"},
		}},
		UI: config.UI{Theme: "default"},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(got.Schedules) != 1 || got.Schedules[0].Name != "tank-home-hourly" {
		t.Fatalf("unexpected: %+v", got)
	}
	if got.Schedules[0].Retention.KeepLast == nil || *got.Schedules[0].Retention.KeepLast != 24 {
		t.Fatalf("keep_last: %+v", got.Schedules[0].Retention)
	}
}

func TestValidateRejectsEmptyName(t *testing.T) {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{{Name: "", Dataset: "tank/a", Cron: "0 * * * *", Prefix: "p", Enabled: true}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateRejectsDuplicateNames(t *testing.T) {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{
		{Name: "a", Dataset: "tank/a", Cron: "0 * * * *", Prefix: "p", Enabled: true},
		{Name: "a", Dataset: "tank/b", Cron: "0 * * * *", Prefix: "p", Enabled: true},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestFindSchedule(t *testing.T) {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{{Name: "hourly", Dataset: "tank/a", Cron: "0 * * * *", Prefix: "p", Enabled: true}}
	s, err := cfg.FindSchedule("hourly")
	if err != nil || s.Dataset != "tank/a" {
		t.Fatalf("got %+v err %v", s, err)
	}
	if _, err := cfg.FindSchedule("missing"); err == nil {
		t.Fatal("expected missing error")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !os.IsNotExist(err) && !filepath.IsAbs(err.Error()) {
		// Load should wrap with path; at minimum error non-nil
		t.Log(err)
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./internal/config/ -v
```

Expected: FAIL (package/types missing)

- [ ] **Step 3: Implement config package**

`internal/config/config.go`:

```go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Retention struct {
	KeepLast *int  `yaml:"keep_last,omitempty"`
	MaxAge   string `yaml:"max_age,omitempty"`
}

type Schedule struct {
	Name      string    `yaml:"name"`
	Dataset   string    `yaml:"dataset"`
	Recursive bool      `yaml:"recursive"`
	Cron      string    `yaml:"cron"`
	Prefix    string    `yaml:"prefix"`
	Enabled   bool      `yaml:"enabled"`
	Retention Retention `yaml:"retention"`
}

type Defaults struct {
	SnapshotPrefix string `yaml:"snapshot_prefix"`
	Recursive      bool   `yaml:"recursive"`
}

type UI struct {
	Theme string `yaml:"theme"`
}

type Config struct {
	Version   int        `yaml:"version"`
	Defaults  Defaults   `yaml:"defaults"`
	Schedules []Schedule `yaml:"schedules"`
	UI        UI         `yaml:"ui"`
}

func Default() Config {
	return Config{
		Version:  1,
		Defaults: Defaults{SnapshotPrefix: "zfstui", Recursive: false},
		UI:       UI{Theme: "default"},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Fill empty prefixes from defaults
	for i := range cfg.Schedules {
		if cfg.Schedules[i].Prefix == "" {
			cfg.Schedules[i].Prefix = cfg.Defaults.SnapshotPrefix
		}
	}
	return &cfg, nil
}

func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Config) Validate() error {
	if c.Version == 0 {
		c.Version = 1
	}
	if c.Defaults.SnapshotPrefix == "" {
		return errors.New("defaults.snapshot_prefix is required")
	}
	seen := map[string]struct{}{}
	for i, s := range c.Schedules {
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("schedules[%d]: name is required", i)
		}
		if strings.TrimSpace(s.Dataset) == "" {
			return fmt.Errorf("schedule %q: dataset is required", s.Name)
		}
		if strings.TrimSpace(s.Cron) == "" {
			return fmt.Errorf("schedule %q: cron is required", s.Name)
		}
		if strings.TrimSpace(s.Prefix) == "" {
			return fmt.Errorf("schedule %q: prefix is required", s.Name)
		}
		if s.Retention.KeepLast != nil && *s.Retention.KeepLast < 0 {
			return fmt.Errorf("schedule %q: keep_last must be >= 0", s.Name)
		}
		key := s.Name
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate schedule name %q", s.Name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (c *Config) FindSchedule(name string) (*Schedule, error) {
	for i := range c.Schedules {
		if c.Schedules[i].Name == name {
			return &c.Schedules[i], nil
		}
	}
	return nil, fmt.Errorf("schedule %q not found", name)
}
```

`internal/config/paths.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	SystemConfigPath = "/etc/zfstui/config.yaml"
)

func UserConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "zfstui", "config.yaml"), nil
}

// ResolvePath returns explicit if non-empty; else system config if readable; else user config path.
func ResolvePath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if _, err := os.Stat(SystemConfigPath); err == nil {
		return SystemConfigPath, nil
	}
	return UserConfigPath()
}

// EnsureDefault writes Default() config if path does not exist.
func EnsureDefault(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	cfg := Default()
	if err := cfg.Save(path); err != nil {
		return fmt.Errorf("create default config: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Add yaml dependency and pass tests**

```bash
go get gopkg.in/yaml.v3
go test ./internal/config/ -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/config/
git commit -m "feat: add config load/save/validate"
```

---

### Task 3: Snapshot naming, cron validation, retention selection

**Files:**
- Create: `internal/schedule/name.go`
- Create: `internal/schedule/name_test.go`
- Create: `internal/schedule/cron.go`
- Create: `internal/schedule/cron_test.go`
- Create: `internal/schedule/retention.go`
- Create: `internal/schedule/retention_test.go`

**Interfaces:**
- Produces:
  - `func SnapshotName(prefix string, t time.Time) string` → `prefix-YYYYMMDD-HHMMSS`
  - `func FullSnapshot(dataset, snapName string) string` → `dataset@snapName`
  - `func MatchesPrefix(fullSnapshot, dataset, prefix string) bool`
  - `func ValidateCron(expr string) error` — 5-field basic validation (split fields, non-empty)
  - `type SnapInfo struct { Name string; Created time.Time }` — Name is full `dataset@snap` or short; use full consistently as `dataset@snap`
  - `func SelectForDeletion(snaps []SnapInfo, keepLast *int, maxAge *time.Duration, now time.Time) []SnapInfo`
  - `func ParseMaxAge(s string) (time.Duration, error)` — supports `Nh`, `Nd`, `Nw` (hours/days/weeks)

- [ ] **Step 1: Write failing tests**

`internal/schedule/name_test.go`:

```go
package schedule_test

import (
	"testing"
	"time"

	"github.com/zfstui/zfstui/internal/schedule"
)

func TestSnapshotName(t *testing.T) {
	ts := time.Date(2026, 7, 28, 15, 4, 5, 0, time.UTC)
	got := schedule.SnapshotName("zfstui", ts)
	if got != "zfstui-20260728-150405" {
		t.Fatalf("got %q", got)
	}
}

func TestMatchesPrefix(t *testing.T) {
	if !schedule.MatchesPrefix("tank/home@zfstui-20260728-150405", "tank/home", "zfstui") {
		t.Fatal("expected match")
	}
	if schedule.MatchesPrefix("tank/home@daily-1", "tank/home", "zfstui") {
		t.Fatal("should not match other prefix")
	}
	if schedule.MatchesPrefix("tank/other@zfstui-1", "tank/home", "zfstui") {
		t.Fatal("wrong dataset")
	}
}
```

`internal/schedule/cron_test.go`:

```go
package schedule_test

import (
	"testing"

	"github.com/zfstui/zfstui/internal/schedule"
)

func TestValidateCronOK(t *testing.T) {
	for _, expr := range []string{"0 * * * *", "30 2 * * *", "*/15 * * * *"} {
		if err := schedule.ValidateCron(expr); err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
	}
}

func TestValidateCronBad(t *testing.T) {
	for _, expr := range []string{"", "0 * * *", "not-a-cron", "0 0 0 0 0 0 0"} {
		if err := schedule.ValidateCron(expr); err == nil {
			t.Fatalf("expected error for %q", expr)
		}
	}
}
```

`internal/schedule/retention_test.go`:

```go
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
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./internal/schedule/ -v
```

- [ ] **Step 3: Implement**

`internal/schedule/name.go`:

```go
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
```

`internal/schedule/cron.go`:

```go
package schedule

import (
	"fmt"
	"strings"
)

// ValidateCron checks a standard 5-field cron expression (minute hour dom month dow).
func ValidateCron(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("cron must have 5 fields, got %d", len(fields))
	}
	for i, f := range fields {
		if f == "" {
			return fmt.Errorf("cron field %d is empty", i)
		}
	}
	return nil
}
```

`internal/schedule/retention.go`:

```go
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
```

**Important retention logic check:** When only `maxAge` is set, `keepLast` is nil so `protected` is false for all; then `maxAge != nil` branch deletes if older. When only `keepLast`, we delete outside N. When both, protected skip for top N; outside N only deleted if older than maxAge. Good.

- [ ] **Step 4: Pass tests**

```bash
go test ./internal/schedule/ -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/schedule/
git commit -m "feat: snapshot naming, cron validation, retention selection"
```

---

### Task 4: ZFS client interface, fake, and real CLI wrapper

**Files:**
- Create: `internal/zfs/zfs.go`
- Create: `internal/zfs/client.go`
- Create: `internal/zfs/fake.go`
- Create: `internal/zfs/client_test.go`

**Interfaces:**
- Produces:
```go
type Pool struct { Name string; Health string; Size, Alloc string }
type Dataset struct { Name string; Used, Avail, Mountpoint, Type string }
type Snapshot struct { Name string; Used string; Creation time.Time } // Name full dataset@snap

type Client interface {
    ListPools(ctx context.Context) ([]Pool, error)
    ListDatasets(ctx context.Context, poolOrDataset string) ([]Dataset, error)
    ListSnapshots(ctx context.Context, dataset string) ([]Snapshot, error)
    CreateSnapshot(ctx context.Context, fullName string, recursive bool) error
    DestroySnapshot(ctx context.Context, fullName string, recursive bool) error
    Rollback(ctx context.Context, fullName string, recursive bool) error
    Clone(ctx context.Context, snapshot, newDataset string) error
    Promote(ctx context.Context, dataset string) error
    Available(ctx context.Context) error // zfs version / which check
}

func NewCLI(dryRun bool) *CLI
type CLI struct { DryRun bool; ZfsPath, ZpoolPath string; Runner func(ctx, name string, args ...string) (string, error) }
```

- [ ] **Step 1: Write tests using injectable Runner**

`internal/zfs/client_test.go`:

```go
package zfs_test

import (
	"context"
	"strings"
	"testing"

	"github.com/zfstui/zfstui/internal/zfs"
)

func TestListPoolsParses(t *testing.T) {
	c := zfs.NewCLI(false)
	c.Runner = func(ctx context.Context, name string, args ...string) (string, error) {
		if name != "zpool" {
			t.Fatalf("cmd %s", name)
		}
		return "tank\tONLINE\t10G\t2G\n", nil
	}
	pools, err := c.ListPools(context.Background())
	if err != nil || len(pools) != 1 || pools[0].Name != "tank" {
		t.Fatalf("%+v %v", pools, err)
	}
}

func TestCreateSnapshotDryRun(t *testing.T) {
	c := zfs.NewCLI(true)
	called := false
	c.Runner = func(ctx context.Context, name string, args ...string) (string, error) {
		called = true
		return "", nil
	}
	if err := c.CreateSnapshot(context.Background(), "tank/home@snap1", false); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("dry-run must not invoke runner")
	}
}

func TestCreateSnapshotArgs(t *testing.T) {
	c := zfs.NewCLI(false)
	var got []string
	c.Runner = func(ctx context.Context, name string, args ...string) (string, error) {
		got = append([]string{name}, args...)
		return "", nil
	}
	_ = c.CreateSnapshot(context.Background(), "tank/home@snap1", true)
	if strings.Join(got, " ") != "zfs snapshot -r tank/home@snap1" {
		t.Fatalf("got %v", got)
	}
}

func TestFakeListAndCreate(t *testing.T) {
	f := zfs.NewFake()
	f.Pools = []zfs.Pool{{Name: "tank", Health: "ONLINE"}}
	f.Datasets = []zfs.Dataset{{Name: "tank/home", Type: "filesystem"}}
	if err := f.CreateSnapshot(context.Background(), "tank/home@a", false); err != nil {
		t.Fatal(err)
	}
	snaps, err := f.ListSnapshots(context.Background(), "tank/home")
	if err != nil || len(snaps) != 1 {
		t.Fatalf("%+v %v", snaps, err)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/zfs/ -v
```

- [ ] **Step 3: Implement**

`internal/zfs/zfs.go`:

```go
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
	Name     string
	Used     string
	Creation time.Time
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
```

`internal/zfs/client.go`:

```go
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
	out, err := c.Runner(ctx, c.ZfsPath, "list", "-H", "-r", "-t", "filesystem,volume",
		"-o", "name,used,avail,mountpoint,type", poolOrDataset)
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
	out, err := c.Runner(ctx, c.ZfsPath, "list", "-H", "-r", "-t", "snapshot",
		"-o", "name,used,creation", "-s", "creation", dataset)
	if err != nil {
		return nil, err
	}
	var snaps []Snapshot
	for _, line := range splitLines(out) {
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		// creation may be unix timestamp if -p used; without -p it's a date string.
		// Use -p for parseability — prefer re-call with -p in production code:
		// For tests inject unix. Production:
		cre := parseCreation(f[2])
		snaps = append(snaps, Snapshot{Name: f[0], Used: f[1], Creation: cre})
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
```

**Refine ListSnapshots** to use `-p` for creation:

```go
out, err := c.Runner(ctx, c.ZfsPath, "list", "-H", "-p", "-r", "-t", "snapshot",
    "-o", "name,used,creation", "-s", "creation", dataset)
```

`internal/zfs/fake.go`:

```go
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
		if strings.HasPrefix(s.Name, dataset+"@") {
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
	f.Log = append(f.Log, "rollback "+fullName)
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
```

- [ ] **Step 4: Pass tests**

```bash
go test ./internal/zfs/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/zfs/
git commit -m "feat: zfs client interface, CLI wrapper, and fake"
```

---

### Task 5: Restore package

**Files:**
- Create: `internal/restore/restore.go`
- Create: `internal/restore/restore_test.go`

**Interfaces:**
- Consumes: `zfs.Client`
- Produces:
```go
type Service struct { Z zfs.Client }
func (s *Service) Rollback(ctx, snapshot string, recursive bool) error
func (s *Service) Clone(ctx, snapshot, newDataset string, promote bool) error
func NewerSnapshots(all []zfs.Snapshot, target string) []zfs.Snapshot // same dataset, created after target
```

- [ ] **Step 1: Tests**

```go
package restore_test

import (
	"context"
	"testing"
	"time"

	"github.com/zfstui/zfstui/internal/restore"
	"github.com/zfstui/zfstui/internal/zfs"
)

func TestClonePromote(t *testing.T) {
	f := zfs.NewFake()
	f.Snapshots = []zfs.Snapshot{{Name: "tank/home@s1", Creation: time.Now()}}
	svc := &restore.Service{Z: f}
	if err := svc.Clone(context.Background(), "tank/home@s1", "tank/home-restored", true); err != nil {
		t.Fatal(err)
	}
	if len(f.Log) != 2 || f.Log[0] != "clone tank/home@s1 tank/home-restored" || f.Log[1] != "promote tank/home-restored" {
		t.Fatalf("%v", f.Log)
	}
}

func TestNewerSnapshots(t *testing.T) {
	base := time.Now()
	all := []zfs.Snapshot{
		{Name: "tank/home@a", Creation: base},
		{Name: "tank/home@b", Creation: base.Add(time.Hour)},
		{Name: "tank/other@c", Creation: base.Add(2 * time.Hour)},
	}
	newer := restore.NewerSnapshots(all, "tank/home@a")
	if len(newer) != 1 || newer[0].Name != "tank/home@b" {
		t.Fatalf("%+v", newer)
	}
}
```

- [ ] **Step 2: Implement and pass**

```go
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
```

```bash
go test ./internal/restore/ -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/restore/
git commit -m "feat: restore rollback and clone/promote"
```

---

### Task 6: Cron render and install/uninstall

**Files:**
- Create: `internal/cron/render.go`
- Create: `internal/cron/render_test.go`
- Create: `internal/cron/install.go`
- Create: `internal/cron/install_test.go`

**Interfaces:**
- Consumes: `config.Config`
- Produces:
```go
const BeginMarker = "# BEGIN zfstui managed"
const EndMarker = "# END zfstui managed"
const SystemHeader = "# Managed by zfstui — do not edit by hand"
const SystemPath = "/etc/cron.d/zfstui"

func RenderSystem(cfg *config.Config, binary, configPath string) (string, error)
func RenderUserBlock(cfg *config.Config, binary, configPath string) (string, error)
func MergeUserCrontab(existing, block string) string
func StripUserBlock(existing string) string
func WriteSystemFile(path, content string) error  // atomic write; tests use temp path
func InstallSystem(cfg, binary, configPath, path string) error
func InstallUser(cfg, binary, configPath string, readCrontab, writeCrontab func() (string, error) / write) error
```

Cron line format (system):

```
# Managed by zfstui — do not edit by hand
SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin

0 * * * * root /usr/local/bin/zfstui run --config /etc/zfstui/config.yaml --schedule tank-home-hourly
```

Only **enabled** schedules. Validate each cron via `schedule.ValidateCron` before render.

- [ ] **Step 1: Tests for render and user block merge**

```go
package cron_test

import (
	"strings"
	"testing"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/cron"
)

func TestRenderSystemEnabledOnly(t *testing.T) {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{
		{Name: "on", Dataset: "tank/a", Cron: "0 * * * *", Prefix: "p", Enabled: true},
		{Name: "off", Dataset: "tank/b", Cron: "0 1 * * *", Prefix: "p", Enabled: false},
	}
	out, err := cron.RenderSystem(&cfg, "/usr/local/bin/zfstui", "/etc/zfstui/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, cron.SystemHeader) {
		t.Fatal("missing header")
	}
	if !strings.Contains(out, "run --config /etc/zfstui/config.yaml --schedule on") {
		t.Fatal(out)
	}
	if strings.Contains(out, "schedule off") {
		t.Fatal("disabled should be omitted")
	}
	if !strings.Contains(out, "0 * * * * root ") {
		t.Fatal("system lines need user field root")
	}
}

func TestMergeUserCrontab(t *testing.T) {
	existing := "MAILTO=me\n0 0 * * * /bin/true\n"
	block := cron.BeginMarker + "\n1 1 * * * /usr/local/bin/zfstui run --schedule x\n" + cron.EndMarker + "\n"
	merged := cron.MergeUserCrontab(existing, block)
	if !strings.Contains(merged, "MAILTO=me") || !strings.Contains(merged, cron.BeginMarker) {
		t.Fatal(merged)
	}
	// second merge replaces block
	block2 := cron.BeginMarker + "\n2 2 * * * /usr/local/bin/zfstui run --schedule y\n" + cron.EndMarker + "\n"
	merged2 := cron.MergeUserCrontab(merged, block2)
	if strings.Contains(merged2, "schedule x") || !strings.Contains(merged2, "schedule y") {
		t.Fatal(merged2)
	}
	stripped := cron.StripUserBlock(merged2)
	if strings.Contains(stripped, cron.BeginMarker) {
		t.Fatal(stripped)
	}
}
```

- [ ] **Step 2: Implement render/install**

`internal/cron/render.go`:

```go
package cron

import (
	"fmt"
	"strings"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/schedule"
)

const (
	BeginMarker  = "# BEGIN zfstui managed"
	EndMarker    = "# END zfstui managed"
	SystemHeader = "# Managed by zfstui — do not edit by hand"
	SystemPath   = "/etc/cron.d/zfstui"
)

func RenderSystem(cfg *config.Config, binary, configPath string) (string, error) {
	var b strings.Builder
	b.WriteString(SystemHeader + "\n")
	b.WriteString("SHELL=/bin/sh\n")
	b.WriteString("PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin\n\n")
	for _, s := range cfg.Schedules {
		if !s.Enabled {
			continue
		}
		if err := schedule.ValidateCron(s.Cron); err != nil {
			return "", fmt.Errorf("schedule %q: %w", s.Name, err)
		}
		fmt.Fprintf(&b, "%s root %s run --config %s --schedule %s\n", s.Cron, binary, configPath, s.Name)
	}
	return b.String(), nil
}

func RenderUserBlock(cfg *config.Config, binary, configPath string) (string, error) {
	var b strings.Builder
	b.WriteString(BeginMarker + "\n")
	for _, s := range cfg.Schedules {
		if !s.Enabled {
			continue
		}
		if err := schedule.ValidateCron(s.Cron); err != nil {
			return "", fmt.Errorf("schedule %q: %w", s.Name, err)
		}
		// user crontab: no user field
		fmt.Fprintf(&b, "%s %s run --config %s --schedule %s\n", s.Cron, binary, configPath, s.Name)
	}
	b.WriteString(EndMarker + "\n")
	return b.String(), nil
}

func MergeUserCrontab(existing, block string) string {
	stripped := StripUserBlock(existing)
	if stripped != "" && !strings.HasSuffix(stripped, "\n") {
		stripped += "\n"
	}
	return stripped + block
}

func StripUserBlock(existing string) string {
	start := strings.Index(existing, BeginMarker)
	if start < 0 {
		return existing
	}
	end := strings.Index(existing[start:], EndMarker)
	if end < 0 {
		return existing[:start]
	}
	end = start + end + len(EndMarker)
	// drop trailing newline after end marker
	rest := existing[end:]
	rest = strings.TrimPrefix(rest, "\n")
	return existing[:start] + rest
}
```

`internal/cron/install.go`:

```go
package cron

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zfstui/zfstui/internal/config"
)

func WriteFileAtomic(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func InstallSystemFile(cfg *config.Config, binary, configPath, destPath string) error {
	body, err := RenderSystem(cfg, binary, configPath)
	if err != nil {
		return err
	}
	return WriteFileAtomic(destPath, body)
}

func UninstallSystemFile(destPath string) error {
	err := os.Remove(destPath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ReadUserCrontab / WriteUserCrontab use `crontab -l` and `crontab -` for real installs.
func ReadUserCrontab() (string, error) {
	out, err := exec.Command("crontab", "-l").CombinedOutput()
	if err != nil {
		// empty crontab often exits 1
		if strings.Contains(string(out), "no crontab") {
			return "", nil
		}
		return "", fmt.Errorf("crontab -l: %w: %s", err, out)
	}
	return string(out), nil
}

func WriteUserCrontab(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("crontab -: %w: %s", err, out)
	}
	return nil
}

func InstallUser(cfg *config.Config, binary, configPath string) error {
	block, err := RenderUserBlock(cfg, binary, configPath)
	if err != nil {
		return err
	}
	existing, err := ReadUserCrontab()
	if err != nil {
		return err
	}
	return WriteUserCrontab(MergeUserCrontab(existing, block))
}

func UninstallUser() error {
	existing, err := ReadUserCrontab()
	if err != nil {
		return err
	}
	return WriteUserCrontab(StripUserBlock(existing))
}

type Status struct {
	Mode      string // "system" | "user" | "none"
	Path      string
	Installed bool
	Preview   string
}

func Preview(cfg *config.Config, binary, configPath string, system bool) (string, error) {
	if system {
		return RenderSystem(cfg, binary, configPath)
	}
	return RenderUserBlock(cfg, binary, configPath)
}
```

- [ ] **Step 3: Pass tests**

```bash
go test ./internal/cron/ -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/cron/
git commit -m "feat: cron render and install/uninstall helpers"
```

---

### Task 7: Run/snapshot/prune application logic

**Files:**
- Create: `internal/app/run.go`
- Create: `internal/app/run_test.go`

**Interfaces:**
- Consumes: `config.Config`, `zfs.Client`, `schedule.*`
- Produces:
```go
type Runner struct {
    Z   zfs.Client
    Now func() time.Time
}
func (r *Runner) RunSchedule(ctx context.Context, cfg *config.Config, name string) (created string, pruned []string, err error)
func (r *Runner) Snapshot(ctx, dataset, prefix string, recursive bool) (full string, err error)
func (r *Runner) PruneSchedule(ctx, cfg *config.Config, name string) (pruned []string, err error)
```

- [ ] **Step 1: Tests with Fake zfs**

```go
package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/zfstui/zfstui/internal/app"
	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/zfs"
)

func TestRunScheduleCreatesAndPrunes(t *testing.T) {
	f := zfs.NewFake()
	f.Datasets = []zfs.Dataset{{Name: "tank/home"}}
	now := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	// seed old snaps
	for i := 0; i < 5; i++ {
		name := "tank/home@zfstui-" + now.Add(-time.Duration(i+1)*time.Hour).Format("20060102-150405")
		f.Snapshots = append(f.Snapshots, zfs.Snapshot{Name: name, Creation: now.Add(-time.Duration(i+1) * time.Hour)})
	}
	keep := 2
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{{
		Name: "hourly", Dataset: "tank/home", Cron: "0 * * * *", Prefix: "zfstui",
		Enabled: true, Retention: config.Retention{KeepLast: &keep},
	}}
	r := &app.Runner{Z: f, Now: func() time.Time { return now }}
	created, pruned, err := r.RunSchedule(context.Background(), &cfg, "hourly")
	if err != nil {
		t.Fatal(err)
	}
	if created == "" || !containsPrefix(created, "tank/home@zfstui-") {
		t.Fatalf("created %q", created)
	}
	// after create there are 6; keep 2 → prune 4
	if len(pruned) < 1 {
		t.Fatalf("expected prunes, got %v (snaps=%d)", pruned, len(f.Snapshots))
	}
}

func containsPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
```

- [ ] **Step 2: Implement**

```go
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
	if s.Retention.KeepLast == nil && s.Retention.MaxAge == "" {
		return nil, nil
	}
	var maxAge *time.Duration
	if s.Retention.MaxAge != "" {
		d, err := schedule.ParseMaxAge(s.Retention.MaxAge)
		if err != nil {
			return nil, err
		}
		maxAge = &d
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
	toDel := schedule.SelectForDeletion(matched, s.Retention.KeepLast, maxAge, r.now())
	var pruned []string
	for _, d := range toDel {
		if err := r.Z.DestroySnapshot(ctx, d.Name, false); err != nil {
			return pruned, fmt.Errorf("destroy %s: %w", d.Name, err)
		}
		pruned = append(pruned, d.Name)
	}
	return pruned, nil
}
```

- [ ] **Step 3: Pass tests + commit**

```bash
go test ./internal/app/ -v
git add internal/app/
git commit -m "feat: schedule run, snapshot, and prune logic"
```

---

### Task 8: CLI wiring (all subcommands)

**Files:**
- Create: `internal/cli/root.go`
- Modify: `cmd/zfstui/main.go`

**Interfaces:**
- Global flags: `--config`, `--dry-run`
- Commands: default TUI (stub OK until Task 9), `version`, `run`, `snapshot`, `prune`, `cron install|uninstall|status`

Use `flag` package with manual subcommand switch (no new heavy deps) **or** `github.com/spf13/cobra` if preferred — **prefer stdlib `flag` + switch for YAGNI**.

- [ ] **Step 1: Implement CLI**

`internal/cli/root.go` — parse `os.Args`, load config via `config.ResolvePath`, construct `zfs.NewCLI(dryRun)`, `app.Runner`, dispatch:

| Args | Action |
|------|--------|
| (none) | call `ui.Run(...)` (Task 9; for now print "use TUI soon" only if ui not ready — Task 8 should leave a `var StartTUI func(...) error` hook or import ui package later) |
| `version` | print version |
| `run --schedule X` | Runner.RunSchedule; print created/pruned |
| `snapshot --dataset D [--prefix P] [--recursive]` | Runner.Snapshot |
| `prune --schedule X` | Runner.PruneSchedule |
| `cron install [--system]` | InstallSystemFile or InstallUser |
| `cron uninstall [--system]` | Uninstall |
| `cron status` | print mode + preview |

Binary path for cron: `os.Executable()` resolved, fallback `zfstui`.

- [ ] **Step 2: Wire main**

```go
package main

import (
	"os"

	"github.com/zfstui/zfstui/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
```

- [ ] **Step 3: Manual smoke (no zfs required for version/help)**

```bash
go run ./cmd/zfstui version
go run ./cmd/zfstui -h   # or help
```

- [ ] **Step 4: Commit**

```bash
git add cmd/zfstui/main.go internal/cli/
git commit -m "feat: CLI subcommands for run, snapshot, prune, cron"
```

---

### Task 9: TUI shell — styles, keys, home screen

**Files:**
- Create: `internal/ui/styles.go`
- Create: `internal/ui/keys.go`
- Create: `internal/ui/app.go`
- Create: `internal/ui/screens/home.go`
- Modify: `internal/cli/root.go` to launch TUI on no args

**Interfaces:**
- Produces: `func Run(cfgPath string, cfg *config.Config, z zfs.Client, dryRun bool) error`
- Pretty dark theme: title cyan/magenta, selection inverted, danger red, ok green, muted gray
- Home menu items: Pools & Datasets, Schedules, Crontab, Toggle Dry-Run, Quit

Dependencies:

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/lipgloss@latest
go get github.com/charmbracelet/bubbles@latest
```

- [ ] **Step 1: Implement styles + app model with stack navigation**

Pattern: `App` holds `[]tea.Model` stack or screen enum + shared `Deps`:

```go
type Deps struct {
    CfgPath string
    Cfg     *config.Config
    Z       zfs.Client
    DryRun  bool
    Binary  string
}
```

Home: list of menu entries with lipgloss title banner:

```
╔══════════════════════════════════════╗
║              zfstui                  ║
║     ZFS Snapshot Manager             ║
╚══════════════════════════════════════╝
```

(Use Unicode box drawing + lipgloss colors; fall back if needed.)

Keys: `j/k`, arrows, enter, `q`, `?`, `D` dry-run toggle (updates Deps and zfs CLI dry-run if concrete `*zfs.CLI`)

- [ ] **Step 2: Manual run**

```bash
go run ./cmd/zfstui
```

Expected: TUI home renders; `q` quits.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/ go.mod go.sum internal/cli/
git commit -m "feat: TUI shell with home screen and theme"
```

---

### Task 10: TUI datasets and snapshots

**Files:**
- Create: `internal/ui/screens/datasets.go`
- Create: `internal/ui/screens/snapshots.go`
- Create: `internal/ui/screens/confirm.go`
- Modify: `internal/ui/app.go`

**Behavior:**
- From Home → Pools: list pools; Enter → datasets under pool
- `/` filter datasets
- Enter dataset → snapshots list
- `c` create snapshot (prefix from config defaults, optional recursive confirm)
- `d` destroy selected with confirm modal
- Esc back
- Status bar shows dry-run and current path
- Errors shown in red footer for 5s or until key

Use `bubbles/list` or simple cursor over string rows for fewer deps friction.

- [ ] **Step 1: Implement screens with Fake-friendly client (real CLI at runtime)**
- [ ] **Step 2: Manual check on Linux with ZFS when available**
- [ ] **Step 3: Commit**

```bash
git commit -m "feat: TUI dataset browser and snapshot create/destroy"
```

---

### Task 11: TUI schedules and crontab screens

**Files:**
- Create: `internal/ui/screens/schedules.go`
- Create: `internal/ui/screens/schedule_form.go`
- Create: `internal/ui/screens/crontab.go`
- Modify: `internal/ui/app.go`, config save on schedule changes

**Behavior:**
- Schedules table: name, dataset, cron, enabled, retention summary
- `a` add, `e` edit, `x` delete (confirm), `space` toggle enabled
- Form fields: name, dataset, cron, prefix, recursive, keep_last, max_age — validate cron on save
- Save writes config via `cfg.Save(cfgPath)`
- Crontab screen: preview rendered lines; `i` install (system if root else user); `u` uninstall; show status
- Dry-run: install shows preview only, no write

- [ ] **Step 1: Implement + unit-test form validation helpers if extracted**
- [ ] **Step 2: Commit**

```bash
git commit -m "feat: TUI schedule editor and crontab install screen"
```

---

### Task 12: TUI restore wizard + polish

**Files:**
- Create: `internal/ui/screens/restore.go`
- Modify: snapshots screen `r` key
- Modify: styles/help

**Behavior:**
- Restore wizard steps: (1) Rollback vs Clone (2) if rollback show newer snaps + type dataset name (3) if clone enter new path + promote y/n (4) confirm
- Call `restore.Service`
- Help overlay `?`
- README + final Makefile targets

- [ ] **Step 1: Implement restore wizard**
- [ ] **Step 2: Write README**

`README.md` sections: Install, Quick start TUI, Config example, Cron install, CLI reference, Safety notes.

- [ ] **Step 3: Full test suite**

```bash
go test ./...
go build -o bin/zfstui ./cmd/zfstui
```

Expected: all pass; binary builds.

- [ ] **Step 4: Commit**

```bash
git add README.md internal/ui/ Makefile
git commit -m "feat: restore wizard, polish, and README"
```

---

## Self-Review (plan vs spec)

| Spec requirement | Task |
|------------------|------|
| Pretty TUI | 9–12 |
| Browse pools/datasets/snapshots | 10 |
| Manual create/destroy | 10 |
| Schedules in config | 2, 11 |
| Managed crontab | 6, 8, 11 |
| `zfstui run --schedule` | 7, 8 |
| Retention keep_last / max_age | 3, 7 |
| Restore rollback | 5, 12 |
| Restore clone (+ promote) | 5, 12 |
| Dry-run | 4, 8, 9 |
| Config paths | 2 |
| CLI for cron | 8 |
| Tests without real ZFS | 2–7 fakes |
| No daemon / no send-receive | honored (non-goals) |

**Placeholder scan:** None intentional; Tasks 9–12 are more UI-heavy with structural guidance — implementers should follow design keymap/screens exactly.

**Type consistency:** `config.Schedule`, `zfs.Client`, `schedule.SnapInfo`, `app.Runner` names are stable across tasks.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-28-zfstui-implementation.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks, fast iteration  
2. **Inline Execution** — execute tasks in this session with executing-plans, batch checkpoints  

Which approach?
