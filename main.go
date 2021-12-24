package main

import (
	"bytes"
	"errors"
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

func grep(expr string, src string) (bool, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return false, err
	}
	var buf bytes.Buffer
	for _, t := range toks {
		var s string
		switch {
		case t.Type == TokenWildcard:
			s = wildName(string(t.Bytes))
		default:
			s = string(t.Bytes)
		}
		buf.WriteString(s)
		buf.WriteByte(' ') // for e.g. consecutive idents (e.g. ForExpr)
	}
	astExpr, diags := hclsyntax.ParseExpression(buf.Bytes(), "", hcl.InitialPos)
	if diags.HasErrors() {
		return false, errors.New(diags.Error())
	}
	astSrc, diags := hclsyntax.ParseExpression([]byte(src), "", hcl.InitialPos)
	if diags.HasErrors() {
		return false, errors.New(diags.Error())
	}
	m := matcher{values: map[string]nodeOrString{}}
	return m.node(astExpr, astSrc), nil
}
