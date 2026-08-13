# zfstui

Pretty terminal UI for managing **ZFS snapshots** on a schedule.

Browse pools and datasets, create and destroy snapshots, define GFS-retention schedules, install managed crontab entries, and restore via **rollback** or **clone** (optional promote). Config YAML is the source of truth; crontab is derived.

## Install

**Requirements:** Go 1.22+, Linux host with `zfs` available (or dry-run for exploration).

```bash
# Build
make build
# → bin/zfstui

# Or:
go build -o bin/zfstui ./cmd/zfstui

# Install system-wide (optional)
sudo make install
# → /usr/local/bin/zfstui
```

Tests (no real ZFS required — fakes):

```bash
make test
# or: go test ./...
```

## Quick start (TUI)

```bash
# First launch creates a default config if missing
zfstui

# Explicit config / dry-run (no mutating zfs/crontab writes)
zfstui --config ~/.config/zfstui/config.yaml --dry-run
```

**TUI navigation**

| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Move |
| `enter` | Open / select |
| `esc` | Back |
| `D` | Toggle session dry-run |
| `?` | Help overlay |
| `q` | Quit |

**Snapshots screen:** `c` create · `d` destroy (typed confirm) · `r` restore wizard  

**Restore wizard (`r`):** choose **Rollback** (lists newer snaps; type dataset name) or **Clone** (new path → promote y/n → confirm). Dry-run is honored via the ZFS client.

**Schedules:** `a` add · `e` edit · `x` delete · `space` toggle enabled. In the editor, press `enter` on **dataset** to choose from live ZFS datasets, or on **cron** to build an hourly, daily, weekly, or monthly schedule. Raw dataset paths and advanced cron expressions remain editable.

**Crontab:** `i` install · `u` uninstall · `r` refresh preview  

## Config example

Default paths (first existing / writable):

- System: `/etc/zfstui/config.yaml`
- User: `~/.config/zfstui/config.yaml`

```yaml
version: 1
defaults:
  snapshot_prefix: zfstui
  recursive: false
ui:
  theme: default
schedules:
  - name: hourly-home
    dataset: tank/home
    recursive: false
    cron: "0 * * * *"
    prefix: zfstui
    enabled: true
    retention:
      daily: 24
      weekly: 8
      monthly: 12
      yearly: 5
  - name: nightly-data
    dataset: tank/data
    recursive: true
    cron: "30 2 * * *"
    prefix: zfstui
    enabled: true
    retention:
      daily: 7
      weekly: 4
      monthly: 12
      yearly: 3
```

**Retention**

| Setting | Behavior |
|---------|----------|
| `daily: N` | Keep one snapshot per UTC day for the newest N days with snapshots |
| `weekly: N` | Keep one snapshot per ISO week for the newest N weeks with snapshots |
| `monthly: N` | Keep one snapshot per UTC month for the newest N months with snapshots |
| `yearly: N` | Keep one snapshot per UTC year for the newest N years with snapshots |
| All zero | No prune |

**Recursive schedules:** when `recursive: true`, create uses `zfs snapshot -r`, but prune only destroys matching snapshots on the **schedule dataset itself** (not child datasets). Manage child retention with separate schedules if needed.

## Cron install

Schedules are installed as **managed** crontab content (marker block or `/etc/cron.d/zfstui`).

```bash
# Preview / install user crontab block
zfstui cron status
zfstui cron install
zfstui cron uninstall

# System file (typically root → /etc/cron.d/zfstui)
sudo zfstui cron install --system
sudo zfstui cron uninstall --system

# Dry-run prints preview without writing
zfstui --dry-run cron install
```

Each enabled schedule becomes a line that runs:

```text
zfstui --config <path> run --schedule <name>
```

## CLI reference

```text
zfstui [global flags]                 Launch TUI
zfstui [global flags] <command> ...

Global flags:
  --config PATH   Config file path
  --dry-run       Simulate mutating zfs/crontab actions
  -h, --help      Help

Commands:
  version                           Print version
  run --schedule NAME               Snapshot + prune for one schedule
  snapshot --dataset D [flags]      One-shot snapshot
      --prefix P                    Name prefix (default: config)
      --recursive                   Recursive snapshot (-r)
  prune --schedule NAME             Retention prune only
  cron install [--system]           Install managed crontab
  cron uninstall [--system]         Remove managed crontab
  cron status                       Install state + previews
  help                              This help
```

Examples:

```bash
zfstui run --schedule hourly-home
zfstui snapshot --dataset tank/home --prefix zfstui
zfstui prune --schedule hourly-home
zfstui --dry-run run --schedule hourly-home
```

## Safety notes

- **Dry-run first.** Use `--dry-run` or `D` in the TUI before live rollback/destroy/cron install.
- **Rollback is destructive.** Newer snapshots on that dataset (and their deltas) are destroyed. The wizard lists them and requires typing the **dataset name**. When newer snapshots exist, rollback uses `zfs rollback -r` automatically.
- **Destroy** requires typing the short snapshot name (after `@`).
- **Clone** is safer for exploration (leave promote **No**). **Promote** reverses the relationship: the clone becomes the origin, the original becomes a dependent clone, and older snapshots move to the clone. It is not a rollback of live data.
- **Privileges:** many `zfs` operations need root or delegated permissions; cron system install needs write access to `/etc/cron.d`.
- **No daemon / no send-receive** in v1 — scheduling is plain cron invoking the CLI.
- Unit tests use a fake ZFS client; they never touch real pools.

## License

See repository for license terms.
