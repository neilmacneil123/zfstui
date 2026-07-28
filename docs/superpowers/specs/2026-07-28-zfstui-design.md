# zfstui — ZFS Snapshot Manager Design

**Date:** 2026-07-28  
**Status:** Approved for implementation planning  
**Stack:** Go + Bubble Tea (Lip Gloss, Bubbles)

## 1. Purpose

`zfstui` is a pretty Linux terminal UI for managing ZFS snapshots on a schedule. Users browse pools and datasets, create and destroy snapshots, define schedules and retention policies, and restore to the original dataset or a new one. Schedules are persisted in a config file; the tool installs them as managed crontab entries that invoke `zfs snapshot` (and optional prune logic).

## 2. Goals and non-goals

### Goals (v1)

- Interactive TUI with clear tables, status bar, and confirmations
- Dataset browser (pools → datasets → snapshots)
- Manual snapshot create/destroy
- Schedule management (cron expressions + labels)
- Config as source of truth; managed crontab install/uninstall/sync
- Restore: rollback in place, or clone/promote to a new dataset path
- Simple retention: keep last N and/or max age (per schedule)
- Dry-run mode for destructive and install operations
- Privilege checks and actionable errors when `zfs` is unavailable or permission is denied

### Non-goals (v1)

- Built-in long-running scheduler daemon
- ZFS send/receive, replication, or remote hosts
- Snapshot holds / tags as first-class UI (beyond naming conventions)
- Multi-user ACLs or web UI
- Wrapping third-party tools (sanoid, zfs-auto-snapshot)

## 3. Architecture

Layered Go application. The TUI is a client of pure(ish) packages so core logic is unit-testable without a real pool.

```
┌─────────────────────────────────────────┐
│  cmd/zfstui          main, flags, root  │
├─────────────────────────────────────────┤
│  internal/ui         Bubble Tea models  │
│    (screens, styles, keymaps)           │
├─────────────────────────────────────────┤
│  internal/config     load/save YAML     │
│  internal/cron       crontab file I/O   │
│  internal/schedule   cron + retention   │
│  internal/zfs        zfs/zpool wrapper  │
│  internal/restore    rollback / clone   │
└─────────────────────────────────────────┘
```

### Package responsibilities

| Package | Responsibility |
|---------|----------------|
| `cmd/zfstui` | Entry point, global flags (`--config`, `--dry-run`), version |
| `internal/ui` | Bubble Tea app, screens, Lip Gloss theme, key bindings |
| `internal/config` | YAML schema, defaults, validation, atomic save |
| `internal/cron` | Generate and install/uninstall managed cron.d or user crontab block |
| `internal/schedule` | Named schedules, cron parsing validation, retention evaluation |
| `internal/zfs` | List pools/datasets/snapshots; create/destroy snapshots; thin CLI wrapper |
| `internal/restore` | Rollback vs clone (and optional promote) orchestration |

### External dependencies (runtime)

- `zfs` and `zpool` on `PATH` (OpenZFS)
- `crontab` and/or write access to `/etc/cron.d/` (root install path)

### Binary

- Name: `zfstui`
- Single static-ish Go binary (CGO disabled preferred)

## 4. Features

### 4.1 Dataset browser

- List pools via `zpool list`
- For selected pool, list filesystems/volumes via `zfs list -r -t filesystem,volume`
- Show used, available, mountpoint, and snapshot count when cheap to obtain
- Filter/search by name

### 4.2 Snapshots

- List snapshots for selected dataset (`zfs list -t snapshot -r` or dataset-scoped)
- Create snapshot with auto name: `{dataset}@{prefix}-{timestamp}` (prefix configurable, default `zfstui`)
- Optional recursive create (`zfs snapshot -r`)
- Destroy selected snapshot with confirmation (and optional recursive destroy when applicable)
- Show creation time and used space when available

### 4.3 Schedules

Each schedule has:

| Field | Description |
|-------|-------------|
| `id` / `name` | Human label |
| `dataset` | Target dataset (or pool/dataset path) |
| `recursive` | Whether to use `zfs snapshot -r` |
| `cron` | Standard 5-field cron expression |
| `prefix` | Snapshot name prefix |
| `retention.keep_last` | Optional: keep newest N matching snapshots |
| `retention.max_age` | Optional: drop older than duration (e.g. `7d`, `30d`) |
| `enabled` | If false, omitted from installed crontab |

UI actions: add, edit, delete, enable/disable, **Install crontab**, **Uninstall crontab**, **Sync** (rewrite managed block from config).

### 4.4 Restore

Two primary paths:

1. **Rollback (original location)**  
   - `zfs rollback` to a selected snapshot  
   - Warn that newer snapshots and intervening changes will be lost  
   - Require typed confirmation of dataset name for destructive rollback  

2. **Clone to new dataset**  
   - `zfs clone snapshot new/dataset`  
   - Optional **promote** so the clone becomes independent of the origin snapshot  

UI flow: pick snapshot → choose Rollback or Clone → if clone, enter new dataset path → confirm → execute (respect dry-run).

### 4.5 Retention (v1)

Applied by the non-TUI path (`zfstui run` / `zfstui prune`) after snapshot create, not only inside the TUI.

- **Match rule:** snapshots on the schedule’s dataset named `{dataset}@{prefix}-*` (prefix from the schedule).
- **Ordering:** newest first (by ZFS creation time, or by timestamp embedded in the name as fallback).

**Deletion rules:**

| Config | Behavior |
|--------|----------|
| Only `keep_last: N` | Delete matching snapshots outside the newest N |
| Only `max_age: D` | Delete matching snapshots older than duration D |
| Both | Never delete a snapshot among the newest N; among the rest, delete those older than D |
| Neither | No prune step |

This guarantees a floor of recent snapshots while still aging out the long tail.

### 4.6 Dry-run and privileges

- Global `--dry-run` or session toggle: print intended `zfs`/`crontab` actions without executing  
- On start, detect whether process can run privileged `zfs` commands; show banner if limited  
- Install crontab for system-wide schedules prefers `/etc/cron.d/zfstui` when root; otherwise managed user crontab block with markers

## 5. Configuration

### Paths

| Path | Purpose |
|------|---------|
| `/etc/zfstui/config.yaml` | System-wide config (preferred when root) |
| `~/.config/zfstui/config.yaml` | User config fallback |
| `--config PATH` | Override |

### Schema (YAML)

```yaml
version: 1
defaults:
  snapshot_prefix: zfstui
  recursive: false
schedules:
  - name: tank-home-hourly
    dataset: tank/home
    recursive: false
    cron: "0 * * * *"
    prefix: zfstui
    enabled: true
    retention:
      keep_last: 24
      max_age: 7d
  - name: tank-data-daily
    dataset: tank/data
    recursive: true
    cron: "30 2 * * *"
    prefix: daily
    enabled: true
    retention:
      keep_last: 14
      max_age: 30d
ui:
  theme: default
```

### Managed crontab

**System (root):** file `/etc/cron.d/zfstui` with header comment `# Managed by zfstui — do not edit by hand`.

**User:** block in user crontab between:

```
# BEGIN zfstui managed
...
# END zfstui managed
```

Each schedule becomes one or two lines:

1. Snapshot: `cron expr root zfstui snapshot --config /etc/zfstui/config.yaml --schedule NAME`  
   (or equivalent: invoke `zfs snapshot` via the tool so naming stays consistent)

2. Prune (if retention set): either chained in the same job or a second line shortly after.

Prefer a single subcommand per schedule:

```
zfstui run --schedule NAME
```

which creates the snapshot then applies retention for that schedule. Dry-run:

```
zfstui run --schedule NAME --dry-run
```

### CLI surface (non-TUI, for cron and scripting)

| Command | Purpose |
|---------|---------|
| `zfstui` | Launch TUI (default) |
| `zfstui run --schedule NAME` | Snapshot + prune for one schedule |
| `zfstui snapshot --dataset D [--recursive] [--prefix P]` | One-shot snapshot |
| `zfstui prune --schedule NAME` | Retention only |
| `zfstui cron install` | Write managed crontab from config |
| `zfstui cron uninstall` | Remove managed crontab |
| `zfstui cron status` | Show whether installed and in sync |
| `zfstui version` | Version string |

The TUI remains the primary UX; CLI exists so cron does not launch Bubble Tea.

## 6. TUI design

### Theme

- Dark terminal-friendly palette via Lip Gloss  
- Status bar: context path, privilege mode, dry-run indicator  
- Help bar: contextual key hints  
- Tables: Bubbles table or custom list with zebra/selection highlight  
- Destructive dialogs: modal with yellow/red emphasis and typed confirm when needed  

### Screens

1. **Home** — Pools | Schedules | Crontab status | Help  
2. **Datasets** — hierarchical or flat filtered list under a pool  
3. **Snapshots** — list for dataset; actions Create / Destroy / Restore  
4. **Restore wizard** — Rollback vs Clone; path input; confirm  
5. **Schedules** — table of schedules; edit form; enable toggle  
6. **Schedule form** — fields validation (cron, dataset exists, retention)  
7. **Crontab** — status, install/uninstall/sync, preview generated lines  
8. **Confirm** — reusable modal  

### Keymap (defaults)

| Key | Action |
|-----|--------|
| `↑↓` / `j k` | Navigate |
| `Enter` | Select / drill in |
| `Esc` / `Backspace` | Back |
| `c` | Create snapshot (context) |
| `d` | Destroy (confirm) |
| `r` | Restore wizard |
| `s` | Schedules |
| `i` | Install crontab (from Crontab screen) |
| `u` | Uninstall crontab |
| `/` | Filter |
| `?` | Help |
| `q` | Quit |
| `D` | Toggle dry-run (session) |

## 7. Data flow

### Create scheduled snapshot (cron path)

1. cron invokes `zfstui run --schedule NAME --config PATH`  
2. Load config; find schedule; validate enabled  
3. Build snapshot name: `{prefix}-{YYYYMMDD-HHMMSS}`  
4. `zfs snapshot [-r] dataset@name`  
5. If retention configured, list matching snapshots, compute delete set, destroy  
6. Exit non-zero on failure; log to stderr  

### Install crontab (TUI or CLI)

1. Load config  
2. Render cron lines for all enabled schedules  
3. Write `/etc/cron.d/zfstui` or replace managed user block  
4. Report success / permission error  

### Restore clone path

1. User selects snapshot `pool/ds@snap`  
2. Chooses Clone, enters `pool/ds-restored`  
3. Confirm → `zfs clone pool/ds@snap pool/ds-restored`  
4. Optional promote → `zfs promote pool/ds-restored`  

### Restore rollback path

1. User selects snapshot  
2. Chooses Rollback; tool lists newer snapshots that will be destroyed  
3. User types dataset name to confirm  
4. `zfs rollback [-r] dataset@snap` as appropriate  

## 8. Error handling

| Condition | Behavior |
|-----------|----------|
| `zfs` not found | Startup error with install hint; exit or limited UI |
| Permission denied | Red status; message to re-run with privileges |
| Dataset missing | Form validation / action error toast |
| Invalid cron | Reject save with field error |
| Crontab write fail | Show path and errno; leave config unchanged |
| Snapshot name conflict | Append suffix or surface error (prefer surface; user renames) |
| Partial prune failure | Stop, report which destroy failed; leave remaining |

All external commands capture combined stdout/stderr for display in an error detail view.

## 9. Testing strategy

| Layer | Approach |
|-------|----------|
| `internal/config` | Parse/validate/round-trip YAML fixtures |
| `internal/schedule` | Cron validation; retention selection pure tests |
| `internal/cron` | Golden-file render of cron.d content; block replace on sample crontab |
| `internal/zfs` | Interface + fake implementation; optional integration tag when root+zfs available |
| `internal/restore` | Fake zfs client sequences for rollback/clone/promote |
| `internal/ui` | Light model tests for state transitions where practical; manual TUI check |

No real pool required for default `go test ./...`.

## 10. Project layout

```
zfstui/
  cmd/zfstui/main.go
  internal/
    config/
    cron/
    schedule/
    zfs/
    restore/
    ui/
      styles.go
      app.go
      screens/
  docs/superpowers/specs/2026-07-28-zfstui-design.md
  go.mod
  README.md
  Makefile
```

## 11. Implementation phases (for planning)

1. Scaffold module, config load/save, fake zfs interface  
2. Real zfs list/create/destroy  
3. Retention + `run` / `snapshot` / `prune` CLI  
4. Cron render + install/uninstall  
5. TUI: home, datasets, snapshots  
6. TUI: schedules, forms, crontab screen  
7. TUI: restore wizard + polish (theme, help, dry-run)  
8. README, Makefile install target, manual test checklist  

## 12. Success criteria

- User can open TUI, browse a real pool, create and destroy a snapshot  
- User can define a schedule, install crontab, and see a matching line in cron.d or crontab  
- `zfstui run --schedule …` creates a named snapshot and prunes per policy  
- User can restore via rollback or clone to a new dataset from the TUI  
- `go test ./...` passes without ZFS privileges  
- UI is cohesive: consistent colors, key help, confirmations on destroy/rollback  

## 13. Open decisions resolved in design

| Topic | Decision |
|-------|----------|
| Scheduler | Crontab generation only; no daemon |
| Config vs cron source of truth | Config file; cron is derived |
| Language | Go + Bubble Tea |
| Architecture | Layered packages + TUI |
| Scope | Focused core (not power-user suite) |
| Retention when both rules set | Never delete newest N; among older, apply max_age |
| Cron job entrypoint | `zfstui run --schedule NAME` |
| Default UI | TUI; CLI for automation only |
