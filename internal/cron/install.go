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
