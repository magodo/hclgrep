package main

import (
	"fmt"
	"github.com/magodo/hclgrep/hclgrep"
	"os"
)

func main() {
	m := hclgrep.Matcher{
		Out: os.Stdout,
	}

	if err := m.FromArgs(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := m.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
