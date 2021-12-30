package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

var usage = func() {
	fmt.Fprint(os.Stderr, `usage: hclgrep pattern [files]

hclgrep performs a query on the given HCL(v2) files.

A pattern is a piece of HCL code which may include wildcards. It can be:

- A body (zero or more attributes, and zero or more blocks)
- An expression

There are two types of wildcards, depending on the scope it resides in:

- Attribute wildcard ("@"): represents an attribute, a block or an object element
- Expression wildcard ("$"): represents an expression or a place that a string is accepted (i.e. as a block type, block label)

The wildcards are followed by a name. Each wildcard with the same name must match the same node/string, excluding "_". Example:

    $x.$_ = $x # assignment of self to a field in self

If "*" is before the name, it will match any number of nodes. Example:

    [$*_] # any number of elements in a tuple

    resource foo "name" {
        @*_  # any number of attributes/blocks inside the resource block body
    }
`)
}

func main() {
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "hclgrep: need at least two args, try 'hclgrep -h' for more information")
		os.Exit(1)
	}
	if err := grepArgs(args[0], args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func grepArgs(expr string, files []string) error {
	exprNode, err := compileExpr(expr)
	if err != nil {
		return err
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", file, err)
		}
		f, diags := hclsyntax.ParseConfig(b, file, hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf("cannot parse source: %s", diags.Error())
		}
		srcNode := bodyContent(f.Body.(*hclsyntax.Body))

		nodes := matches(exprNode, srcNode)
		wd, _ := os.Getwd()
		for _, n := range nodes {
			rng := n.Range()
			if strings.HasPrefix(rng.Filename, wd) {
				rng.Filename = rng.Filename[len(wd)+1:]
			}
			fmt.Printf("%s:\n%s\n", rng, string(rng.SliceBytes(b)))
		}
	}
	return nil
}

func compileExpr(expr string) (hclsyntax.Node, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return nil, fmt.Errorf("cannot tokenize expr: %v", err)
	}

	p := toks.Bytes()
	node, diags := parse(p, "", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("cannot parse expr: %v", diags.Error())
	}
	return node, nil
}
