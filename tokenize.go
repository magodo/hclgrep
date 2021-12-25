package main

import (
	"bytes"
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
	TokenWildcard
	TokenAttrWildcard
)

type fullToken struct {
	Type  exprTokenType
	Bytes []byte
	Range hcl.Range
}

type fullTokens []fullToken

const (
	wildcardLit     = "$"
	attrWildcardLit = "@"
)

// tokenize create fullTokens by substituting the wildcard token in the source.
// Also it removes any leading newline.
func tokenize(src string) (fullTokens, error) {
	tokens, _diags := hclsyntax.LexExpression([]byte(src), "", hcl.InitialPos)

	var diags hcl.Diagnostics
	for _, diag := range _diags {
		if tok := string(diag.Subject.SliceBytes([]byte(src))); diag.Summary == "Invalid character" && tok == wildcardLit || tok == attrWildcardLit {
			continue
		}
		diags = diags.Append(diag)
	}
	if diags.HasErrors() {
		return nil, errors.New(diags.Error())
	}

	var (
		toks              []fullToken
		wildcardTokenType = hclsyntax.TokenNil
	)
	var start int
	for start = 0; start < len(tokens) && tokens[start].Type == hclsyntax.TokenNewline; start++ {
	}
	for _, tok := range tokens[start:] {
		if wildcardTokenType != hclsyntax.TokenNil {
			if tok.Type != hclsyntax.TokenIdent {
				return nil, fmt.Errorf("%v: %s must be followed by ident, got %v",
					tok.Range, wildcardLit, tok.Type)
			}
			toks = append(toks, fullToken{
				Type:  exprTokenType(wildcardTokenType),
				Range: tok.Range,
				Bytes: tok.Bytes,
			})

			wildcardTokenType = hclsyntax.TokenNil
			continue
		}
		if tok.Type == hclsyntax.TokenEOF {
			break
		}
		if tok.Type == hclsyntax.TokenInvalid {
			switch string(tok.Bytes) {
			case wildcardLit:
				wildcardTokenType = hclsyntax.TokenType(TokenWildcard)
			case attrWildcardLit:
				wildcardTokenType = hclsyntax.TokenType(TokenAttrWildcard)
			default:
				panic(fmt.Sprintf("unexpected invalid token %s", string(tok.Bytes)))
			}
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

func (toks fullTokens) Bytes() []byte {
	var buf bytes.Buffer
	for i, t := range toks {
		var s string
		switch {
		case t.Type == TokenWildcard:
			s = wildName(string(t.Bytes))
		case t.Type == TokenAttrWildcard:
			s = wildAttr(string(t.Bytes))
		default:
			s = string(t.Bytes)
		}
		buf.WriteString(s)

		if i+1 < len(toks) {
			peekTok := toks[i+1]
			if peekTok.Type == exprTokenType(hclsyntax.TokenIdent) || peekTok.Type == TokenWildcard || peekTok.Type == TokenAttrWildcard {
				buf.WriteByte(' ') // for e.g. consecutive idents (e.g. ForExpr)
			}
		}
	}
	return buf.Bytes()
}
