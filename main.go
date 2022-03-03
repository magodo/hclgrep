package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/magodo/hclgrep/hclgrep"
	"os"
)

func main() {
	opts, files, err := hclgrep.ParseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	m := hclgrep.NewMatcher(opts...)
	if err := m.Files(files); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
