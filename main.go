package main

import (
	"bytes"
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
	var buf bytes.Buffer
	for i, t := range toks {
		var s string
		switch {
		case t.Type == TokenWildcard:
			s = wildName(string(t.Bytes))
		default:
			s = string(t.Bytes)
		}
		buf.WriteString(s)

		if i+1 < len(toks) {
			peekTok := toks[i+1]
			if peekTok.Type == exprTokenType(hclsyntax.TokenIdent) || peekTok.Type == TokenWildcard {
				buf.WriteByte(' ') // for e.g. consecutive idents (e.g. ForExpr)
			}
		}
	}
	astExpr, diags := parse(buf.Bytes(), "", hcl.InitialPos)
	if diags.HasErrors() {
		return false, fmt.Errorf("cannot parse expr: %v", diags.Error())
	}
	astSrc, diags := parse([]byte(src), "", hcl.InitialPos)
	if diags.HasErrors() {
		return false, fmt.Errorf("cannot parse src: %v", diags.Error())
	}
	m := matcher{values: map[string]nodeOrString{}}
	return m.node(astExpr, astSrc), nil
}
