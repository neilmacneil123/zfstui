package schedule_test

import (
	"testing"

	"github.com/zfstui/zfstui/internal/schedule"
)

func TestValidateCronOK(t *testing.T) {
	for _, expr := range []string{"0 * * * *", "30 2 * * *", "*/15 * * * *"} {
		if err := schedule.ValidateCron(expr); err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
	}
}

func TestValidateCronBad(t *testing.T) {
	for _, expr := range []string{"", "0 * * *", "not-a-cron", "0 0 0 0 0 0 0"} {
		if err := schedule.ValidateCron(expr); err == nil {
			t.Fatalf("expected error for %q", expr)
		}
	}
}
