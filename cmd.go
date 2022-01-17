package main

import (
	"flag"
	"fmt"
	"strconv"
)

type cmd struct {
	name  string
	src   string
	value interface{}
}

type strCmdFlag struct {
	name string
	cmds *[]cmd
}

func (o *strCmdFlag) String() string { return "" }
func (o *strCmdFlag) Set(val string) error {
	*o.cmds = append(*o.cmds, cmd{name: o.name, src: val})
	return nil
}

func parseCmds(args []string) ([]cmd, []string, error) {
	flagSet := flag.NewFlagSet("hclgrep", flag.ExitOnError)
	flagSet.Usage = usage

	var cmds []cmd
	flagSet.Var(&strCmdFlag{
		name: "x",
		cmds: &cmds,
	}, "x", "")
	flagSet.Var(&strCmdFlag{
		name: "p",
		cmds: &cmds,
	}, "p", "")

	flagSet.Parse(args)
	files := flagSet.Args()

	if len(cmds) < 1 {
		return nil, nil, fmt.Errorf("need at least one command")
	}

	for i, cmd := range cmds {
		switch cmd.name {
		case "x":
			node, err := compileExpr(cmd.src)
			if err != nil {
				return nil, nil, fmt.Errorf("compiling expression %q: %w", node, err)
			}
			cmds[i].value = node
		case "p":
			n, err := strconv.Atoi(cmd.src)
			if err != nil {
				return nil, nil, err
			}
			cmds[i].value = n
		}
	}
	return cmds, files, nil
}
