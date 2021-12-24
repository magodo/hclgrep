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

func (m *matcher) node(pattern, node hclsyntax.Node) bool {
	if pattern == nil || node == nil {
		return pattern == node
	}
	switch x := pattern.(type) {
	// Expressions
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
		if name == "_" {
			// values are discarded, matches anything
			return true
		}
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
	// Body
	case *hclsyntax.Body:
		y, ok := node.(*hclsyntax.Body)
		return ok && m.body(x, y)
	// Attribute(s)
	case hclsyntax.Attributes:
		y, ok := node.(hclsyntax.Attributes)
		return ok && m.attributes(x, y)
	case *hclsyntax.Attribute:
		y, ok := node.(*hclsyntax.Attribute)
		return ok && m.attribute(x, y)
	// Block(s)
	case hclsyntax.Blocks:
		y, ok := node.(hclsyntax.Blocks)
		return ok && m.blocks(x, y)
	case *hclsyntax.Block:
		y, ok := node.(*hclsyntax.Block)
		return ok && m.block(x, y)
	default:
		// Including: hclsyntax.ChildScope
		panic(fmt.Sprintf("unexpected node: %T", x))
	}
}

func (m *matcher) attributes(x, y hclsyntax.Attributes) bool {
	if len(x) != len(y) {
		return false
	}
	for k, elemx := range x {
		elemy := y[k]
		if !m.attribute(elemx, elemy) {
			return false
		}
	}
	return true
}

func (m *matcher) attribute(x, y *hclsyntax.Attribute) bool {
	if x == nil || y == nil {
		return x == y
	}
	return m.node(x.Expr, y.Expr) &&
		m.potentialWildcardIdentEqual(x.Name, y.Name)
}

func (m *matcher) blocks(x, y hclsyntax.Blocks) bool {
	if len(x) != len(y) {
		return false
	}
	for k, elemx := range x {
		elemy := y[k]
		if !m.block(elemx, elemy) {
			return false
		}
	}
	return true
}

func (m *matcher) block(x, y *hclsyntax.Block) bool {
	if x == nil || y == nil {
		return x == y
	}
	return m.potentialWildcardIdentEqual(x.Type, y.Type) &&
		m.potentialWildcardIdentsEqual(x.Labels, y.Labels) &&
		m.body(x.Body, y.Body)
}

func (m *matcher) body(x, y *hclsyntax.Body) bool {
	if x == nil || y == nil {
		return x == y
	}

	return m.attributes(x.Attributes, y.Attributes) && m.blocks(x.Blocks, y.Blocks)
}

func (m *matcher) potentialWildcardIdentsEqual(identX, identY []string) bool {
	if len(identX) != len(identY) {
		return false
	}
	for i, elemX := range identX {
		elemY := identY[i]
		if m.potentialWildcardIdentEqual(elemX, elemY) {
			return false
		}
	}
	return true
}

func (m *matcher) potentialWildcardIdentEqual(identX, identY string) bool {
	if !isWildName(identX) {
		return identX == identY
	}
	name := fromWildName(identX)
	if name == "_" {
		// values are discarded, matches anything
		return true
	}
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

func variableExpr(node hclsyntax.Node) (string, bool) {
	vexp, ok := node.(*hclsyntax.ScopeTraversalExpr)
	if !(ok && len(vexp.Traversal) == 1 && !vexp.Traversal.IsRelative()) {
		return "", false
	}
	return vexp.Traversal.RootName(), true
}
