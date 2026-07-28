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
