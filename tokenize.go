package main

import (
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// exprTokenType exists to add extra possible tokens on top of the ones
// recognized by vanilla HCL2.
type exprTokenType hclsyntax.TokenType

const (
	_ exprTokenType = -iota
	tokWildcard
)

type fullToken struct {
	Type  exprTokenType
	Bytes []byte
	Range hcl.Range
}

const (
	wildcardLit       = "&"
	wildcardTokenType = hclsyntax.TokenBitwiseAnd
)

func tokenize(src string) ([]fullToken, error) {
	tokens, _diags := hclsyntax.LexExpression([]byte(src), "", hcl.InitialPos)

	var diags hcl.Diagnostics
	for _, diag := range _diags {
		if diag.Detail == "Bitwise operators are not supported. Did you mean boolean AND (\"&&\")?" {
			continue
		}
		diags = diags.Append(diag)
	}
	if diags.HasErrors() {
		return nil, errors.New(diags.Error())
	}

	var (
		toks        []fullToken
		gotWildcard bool
	)
	for _, tok := range tokens {
		if tok.Type == hclsyntax.TokenEOF {
			break
		}
		if gotWildcard {
			if tok.Type != hclsyntax.TokenIdent {
				return nil, fmt.Errorf("%v: %s must be followed by ident, got %v",
					tok.Range, wildcardLit, tok.Type)
			}
			gotWildcard = false
			toks = append(toks, fullToken{
				Type:  tokWildcard,
				Range: tok.Range,
				Bytes: tok.Bytes,
			})
		} else if tok.Type == wildcardTokenType {
			gotWildcard = true
		} else {
			toks = append(toks, fullToken{
				Type:  exprTokenType(tok.Type),
				Range: tok.Range,
				Bytes: tok.Bytes,
			})
		}
	}
	return toks, nil
}
