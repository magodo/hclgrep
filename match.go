package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

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
	count  int
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
		return m.wildcardMatchNode(name, node)
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
	// Attribute
	case *hclsyntax.Attribute:
		y, ok := node.(*hclsyntax.Attribute)
		return ok && m.attribute(x, y)
	// Block
	case *hclsyntax.Block:
		y, ok := node.(*hclsyntax.Block)
		return ok && m.block(x, y)
	default:
		// Including:
		// - hclsyntax.ChildScope
		// - hclsyntax.Blocks
		// - hclsyntax.Attributes
		panic(fmt.Sprintf("unexpected node: %T", x))
	}
}

func (m *matcher) attribute(x, y *hclsyntax.Attribute) bool {
	if x == nil || y == nil {
		return x == y
	}
	if isWildAttr(x.Name, x.Expr) {
		name := fromWildAttrName(x.Name)
		return m.wildcardMatchNode(name, y)
	}
	return m.node(x.Expr, y.Expr) &&
		m.potentialWildcardIdentEqual(x.Name, y.Name)
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

	// Sort the attributes/blocks to reserve the order in source
	bodyEltsX := sortBody(x)
	bodyEltsY := sortBody(y)

	if len(bodyEltsX) != len(bodyEltsY) {
		return false
	}

	for i, rawEltX := range bodyEltsX {
		rawEltY := bodyEltsY[i]
		switch eltx := rawEltX.(type) {
		case *hclsyntax.Attribute:
			if isWildAttr(eltx.Name, eltx.Expr) {
				name := fromWildAttrName(eltx.Name)
				if !m.wildcardMatchNode(name, rawEltY.(hclsyntax.Node)) {
					return false
				}
				continue
			}
			elty, ok := rawEltY.(*hclsyntax.Attribute)
			if !ok || !m.attribute(eltx, elty) {
				return false
			}
		case *hclsyntax.Block:
			elty, ok := rawEltY.(*hclsyntax.Block)
			if !ok || !m.block(eltx, elty) {
				return false
			}
		default:
			panic("never reach here")
		}
	}
	return true
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
	return m.wildcardMatchString(name, identY)
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
		return ok && m.potentialWildcardIdentEqual(t1.Name, t2.Name)
	case hcl.TraverseAttr:
		t2, ok := t2.(hcl.TraverseAttr)
		return ok && m.potentialWildcardIdentEqual(t1.Name, t2.Name)
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

func (m *matcher) wildcardMatchNode(name string, node hclsyntax.Node) bool {
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
}

func (m *matcher) wildcardMatchString(name, target string) bool {
	if name == "_" {
		// values are discarded, matches anything
		return true
	}
	prev, ok := m.values[name]
	if !ok {
		m.values[name] = newNodeOrStringForString(target)
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
	return prevName == target
}

const (
	wildPrefix    = "hclgrep_"
	wildAttrValue = "hclgrepattr"
)

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

var wildattrMap = map[string]int{}

func wildAttr(name string) string {
	attr := wildName(name) + "-" + strconv.Itoa(wildattrMap[name]) + "=" + wildAttrValue
	wildattrMap[name] += 1
	return attr
}

func isWildAttr(key string, value hclsyntax.Expression) bool {
	v, ok := variableExpr(value)
	if !(ok && v == wildAttrValue) {
		return false
	}

	return isWildName(strings.Split(key, "-")[0])
}

func fromWildAttrName(name string) string {
	return strings.TrimPrefix(strings.Split(name, "-")[0], wildPrefix)
}

func variableExpr(node hclsyntax.Node) (string, bool) {
	vexp, ok := node.(*hclsyntax.ScopeTraversalExpr)
	if !(ok && len(vexp.Traversal) == 1 && !vexp.Traversal.IsRelative()) {
		return "", false
	}
	return vexp.Traversal.RootName(), true
}

func sortBody(body *hclsyntax.Body) []interface{} {
	l := len(body.Blocks) + len(body.Attributes)
	m := make(map[int]interface{}, l)
	offsets := make([]int, 0, l)
	for _, blk := range body.Blocks {
		offset := blk.Range().Start.Byte
		m[offset] = blk
		offsets = append(offsets, offset)
	}
	for _, attr := range body.Attributes {
		offset := attr.Range().Start.Byte
		m[offset] = attr
		offsets = append(offsets, offset)
	}
	sort.Ints(offsets)
	out := make([]interface{}, 0, l)
	for _, offset := range offsets {
		out = append(out, m[offset])
	}
	return out
}
