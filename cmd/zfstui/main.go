package main

import (
	"os"

	"github.com/zfstui/zfstui/internal/cli"
	_ "github.com/zfstui/zfstui/internal/ui" // registers cli.StartTUI
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
