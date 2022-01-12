package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

var usage = func() {
	fmt.Fprint(os.Stderr, `usage: hclgrep -x PATTERN ... [FILE...]

hclgrep performs a query on the given HCL(v2) files.

A pattern is a piece of HCL code which may include wildcards. It can be:

- A body (zero or more attributes, and zero or more blocks)
- An expression

There are two types of wildcards can be used in a pattern, depending on the scope it resides in:

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

type patternFlag []string

func (o *patternFlag) String() string { return "" }
func (o *patternFlag) Set(val string) error {
	*o = append(*o, val)
	return nil
}

func main() {
	flag.Usage = usage

	var patterns patternFlag
	flag.Var(&patterns, "x", "")
	flag.Parse()
	if len(patterns) < 1 {
		fmt.Fprintln(os.Stderr, "hclgrep: need at least one pattern, try 'hclgrep -h' for more information")
		os.Exit(1)
	}

	files := flag.Args()
	if err := grep(patterns, files); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func grep(exprs []string, files []string) error {
	var exprNodes []hclsyntax.Node
	for _, expr := range exprs {
		exprNode, err := compileExpr(expr)
		if err != nil {
			return fmt.Errorf("compiling expression %q: %w", expr, err)
		}
		exprNodes = append(exprNodes, exprNode)
	}

	if len(files) == 0 {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading from stdin: %w", err)
		}
		return grepOneSource(exprNodes, "stdin", b)
	}

	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", file, err)
		}
		if err := grepOneSource(exprNodes, file, b); err != nil {
			return err
		}
	}
	return nil
}

type orderedNodes []hclsyntax.Node

func (nodes orderedNodes) Less(i, j int) bool {
	return nodes[i].Range().Start.Byte < nodes[j].Range().Start.Byte
}
func (nodes orderedNodes) Swap(i, j int) {
	nodes[i], nodes[j] = nodes[j], nodes[i]
}
func (nodes orderedNodes) Len() int {
	return len(nodes)
}

func grepOneSource(exprNodes []hclsyntax.Node, fileName string, b []byte) error {
	f, diags := hclsyntax.ParseConfig(b, fileName, hcl.InitialPos)
	if diags.HasErrors() {
		return fmt.Errorf("cannot parse source: %s", diags.Error())
	}
	srcNode := f.Body.(*hclsyntax.Body)

	nodes := orderedNodes{srcNode}
	for _, exprNode := range exprNodes {
		wl := make([]hclsyntax.Node, len(nodes))
		copy(wl, nodes)
		nodeMap := map[hclsyntax.Node]bool{}
		for _, node := range wl {
			matchedNodes := matches(exprNode, node)
			for _, mn := range matchedNodes {
				nodeMap[mn] = true
			}
		}
		nodes = make(orderedNodes, 0, len(nodeMap))
		for node := range nodeMap {
			nodes = append(nodes, node)
		}
		sort.Sort(nodes)
	}

	wd, _ := os.Getwd()
	for _, n := range nodes {
		rng := n.Range()
		if strings.HasPrefix(rng.Filename, wd) {
			rng.Filename = rng.Filename[len(wd)+1:]
		}
		fmt.Printf("%s:\n%s\n", rng, string(rng.SliceBytes(b)))
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
