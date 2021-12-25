package main

import (
	"flag"
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"log"
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

func grep(expr string, src string) (bool, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return false, fmt.Errorf("cannot tokenize expr: %v", err)
	}

	p := toks.Bytes()
	astExpr, diags := parse(p, "", hcl.InitialPos, toks[0].Type == exprTokenType(hclsyntax.TokenOBrace))
	if diags.HasErrors() {
		return false, fmt.Errorf("cannot parse expr: %v", diags.Error())
	}

	astSrc, diags := parse([]byte(src), "", hcl.InitialPos, toks[0].Type == exprTokenType(hclsyntax.TokenOBrace))
	if diags.HasErrors() {
		return false, fmt.Errorf("cannot parse src: %v", diags.Error())
	}
	m := matcher{values: map[string]nodeOrString{}}
	return m.node(astExpr, astSrc), nil
}
