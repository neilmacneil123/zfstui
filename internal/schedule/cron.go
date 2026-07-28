package schedule

import (
	"fmt"
	"strings"
)

// ValidateCron checks a standard 5-field cron expression (minute hour dom month dow).
func ValidateCron(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("cron must have 5 fields, got %d", len(fields))
	}
	for i, f := range fields {
		if f == "" {
			return fmt.Errorf("cron field %d is empty", i)
		}
	}
	return nil
}
