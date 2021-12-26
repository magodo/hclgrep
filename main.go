package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) != 2 {
		log.Fatal("needs two args")
	}
	match, err := grep(args[0], args[1])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(match)
}

func grep(expr string, src string) (int, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return 0, fmt.Errorf("cannot tokenize expr: %v", err)
	}

	p := toks.Bytes()
	exprNode, diags := parse(p, "", hcl.InitialPos, toks[0].Type == exprTokenType(hclsyntax.TokenOBrace))
	if diags.HasErrors() {
		return 0, fmt.Errorf("cannot parse expr: %v", diags.Error())
	}

	srcNode, diags := parse([]byte(src), "", hcl.InitialPos, toks[0].Type == exprTokenType(hclsyntax.TokenOBrace))
	if diags.HasErrors() {
		return 0, fmt.Errorf("cannot parse src: %v", diags.Error())
	}
	m := matcher{values: map[string]nodeOrString{}, count: 0}
	match := func(srcNode hclsyntax.Node) {
		if m.node(exprNode, srcNode) {
			m.count++
		}
	}
	hclsyntax.VisitAll(srcNode, func(node hclsyntax.Node) hcl.Diagnostics {
		match(node)
		return nil
	})
	return m.count, nil
}
