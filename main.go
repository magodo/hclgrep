package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "need at least two args")
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

		matches := search(exprNode, srcNode)
		wd, _ := os.Getwd()
		for _, n := range matches {
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

func search(exprNode, node hclsyntax.Node) []hclsyntax.Node {
	matches := []hclsyntax.Node{}
	match := func(node hclsyntax.Node) {
		m := matcher{values: map[string]nodeOrString{}}
		if m.node(exprNode, node) {
			matches = append(matches, node)
		}
	}
	hclsyntax.VisitAll(node, func(node hclsyntax.Node) hcl.Diagnostics {
		match(node)
		return nil
	})
	return matches
}
