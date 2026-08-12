package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zfstui/zfstui/internal/schedule"
)

// scheduleNameRe restricts names so they are safe in cron command lines.
var scheduleNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Retention struct {
	Daily   int `yaml:"daily,omitempty"`
	Weekly  int `yaml:"weekly,omitempty"`
	Monthly int `yaml:"monthly,omitempty"`
	Yearly  int `yaml:"yearly,omitempty"`
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
		if !scheduleNameRe.MatchString(s.Name) {
			return fmt.Errorf("schedule %q: name must match [A-Za-z0-9._-]+", s.Name)
		}
		if strings.TrimSpace(s.Dataset) == "" {
			return fmt.Errorf("schedule %q: dataset is required", s.Name)
		}
		if strings.TrimSpace(s.Cron) == "" {
			return fmt.Errorf("schedule %q: cron is required", s.Name)
		}
		if err := schedule.ValidateCron(s.Cron); err != nil {
			return fmt.Errorf("schedule %q: %w", s.Name, err)
		}
		if strings.TrimSpace(s.Prefix) == "" {
			return fmt.Errorf("schedule %q: prefix is required", s.Name)
		}
		if s.Retention.Daily < 0 {
			return fmt.Errorf("schedule %q: retention.daily must be >= 0", s.Name)
		}
		if s.Retention.Weekly < 0 {
			return fmt.Errorf("schedule %q: retention.weekly must be >= 0", s.Name)
		}
		if s.Retention.Monthly < 0 {
			return fmt.Errorf("schedule %q: retention.monthly must be >= 0", s.Name)
		}
		if s.Retention.Yearly < 0 {
			return fmt.Errorf("schedule %q: retention.yearly must be >= 0", s.Name)
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
