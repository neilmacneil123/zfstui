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
	InSync    bool
	Err       error
}

// Inspect compares the installed managed cron content with the same renderer
// used by installation. Read errors are returned in Status so callers can
// display partial dashboard data without failing the whole load.
func Inspect(cfg *config.Config, binary, configPath string, system bool) Status {
	if system {
		return InspectSystem(cfg, binary, configPath, SystemPath)
	}
	all, err := ReadUserCrontab()
	if err != nil {
		return Status{Mode: "user", Err: err}
	}
	return InspectUser(cfg, binary, configPath, all)
}

// InspectSystem is the path-injectable form used by tests and alternate
// installations.
func InspectSystem(cfg *config.Config, binary, configPath, path string) Status {
	st := Status{Mode: "system", Path: path}
	expected, err := RenderSystem(cfg, binary, configPath)
	if err != nil {
		st.Err = err
		return st
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st
	}
	if err != nil {
		st.Err = err
		return st
	}
	installed := string(b)
	st.Installed = installed != ""
	st.InSync = st.Installed && installed == expected
	return st
}

// InspectUser compares an already-read crontab, allowing callers to use a
// different crontab transport without duplicating managed-block parsing.
func InspectUser(cfg *config.Config, binary, configPath, all string) Status {
	st := Status{Mode: "user"}
	expected, err := RenderUserBlock(cfg, binary, configPath)
	if err != nil {
		st.Err = err
		return st
	}
	installed := userManagedBlock(all)
	st.Installed = installed != ""
	st.InSync = st.Installed && installed == expected
	return st
}

func userManagedBlock(s string) string {
	start := strings.Index(s, BeginMarker)
	if start < 0 {
		return ""
	}
	end := strings.Index(s[start:], EndMarker)
	if end < 0 {
		return ""
	}
	end = start + end + len(EndMarker)
	if end < len(s) && s[end] == '\n' {
		end++
	}
	return s[start:end]
}

func Preview(cfg *config.Config, binary, configPath string, system bool) (string, error) {
	if system {
		return RenderSystem(cfg, binary, configPath)
	}
	return RenderUserBlock(cfg, binary, configPath)
}
