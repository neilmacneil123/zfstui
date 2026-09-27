package cron_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/cron"
)

func TestInspectSystemStates(t *testing.T) {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{{Name: "daily", Dataset: "tank/a", Cron: "0 0 * * *", Enabled: true, Prefix: "z"}}
	path := filepath.Join(t.TempDir(), "zfstui")
	st := cron.InspectSystem(&cfg, "/bin/zfstui", "/cfg", path)
	if st.Installed || st.Err != nil {
		t.Fatalf("absent: %#v", st)
	}
	want, err := cron.RenderSystem(&cfg, "/bin/zfstui", "/cfg")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(want), 0644); err != nil {
		t.Fatal(err)
	}
	st = cron.InspectSystem(&cfg, "/bin/zfstui", "/cfg", path)
	if !st.Installed || !st.InSync || st.Err != nil {
		t.Fatalf("sync: %#v", st)
	}
	if err := os.WriteFile(path, []byte(want+"# stale\n"), 0644); err != nil {
		t.Fatal(err)
	}
	st = cron.InspectSystem(&cfg, "/bin/zfstui", "/cfg", path)
	if !st.Installed || st.InSync {
		t.Fatalf("stale: %#v", st)
	}
}

func TestInspectUserManagedBlock(t *testing.T) {
	cfg := config.Default()
	block, err := cron.RenderUserBlock(&cfg, "zfstui", "/cfg")
	if err != nil {
		t.Fatal(err)
	}
	st := cron.InspectUser(&cfg, "zfstui", "/cfg", "MAILTO=x\n"+block)
	if !st.Installed || !st.InSync {
		t.Fatalf("status %#v", st)
	}
	st = cron.InspectUser(&cfg, "zfstui", "/cfg", "")
	if st.Installed {
		t.Fatalf("status %#v", st)
	}
}

func TestWriteFileAtomicAndInstallSystemFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "cron.d", "zfstui")

	cfg := config.Default()
	cfg.Schedules = []config.Schedule{
		{Name: "hourly", Dataset: "tank/home", Cron: "0 * * * *", Prefix: "p", Enabled: true},
	}
	if err := cron.InstallSystemFile(&cfg, "/usr/local/bin/zfstui", "/etc/zfstui/config.yaml", dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, cron.SystemHeader) {
		t.Fatal(body)
	}
	if !strings.Contains(body, "schedule hourly") {
		t.Fatal(body)
	}
	if _, err := os.Stat(dest + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp file should not remain after atomic write")
	}
}

func TestUninstallSystemFile(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "zfstui")
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cron.UninstallSystemFile(dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("file should be removed")
	}
	// missing is ok
	if err := cron.UninstallSystemFile(dest); err != nil {
		t.Fatal(err)
	}
}

func TestPreview(t *testing.T) {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{
		{Name: "on", Dataset: "tank/a", Cron: "0 * * * *", Prefix: "p", Enabled: true},
	}
	sys, err := cron.Preview(&cfg, "/bin/zfstui", "/cfg.yaml", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sys, "root ") {
		t.Fatal(sys)
	}
	user, err := cron.Preview(&cfg, "/bin/zfstui", "/cfg.yaml", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(user, cron.BeginMarker) {
		t.Fatal(user)
	}
}
