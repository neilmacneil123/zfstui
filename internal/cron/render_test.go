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

func TestRenderSystemInvalidCron(t *testing.T) {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{
		{Name: "bad", Dataset: "tank/a", Cron: "not-a-cron", Prefix: "p", Enabled: true},
	}
	_, err := cron.RenderSystem(&cfg, "/usr/local/bin/zfstui", "/etc/zfstui/config.yaml")
	if err == nil {
		t.Fatal("expected error for invalid cron")
	}
}

func TestRenderUserBlockEnabledOnly(t *testing.T) {
	cfg := config.Default()
	cfg.Schedules = []config.Schedule{
		{Name: "on", Dataset: "tank/a", Cron: "0 * * * *", Prefix: "p", Enabled: true},
		{Name: "off", Dataset: "tank/b", Cron: "0 1 * * *", Prefix: "p", Enabled: false},
	}
	out, err := cron.RenderUserBlock(&cfg, "/usr/local/bin/zfstui", "/etc/zfstui/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, cron.BeginMarker+"\n") {
		t.Fatal("missing begin marker")
	}
	if !strings.Contains(out, cron.EndMarker) {
		t.Fatal("missing end marker")
	}
	if !strings.Contains(out, "0 * * * * /usr/local/bin/zfstui run --config /etc/zfstui/config.yaml --schedule on") {
		t.Fatal(out)
	}
	if strings.Contains(out, " root ") {
		t.Fatal("user crontab must not include user field")
	}
	if strings.Contains(out, "schedule off") {
		t.Fatal("disabled should be omitted")
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
