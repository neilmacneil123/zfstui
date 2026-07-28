package main

import (
	"os"

	"github.com/zfstui/zfstui/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
