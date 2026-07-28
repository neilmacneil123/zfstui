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
