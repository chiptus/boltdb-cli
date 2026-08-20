package main

import (
	"fmt"
	"os"

	"github.com/chiptus/boltdb-cli/cmd"
)

func main() {
	if err := cmd.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
