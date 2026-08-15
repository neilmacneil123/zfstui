package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zfstui/zfstui/internal/app"
	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/cron"
	"github.com/zfstui/zfstui/internal/version"
	"github.com/zfstui/zfstui/internal/zfs"
)

// StartTUI is set by the ui package (Task 9) to launch the TUI.
// Signature: config path, loaded config, zfs client, dry-run flag.
var StartTUI func(cfgPath string, cfg *config.Config, z zfs.Client, dryRun bool) error

// Main parses args (without the program name) and returns a process exit code.
func Main(args []string) int {
	cfgPath, dryRun, wantHelp, rest, err := extractGlobals(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui:", err)
		return 2
	}

	if wantHelp {
		printUsage(os.Stdout)
		return 0
	}

	if len(rest) == 0 {
		return cmdDefault(cfgPath, dryRun)
	}

	cmd := rest[0]
	cmdArgs := rest[1:]

	switch cmd {
	case "help":
		printUsage(os.Stdout)
		return 0
	case "version":
		fmt.Printf("v%s (build %s)\n", version.Version, version.BuildDate)
		return 0
	case "run":
		return cmdRun(cfgPath, dryRun, cmdArgs)
	case "snapshot":
		return cmdSnapshot(cfgPath, dryRun, cmdArgs)
	case "prune":
		return cmdPrune(cfgPath, dryRun, cmdArgs)
	case "cron":
		return cmdCron(cfgPath, dryRun, cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "zfstui: unknown command %q\n\n", cmd)
		printUsage(os.Stderr)
		return 2
	}
}

// extractGlobals pulls --config / --dry-run / -h / --help from anywhere in args.
func extractGlobals(args []string) (cfgPath string, dryRun, help bool, rest []string, err error) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--config" || a == "-config":
			if i+1 >= len(args) {
				return "", false, false, nil, fmt.Errorf("--config requires a path")
			}
			cfgPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--config="):
			cfgPath = strings.TrimPrefix(a, "--config=")
		case a == "--dry-run":
			dryRun = true
		case a == "-h" || a == "--help":
			help = true
		default:
			rest = append(rest, a)
		}
	}
	return cfgPath, dryRun, help, rest, nil
}

func printUsage(w *os.File) {
	fmt.Fprint(w, `zfstui — ZFS snapshot manager

Usage:
  zfstui [global flags]                 Launch TUI (when available)
  zfstui [global flags] <command> ...

Global flags:
  --config PATH   Config file (default: /etc/zfstui/config.yaml or ~/.config/zfstui/config.yaml)
  --dry-run       Print intended actions without executing mutating commands
  -h, --help      Show this help

Commands:
  version                           Print version and build date
  run --schedule NAME               Snapshot + prune for one schedule
  snapshot --dataset D [flags]      One-shot snapshot
      --prefix P                    Snapshot prefix (default: config defaults.snapshot_prefix)
      --recursive                   Recursive snapshot (-r)
  prune --schedule NAME             Retention prune only
  cron install [--system]           Install managed crontab from config
  cron uninstall [--system]         Remove managed crontab
  cron status                       Show install state and cron preview
  help                              Show this help
`)
}

func binaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "zfstui"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

func resolveAndLoad(explicit string) (path string, cfg *config.Config, err error) {
	path, err = config.ResolvePath(explicit)
	if err != nil {
		return "", nil, err
	}
	cfg, err = config.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, nil, fmt.Errorf("config not found at %s (create it or pass --config)", path)
		}
		return path, nil, err
	}
	if err := cfg.Validate(); err != nil {
		return path, nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return path, cfg, nil
}

func cmdDefault(cfgPath string, dryRun bool) int {
	path, err := config.ResolvePath(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui:", err)
		return 1
	}
	if err := config.EnsureDefault(path); err != nil {
		fmt.Fprintln(os.Stderr, "zfstui:", err)
		return 1
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui:", err)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "zfstui: invalid config:", err)
		return 1
	}

	z := zfs.NewCLI(dryRun)
	if StartTUI != nil {
		if err := StartTUI(path, cfg, z, dryRun); err != nil {
			fmt.Fprintln(os.Stderr, "zfstui:", err)
			return 1
		}
		return 0
	}

  fmt.Fprintf(os.Stdout, "TUI is not ready yet. Available commands: version, run, snapshot, prune, cron, help\nBuild: v%s (%s)\n", version.Version, version.BuildDate)
  return 0
}

func cmdRun(cfgPath string, dryRun bool, args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	schedule := fs.String("schedule", "", "schedule name")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *schedule == "" {
		fmt.Fprintln(os.Stderr, "zfstui run: --schedule is required")
		return 2
	}

	path, cfg, err := resolveAndLoad(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui run:", err)
		return 1
	}
	_ = path

	runner := &app.Runner{Z: zfs.NewCLI(dryRun)}
	created, pruned, err := runner.RunSchedule(context.Background(), cfg, *schedule)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui run:", err)
		return 1
	}
	fmt.Printf("created: %s\n", created)
	if len(pruned) == 0 {
		fmt.Println("pruned: (none)")
	} else {
		for _, p := range pruned {
			fmt.Printf("pruned: %s\n", p)
		}
	}
	return 0
}

func cmdSnapshot(cfgPath string, dryRun bool, args []string) int {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dataset := fs.String("dataset", "", "dataset to snapshot")
	prefix := fs.String("prefix", "", "snapshot name prefix")
	recursive := fs.Bool("recursive", false, "recursive snapshot")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dataset == "" {
		fmt.Fprintln(os.Stderr, "zfstui snapshot: --dataset is required")
		return 2
	}

	_, cfg, err := resolveAndLoad(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui snapshot:", err)
		return 1
	}

	p := *prefix
	if p == "" {
		p = cfg.Defaults.SnapshotPrefix
	}

	runner := &app.Runner{Z: zfs.NewCLI(dryRun)}
	created, err := runner.Snapshot(context.Background(), *dataset, p, *recursive)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui snapshot:", err)
		return 1
	}
	fmt.Printf("created: %s\n", created)
	return 0
}

func cmdPrune(cfgPath string, dryRun bool, args []string) int {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	schedule := fs.String("schedule", "", "schedule name")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *schedule == "" {
		fmt.Fprintln(os.Stderr, "zfstui prune: --schedule is required")
		return 2
	}

	_, cfg, err := resolveAndLoad(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui prune:", err)
		return 1
	}

	runner := &app.Runner{Z: zfs.NewCLI(dryRun)}
	pruned, err := runner.PruneSchedule(context.Background(), cfg, *schedule)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui prune:", err)
		return 1
	}
	if len(pruned) == 0 {
		fmt.Println("pruned: (none)")
	} else {
		for _, p := range pruned {
			fmt.Printf("pruned: %s\n", p)
		}
	}
	return 0
}

func cmdCron(cfgPath string, dryRun bool, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "zfstui cron: subcommand required (install|uninstall|status)")
		return 2
	}
	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "install":
		return cmdCronInstall(cfgPath, dryRun, subArgs)
	case "uninstall":
		return cmdCronUninstall(dryRun, subArgs)
	case "status":
		return cmdCronStatus(cfgPath)
	default:
		fmt.Fprintf(os.Stderr, "zfstui cron: unknown subcommand %q\n", sub)
		return 2
	}
}

func parseSystemFlag(args []string) (system bool, err error) {
	fs := flag.NewFlagSet("cron", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	sys := fs.Bool("system", false, "manage /etc/cron.d/zfstui")
	if err := fs.Parse(args); err != nil {
		return false, err
	}
	return *sys, nil
}

func cmdCronInstall(cfgPath string, dryRun bool, args []string) int {
	system, err := parseSystemFlag(args)
	if err != nil {
		return 2
	}

	path, cfg, err := resolveAndLoad(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui cron install:", err)
		return 1
	}
	bin := binaryPath()

	if dryRun {
		preview, err := cron.Preview(cfg, bin, path, system)
		if err != nil {
			fmt.Fprintln(os.Stderr, "zfstui cron install:", err)
			return 1
		}
		fmt.Print(preview)
		return 0
	}

	if system {
		if err := cron.InstallSystemFile(cfg, bin, path, cron.SystemPath); err != nil {
			fmt.Fprintln(os.Stderr, "zfstui cron install:", err)
			return 1
		}
		fmt.Printf("installed system cron file: %s\n", cron.SystemPath)
		return 0
	}
	if err := cron.InstallUser(cfg, bin, path); err != nil {
		fmt.Fprintln(os.Stderr, "zfstui cron install:", err)
		return 1
	}
	fmt.Println("installed managed block in user crontab")
	return 0
}

func cmdCronUninstall(dryRun bool, args []string) int {
	system, err := parseSystemFlag(args)
	if err != nil {
		return 2
	}

	if dryRun {
		if system {
			fmt.Printf("would remove %s\n", cron.SystemPath)
		} else {
			fmt.Println("would strip managed block from user crontab")
		}
		return 0
	}

	if system {
		if err := cron.UninstallSystemFile(cron.SystemPath); err != nil {
			fmt.Fprintln(os.Stderr, "zfstui cron uninstall:", err)
			return 1
		}
		fmt.Printf("removed system cron file: %s\n", cron.SystemPath)
		return 0
	}
	if err := cron.UninstallUser(); err != nil {
		fmt.Fprintln(os.Stderr, "zfstui cron uninstall:", err)
		return 1
	}
	fmt.Println("removed managed block from user crontab")
	return 0
}

func cmdCronStatus(cfgPath string) int {
	path, cfg, err := resolveAndLoad(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui cron status:", err)
		return 1
	}
	bin := binaryPath()

	sysExists := false
	if _, err := os.Stat(cron.SystemPath); err == nil {
		sysExists = true
	}

	mode := "user"
	if sysExists {
		mode = "system"
	} else {
		mode = "none (user install available)"
	}

	fmt.Printf("config: %s\n", path)
	fmt.Printf("binary: %s\n", bin)
	fmt.Printf("system file: %s\n", cron.SystemPath)
	if sysExists {
		fmt.Println("system file exists: yes")
	} else {
		fmt.Println("system file exists: no")
	}
	fmt.Printf("mode hint: %s\n", mode)

	sysPreview, err := cron.Preview(cfg, bin, path, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui cron status: system preview:", err)
		return 1
	}
	userPreview, err := cron.Preview(cfg, bin, path, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zfstui cron status: user preview:", err)
		return 1
	}

	fmt.Println("--- system preview ---")
	fmt.Print(sysPreview)
	fmt.Println("--- user preview ---")
	fmt.Print(userPreview)
	return 0
}
