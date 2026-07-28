package main

import (
	"fmt"
	"os"

	"github.com/zfstui/zfstui/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version.Version)
		return
	}
	fmt.Fprintln(os.Stderr, "zfstui: not implemented yet")
	os.Exit(1)
}
