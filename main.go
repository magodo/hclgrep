package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"

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
		case t.Type == tokWildcard:
			s = wildName(string(t.Bytes))
		case len(t.Bytes) != 0:
			s = string(t.Bytes)
		default:
			// TODO: whether this is needed?
			buf.WriteRune(rune(t.Type))
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

type nodeOrString struct {
	String *string
	Node   hclsyntax.Node
}

func newNodeOrStringForString(s string) nodeOrString {
	return nodeOrString{String: &s}
}

func newNodeOrStringForNode(node hclsyntax.Node) nodeOrString {
	return nodeOrString{Node: node}
}

type matcher struct {
	values map[string]nodeOrString
}

func (m *matcher) node(expr, node hclsyntax.Node) bool {
	if expr == nil || node == nil {
		return expr == node
	}
	switch x := expr.(type) {
	case *hclsyntax.LiteralValueExpr:
		y, ok := node.(*hclsyntax.LiteralValueExpr)
		return ok && x.Val.Equals(y.Val).True()
	case *hclsyntax.TupleConsExpr:
		y, ok := node.(*hclsyntax.TupleConsExpr)
		return ok && m.exprs(x.Exprs, y.Exprs)
	case *hclsyntax.ObjectConsExpr:
		y, ok := node.(*hclsyntax.ObjectConsExpr)
		return ok && m.objectConsItems(x.Items, y.Items)
	case *hclsyntax.TemplateExpr:
		y, ok := node.(*hclsyntax.TemplateExpr)
		return ok && m.exprs(x.Parts, y.Parts)
	case *hclsyntax.FunctionCallExpr:
		y, ok := node.(*hclsyntax.FunctionCallExpr)
		return ok &&
			m.potentialWildcardIdentEqual(x.Name, y.Name) &&
			m.exprs(x.Args, y.Args) && x.ExpandFinal == y.ExpandFinal
	case *hclsyntax.ForExpr:
		y, ok := node.(*hclsyntax.ForExpr)
		return ok &&
			m.potentialWildcardIdentEqual(x.KeyVar, y.KeyVar) &&
			m.potentialWildcardIdentEqual(x.ValVar, y.ValVar) &&
			m.node(x.CollExpr, y.CollExpr) && m.node(x.KeyExpr, y.KeyExpr) && m.node(x.ValExpr, y.ValExpr) && m.node(x.CondExpr, y.CondExpr) && x.Group == y.Group
	case *hclsyntax.IndexExpr:
		y, ok := node.(*hclsyntax.IndexExpr)
		return ok && m.node(x.Collection, y.Collection) && m.node(x.Key, y.Key)
	case *hclsyntax.SplatExpr:
		y, ok := node.(*hclsyntax.SplatExpr)
		return ok && m.node(x.Source, y.Source) && m.node(x.Each, y.Each) && m.node(x.Item, y.Item)
	case *hclsyntax.ParenthesesExpr:
		y, ok := node.(*hclsyntax.ParenthesesExpr)
		return ok && m.node(x.Expression, y.Expression)
	case *hclsyntax.UnaryOpExpr:
		y, ok := node.(*hclsyntax.UnaryOpExpr)
		return ok && m.operation(x.Op, y.Op) && m.node(x.Val, y.Val)
	case *hclsyntax.BinaryOpExpr:
		y, ok := node.(*hclsyntax.BinaryOpExpr)
		return ok && m.operation(x.Op, y.Op) && m.node(x.LHS, y.LHS) && m.node(x.RHS, y.RHS)
	case *hclsyntax.ConditionalExpr:
		y, ok := node.(*hclsyntax.ConditionalExpr)
		return ok && m.node(x.Condition, y.Condition) && m.node(x.TrueResult, y.TrueResult) && m.node(x.FalseResult, y.FalseResult)
	case *hclsyntax.ScopeTraversalExpr:
		xname, ok := variableExpr(x)
		if !ok || !isWildName(xname) {
			y, ok := node.(*hclsyntax.ScopeTraversalExpr)
			return ok && m.traversal(x.Traversal, y.Traversal)
		}
		name := fromWildName(xname)
		prev, ok := m.values[name]
		if !ok {
			m.values[name] = newNodeOrStringForNode(node)
			return true
		}
		if prev.String == nil {
			return m.node(prev.Node, node)
		}
		nodeVar, ok := variableExpr(node)
		return ok && nodeVar == *prev.String
	case *hclsyntax.RelativeTraversalExpr:
		y, ok := node.(*hclsyntax.RelativeTraversalExpr)
		return ok && m.traversal(x.Traversal, y.Traversal) && m.node(x.Source, y.Source)
	case *hclsyntax.ObjectConsKeyExpr:
		y, ok := node.(*hclsyntax.ObjectConsKeyExpr)
		return ok && m.node(x.Wrapped, y.Wrapped) && x.ForceNonLiteral == y.ForceNonLiteral
	case *hclsyntax.TemplateJoinExpr:
		y, ok := node.(*hclsyntax.TemplateJoinExpr)
		return ok && m.node(x.Tuple, y.Tuple)
	case *hclsyntax.TemplateWrapExpr:
		y, ok := node.(*hclsyntax.TemplateWrapExpr)
		return ok && m.node(x.Wrapped, y.Wrapped)
	case *hclsyntax.AnonSymbolExpr:
		_, ok := node.(*hclsyntax.AnonSymbolExpr)
		// Only do type check
		return ok
	default:
		panic(fmt.Sprintf("unexpected node: %T", x))
	}
}

func (m *matcher) potentialWildcardIdentEqual(identX, identY string) bool {
	if !isWildName(identX) {
		return identX == identY
	}
	name := fromWildName(identX)
	prev, ok := m.values[name]
	if !ok {
		m.values[name] = newNodeOrStringForString(name)
		return true
	}

	var prevName string
	if prev.String != nil {
		prevName = *prev.String
	} else {
		prevName, ok = variableExpr(prev.Node)
		if !ok {
			return false
		}
	}
	return prevName == identY
}

func (m *matcher) exprs(exprs1, exprs2 []hclsyntax.Expression) bool {
	if len(exprs1) != len(exprs2) {
		return false
	}
	for i, e1 := range exprs1 {
		if !m.node(e1, exprs2[i]) {
			return false
		}
	}
	return true
}

func (m *matcher) objectConsItems(items1, items2 []hclsyntax.ObjectConsItem) bool {
	if len(items1) != len(items2) {
		return false
	}
	for i, e1 := range items1 {
		if !(m.node(e1.KeyExpr, items2[i].KeyExpr) && m.node(e1.ValueExpr, items2[i].ValueExpr)) {
			return false
		}
	}
	return true
}

func (m *matcher) operation(op1, op2 *hclsyntax.Operation) bool {
	if op1 == nil || op2 == nil {
		return op1 == op2
	}
	return op1.Impl == op2.Impl && op1.Type.Equals(op2.Type)
}

func (m *matcher) traversal(traversal1, traversal2 hcl.Traversal) bool {
	if len(traversal1) != len(traversal2) {
		return false
	}
	for i, t1 := range traversal1 {
		if !m.traverser(t1, traversal2[i]) {
			return false
		}
	}
	return true
}

func (m *matcher) traverser(t1, t2 hcl.Traverser) bool {
	switch t1 := t1.(type) {
	case hcl.TraverseRoot:
		t2, ok := t2.(hcl.TraverseRoot)
		return ok && t1.Name == t2.Name
	case hcl.TraverseAttr:
		t2, ok := t2.(hcl.TraverseAttr)
		return ok && t1.Name == t2.Name
	case hcl.TraverseIndex:
		t2, ok := t2.(hcl.TraverseIndex)
		return ok && t1.Key.Equals(t2.Key).True()
	case hcl.TraverseSplat:
		t2, ok := t2.(hcl.TraverseSplat)
		return ok && m.traversal(t1.Each, t2.Each)
	default:
		panic(fmt.Sprintf("unexpected node: %T", t1))
	}
}

const wildPrefix = "hclgrep_"

func wildName(name string) string {
	// good enough for now
	return wildPrefix + name
}

func isWildName(name string) bool {
	return strings.HasPrefix(name, wildPrefix)
}

func fromWildName(name string) string {
	return strings.TrimPrefix(name, wildPrefix)
}

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

func variableExpr(node hclsyntax.Node) (string, bool) {
	vexp, ok := node.(*hclsyntax.ScopeTraversalExpr)
	if !(ok && len(vexp.Traversal) == 1 && !vexp.Traversal.IsRelative()) {
		return "", false
	}
	return vexp.Traversal.RootName(), true
}
