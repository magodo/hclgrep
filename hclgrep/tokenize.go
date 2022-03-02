package hclgrep

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
	TokenWildcardAny
	TokenAttrWildcard
	TokenAttrWildcardAny
)

type fullToken struct {
	Type  hclsyntax.TokenType
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
		if tok := string(diag.Subject.SliceBytes([]byte(src))); diag.Summary == "Invalid character" && (tok == wildcardLit || tok == attrWildcardLit) {
			continue
		}
		diags = diags.Append(diag)
	}
	if diags.HasErrors() {
		return nil, errors.New(diags.Error())
	}

	var start int
	for start = 0; start < len(tokens) && tokens[start].Type == hclsyntax.TokenNewline; start++ {
	}

	var remaining []fullToken
	for _, tok := range tokens[start:] {
		remaining = append(remaining, fullToken{tok.Type, tok.Bytes, tok.Range})
		if tok.Type == hclsyntax.TokenEOF {
			break
		}
	}
	next := func() fullToken {
		t := remaining[0]
		remaining = remaining[1:]
		return t
	}

	var (
		toks              []fullToken
		wildcardTokenType = hclsyntax.TokenNil
	)
	t := next()
	for {
		if t.Type == hclsyntax.TokenEOF {
			break
		}
		if !(t.Type == hclsyntax.TokenInvalid &&
			(string(t.Bytes) == wildcardLit || string(t.Bytes) == attrWildcardLit)) {
			// regular HCL
			toks = append(toks, fullToken{
				Type:  t.Type,
				Range: t.Range,
				Bytes: t.Bytes,
			})
			t = next()
			continue
		}
		switch string(t.Bytes) {
		case wildcardLit:
			wildcardTokenType = hclsyntax.TokenType(TokenWildcard)
		case attrWildcardLit:
			wildcardTokenType = hclsyntax.TokenType(TokenAttrWildcard)
		default:
			panic("never reach here")
		}
		t = next()
		if string(t.Bytes) == string(hclsyntax.TokenStar) {
			switch wildcardTokenType {
			case hclsyntax.TokenType(TokenWildcard):
				wildcardTokenType = hclsyntax.TokenType(TokenWildcardAny)
			case hclsyntax.TokenType(TokenAttrWildcard):
				wildcardTokenType = hclsyntax.TokenType(TokenAttrWildcardAny)
			}
			t = next()
		}
		if t.Type != hclsyntax.TokenIdent {
			return nil, fmt.Errorf("%v: wildcard must be followed by ident, got %v",
				t.Range, t.Type)
		}
		toks = append(toks, fullToken{
			Type:  wildcardTokenType,
			Bytes: t.Bytes,
			Range: t.Range,
		})
		t = next()
	}

	return toks, nil
}

func (toks fullTokens) Bytes() []byte {
	var buf bytes.Buffer
	for i, t := range toks {
		var s string
		switch {
		case t.Type == hclsyntax.TokenType(TokenWildcard):
			s = wildName(string(t.Bytes), false)
		case t.Type == hclsyntax.TokenType(TokenWildcardAny):
			s = wildName(string(t.Bytes), true)
		case t.Type == hclsyntax.TokenType(TokenAttrWildcard):
			s = wildAttr(string(t.Bytes), false)
		case t.Type == hclsyntax.TokenType(TokenAttrWildcardAny):
			s = wildAttr(string(t.Bytes), true)
		default:
			s = string(t.Bytes)
		}
		buf.WriteString(s)

		if i+1 < len(toks) {
			peekTok := toks[i+1]
			if peekTok.Type == hclsyntax.TokenIdent ||
				peekTok.Type == hclsyntax.TokenType(TokenWildcard) ||
				peekTok.Type == hclsyntax.TokenType(TokenAttrWildcard) ||
				peekTok.Type == hclsyntax.TokenType(TokenWildcardAny) ||
				peekTok.Type == hclsyntax.TokenType(TokenAttrWildcardAny) {
				buf.WriteByte(' ') // for e.g. consecutive idents (e.g. ForExpr)
			}
		}
	}
	return buf.Bytes()
}
