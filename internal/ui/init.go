package ui

import "github.com/zfstui/zfstui/internal/cli"

func init() {
	cli.StartTUI = Run
}
