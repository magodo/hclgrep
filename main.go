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

	cmds, files, err := m.ParseCmds(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if len(files) == 0 {
		if err := m.File(cmds, "stdin", os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	for _, file := range files {
		in, err := os.Open(file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		err = m.File(cmds, file, in)
		in.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
