package main

import (
	"flag"
	"fmt"
	"github.com/magodo/hclgrep/hclgrep"
	"os"
)

func main() {
	opts, files, err := hclgrep.ParseArgs(os.Args[1:], flag.ExitOnError)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	m := hclgrep.NewMatcher(opts...)
	if err := m.Files(files); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
