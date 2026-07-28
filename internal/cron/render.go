package cron

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zfstui/zfstui/internal/config"
	"github.com/zfstui/zfstui/internal/schedule"
)

const (
	BeginMarker  = "# BEGIN zfstui managed"
	EndMarker    = "# END zfstui managed"
	SystemHeader = "# Managed by zfstui — do not edit by hand"
	SystemPath   = "/etc/cron.d/zfstui"
)

func RenderSystem(cfg *config.Config, binary, configPath string) (string, error) {
	var b strings.Builder
	b.WriteString(SystemHeader + "\n")
	b.WriteString("SHELL=/bin/sh\n")
	b.WriteString("PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin\n\n")
	qBin := strconv.Quote(binary)
	qCfg := strconv.Quote(configPath)
	for _, s := range cfg.Schedules {
		if !s.Enabled {
			continue
		}
		if err := schedule.ValidateCron(s.Cron); err != nil {
			return "", fmt.Errorf("schedule %q: %w", s.Name, err)
		}
		fmt.Fprintf(&b, "%s root %s run --config %s --schedule %s\n", s.Cron, qBin, qCfg, s.Name)
	}
	return b.String(), nil
}

func RenderUserBlock(cfg *config.Config, binary, configPath string) (string, error) {
	var b strings.Builder
	b.WriteString(BeginMarker + "\n")
	qBin := strconv.Quote(binary)
	qCfg := strconv.Quote(configPath)
	for _, s := range cfg.Schedules {
		if !s.Enabled {
			continue
		}
		if err := schedule.ValidateCron(s.Cron); err != nil {
			return "", fmt.Errorf("schedule %q: %w", s.Name, err)
		}
		// user crontab: no user field
		fmt.Fprintf(&b, "%s %s run --config %s --schedule %s\n", s.Cron, qBin, qCfg, s.Name)
	}
	b.WriteString(EndMarker + "\n")
	return b.String(), nil
}

func MergeUserCrontab(existing, block string) string {
	stripped := StripUserBlock(existing)
	if stripped != "" && !strings.HasSuffix(stripped, "\n") {
		stripped += "\n"
	}
	return stripped + block
}

func StripUserBlock(existing string) string {
	start := strings.Index(existing, BeginMarker)
	if start < 0 {
		return existing
	}
	end := strings.Index(existing[start:], EndMarker)
	if end < 0 {
		return existing[:start]
	}
	end = start + end + len(EndMarker)
	// drop trailing newline after end marker
	rest := existing[end:]
	rest = strings.TrimPrefix(rest, "\n")
	return existing[:start] + rest
}
