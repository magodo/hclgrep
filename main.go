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

	astExpr, diags := parse(toks.Bytes(), "", hcl.InitialPos, toks[0].Type == exprTokenType(hclsyntax.TokenOBrace))
	if diags.HasErrors() {
		// HCL body only allows attribute or block, but not expression. This makes the substituted "wildcard expression" failed to parse.
		// Here we further check the diags and adjust those wildcard substitutions into "wildcard attribute".
		for _, diag := range diags {
			if diag.Detail != "An argument or block definition is required here. To set an argument, use the equals sign \"=\" to introduce the argument value." {
				return false, fmt.Errorf("cannot parse expr: %v", diags.Error())
			}

			// The diag range is actually the violating identifier's range, ensure it is the "wildcard expression"
			tokMap := map[hcl.Range]fullToken{}
			for _, tok := range toks {
				tokMap[tok.Range] = tok
			}
			if tok, ok := tokMap[*diag.Subject]; !ok || tok.Type != TokenWildcard {
				return false, fmt.Errorf("cannot parse expr: %v", diags.Error())
			}

		}

		return false, fmt.Errorf("cannot parse expr: %v", diags.Error())
	}

	astSrc, diags := parse([]byte(src), "", hcl.InitialPos, toks[0].Type == exprTokenType(hclsyntax.TokenOBrace))
	if diags.HasErrors() {
		return false, fmt.Errorf("cannot parse src: %v", diags.Error())
	}
	m := matcher{values: map[string]nodeOrString{}}
	return m.node(astExpr, astSrc), nil
}
