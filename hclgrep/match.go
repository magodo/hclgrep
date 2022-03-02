package hclgrep

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type Matcher struct {
	Out io.Writer

	parents map[hclsyntax.Node]hclsyntax.Node
	b       []byte

	// whether prefix the matches with filenname and byte offset
	prefix bool

	// node values recorded by name, excluding "_" (used only by the
	// actual matching phase)
	values map[string]substitution

	// only set for unit test
	test bool
}

// File matches one File against one or more cmds, output the final matches to matcher's out.
func (m *Matcher) File(cmds []Cmd, fileName string, in io.Reader) error {
	m.parents = make(map[hclsyntax.Node]hclsyntax.Node)
	var err error
	m.b, err = io.ReadAll(in)
	if err != nil {
		return err
	}
	f, diags := hclsyntax.ParseConfig(m.b, fileName, hcl.InitialPos)
	if diags.HasErrors() {
		return fmt.Errorf("cannot parse source: %s", diags.Error())
	}
	matches := m.matches(cmds, f.Body.(*hclsyntax.Body))
	wd, _ := os.Getwd()

	if cmds[len(cmds)-1].name == CmdNameWrite {
		return nil
	}

	for _, n := range matches {
		rng := n.Range()
		output := string(rng.SliceBytes(m.b))
		if m.prefix {
			if strings.HasPrefix(rng.Filename, wd) {
				rng.Filename = rng.Filename[len(wd)+1:]
			}
			output = fmt.Sprintf("%s:\n%s", rng, output)
		}

		fmt.Fprintf(m.Out, "%s\n", output)
	}
	return nil
}

// matches matches one node against one or more cmds.
func (m *Matcher) matches(cmds []Cmd, node hclsyntax.Node) []hclsyntax.Node {
	m.fillParents(node)
	initial := []submatch{{node: node, values: map[string]substitution{}}}
	final := m.submatches(cmds, initial)
	matches := make([]hclsyntax.Node, len(final))
	for i := range matches {
		matches[i] = final[i].node
	}
	return matches
}

type parentsWalker struct {
	stack   []hclsyntax.Node
	parents map[hclsyntax.Node]hclsyntax.Node
}

func (w *parentsWalker) Enter(node hclsyntax.Node) hcl.Diagnostics {
	switch node.(type) {
	case hclsyntax.Attributes,
		hclsyntax.Blocks,
		hclsyntax.ChildScope:
		return nil
	}
	w.parents[node] = w.stack[len(w.stack)-1]
	w.stack = append(w.stack, node)
	return nil
}

func (w *parentsWalker) Exit(node hclsyntax.Node) hcl.Diagnostics {
	switch node.(type) {
	case hclsyntax.Attributes,
		hclsyntax.Blocks,
		hclsyntax.ChildScope:
		return nil
	}
	w.stack = w.stack[:len(w.stack)-1]
	return nil
}

func (m *Matcher) fillParents(nodes ...hclsyntax.Node) {
	walker := &parentsWalker{
		parents: map[hclsyntax.Node]hclsyntax.Node{},
		stack:   make([]hclsyntax.Node, 1, 32),
	}
	for _, node := range nodes {
		hclsyntax.Walk(node, walker)
	}
	m.parents = walker.parents
}

type submatch struct {
	node   hclsyntax.Node
	values map[string]substitution
}

func (m *Matcher) submatches(cmds []Cmd, subs []submatch) []submatch {
	if len(cmds) == 0 {
		return subs
	}
	var fn func(Cmd, []submatch) []submatch
	cmd := cmds[0]
	switch cmd.name {
	case CmdNameMatch:
		fn = m.cmdMatch
	case CmdNameFilterMatch:
		fn = m.cmdFilter(true)
	case CmdNameFilterUnMatch:
		fn = m.cmdFilter(false)
	case CmdNameParent:
		fn = m.cmdParent
	case CmdNameRx:
		fn = m.cmdRx
	case CmdNameWrite:
		fn = m.cmdWrite
	default:
		panic(fmt.Sprintf("unknown command: %q", cmd.name))
	}
	return m.submatches(cmds[1:], fn(cmd, subs))
}

func (m *Matcher) cmdMatch(cmd Cmd, subs []submatch) []submatch {
	var matches []submatch
	for _, sub := range subs {
		hclsyntax.VisitAll(sub.node, func(node hclsyntax.Node) hcl.Diagnostics {
			m.values = valsCopy(sub.values)
			if m.node(cmd.value.Value().(hclsyntax.Node), node) {
				matches = append(matches, submatch{
					node:   node,
					values: m.values,
				})
			}
			return nil
		})
	}
	return matches
}

func (m *Matcher) cmdFilter(wantMatch bool) func(Cmd, []submatch) []submatch {
	return func(cmd Cmd, subs []submatch) []submatch {
		var matches []submatch
		var any bool
		for _, sub := range subs {
			any = false
			hclsyntax.VisitAll(sub.node, func(node hclsyntax.Node) hcl.Diagnostics {
				// return early if already match, so that the values are kept to be the state of the first match (DFS)
				if any {
					return nil
				}
				m.values = valsCopy(sub.values)
				if m.node(cmd.value.Value().(hclsyntax.Node), node) {
					any = true
				}
				return nil
			})
			if any == wantMatch {
				// update the values of submatch for '-g'
				if wantMatch {
					sub.values = m.values
				}
				matches = append(matches, sub)
			}
		}
		return matches
	}
}

func (m *Matcher) cmdParent(cmd Cmd, subs []submatch) []submatch {
	var newsubs []submatch
	for _, sub := range subs {
		reps := int(cmd.value.Value().(CmdValueLevel))
		for j := 0; j < reps; j++ {
			sub.node = m.parentOf(sub.node)
		}
		if sub.node != nil {
			newsubs = append(newsubs, sub)
		}
	}
	return newsubs
}

func (m *Matcher) cmdRx(cmd Cmd, subs []submatch) []submatch {
	var newsubs []submatch
	for _, sub := range subs {
		rx := cmd.value.Value().(CmdValueRx)
		val, ok := sub.values[rx.name]
		if !ok {
			continue
		}
		var valLit string
		switch {
		case val.String != nil:
			valLit = *val.String
		case val.Node != nil:
			var ok bool
			// check whether the node is a variable
			valLit, ok = variableExpr(val.Node)
			if !ok {
				switch node := val.Node.(type) {
				case *hclsyntax.TemplateExpr:
					if len(node.Parts) != 1 {
						continue
					}
					tmpl := node.Parts[0]
					lve, ok := tmpl.(*hclsyntax.LiteralValueExpr)
					if !ok {
						continue
					}
					value, _ := lve.Value(nil)
					valLit = value.AsString()
				case *hclsyntax.LiteralValueExpr:
					value, _ := node.Value(nil)
					switch value.Type() {
					case cty.String:
						valLit = value.AsString()
					case cty.Bool:
						valLit = "true"
						if value.False() {
							valLit = "false"
						}
					case cty.Number:
						// TODO: handle float?
						valLit = value.AsBigFloat().String()
					}
				}
			}
		case val.ObjectConsItem != nil:
		case val.Traverser != nil:
			switch trav := (*val.Traverser).(type) {
			case hcl.TraverseRoot:
				valLit = trav.Name
			case hcl.TraverseAttr:
				valLit = trav.Name
			default:
				continue
			}
		default:
			panic("never reach here")
		}

		if rx.rx.MatchString(valLit) {
			newsubs = append(newsubs, sub)
		}
	}
	return newsubs
}

func (m *Matcher) cmdWrite(cmd Cmd, subs []submatch) []submatch {
	for _, sub := range subs {
		name := string(cmd.value.Value().(CmdValueString))
		val, ok := sub.values[name]
		if !ok {
			continue
		}
		switch {
		case val.String != nil:
			fmt.Fprintln(m.Out, *val.String)
		case val.Node != nil:
			fmt.Fprintln(m.Out, string(val.Node.Range().SliceBytes(m.b)))
		case val.ObjectConsItem != nil:
		case val.Traverser != nil:
			switch trav := (*val.Traverser).(type) {
			case hcl.TraverseRoot:
				fmt.Fprintln(m.Out, trav.Name)
			case hcl.TraverseAttr:
				fmt.Fprintln(m.Out, trav.Name)
			default:
				continue
			}
		default:
			panic("never reach here")
		}
	}

	return subs
}

func (m *Matcher) parentOf(node hclsyntax.Node) hclsyntax.Node {
	return m.parents[node]
}

func valsCopy(values map[string]substitution) map[string]substitution {
	v2 := make(map[string]substitution, len(values))
	for k, v := range values {
		v2[k] = v
	}
	return v2
}

type substitution struct {
	String         *string
	Node           hclsyntax.Node
	ObjectConsItem *hclsyntax.ObjectConsItem
	Traverser      *hcl.Traverser
}

func newStringSubstitution(s string) substitution {
	return substitution{String: &s}
}

func newNodeSubstitution(node hclsyntax.Node) substitution {
	return substitution{Node: node}
}

func newObjectConsItemSubstitution(item *hclsyntax.ObjectConsItem) substitution {
	return substitution{ObjectConsItem: item}
}

func newTraverserSubstitution(trav hcl.Traverser) substitution {
	return substitution{Traverser: &trav}
}

func (m *Matcher) node(pattern, node hclsyntax.Node) bool {
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
		// In case the index key of x is a wildcard, try to also match "y" even if it is not an IndexExpr
		xname, ok := variableExpr(x.Key)
		if ok && isWildName(xname) {
			switch y := node.(type) {
			case *hclsyntax.ScopeTraversalExpr:
				l := len(y.Traversal)
				ySourceTraversal := &hclsyntax.ScopeTraversalExpr{
					Traversal: make(hcl.Traversal, l-1),
				}
				copy(ySourceTraversal.Traversal, y.Traversal[:l-1])
				return m.node(x.Collection, ySourceTraversal) && m.wildcardMatchTraverse(xname, y.Traversal[l-1])
			case *hclsyntax.IndexExpr:
				return m.node(x.Collection, y.Collection) && m.wildcardMatchNode(xname, y.Key)
			case *hclsyntax.RelativeTraversalExpr:
				return m.node(x.Collection, y.Source) && len(y.Traversal) == 1 && m.wildcardMatchTraverse(xname, y.Traversal[0])
			default:
				return false
			}
		}

		// Otherwise, regular match against the same type
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
		if ok && isWildName(xname) {
			name, _ := fromWildName(xname)
			return m.wildcardMatchNode(name, node)
		}
		y, ok := node.(*hclsyntax.ScopeTraversalExpr)
		return ok && m.traversal(x.Traversal, y.Traversal)
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
		return m.attribute(x, node)
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

type matchFunc func(*Matcher, interface{}, interface{}) bool
type wildNameFunc func(interface{}) (string, bool)

type iterable interface {
	at(i int) interface{}
	len() int
}

type stringIterable []string

func (it stringIterable) at(i int) interface{} {
	return it[i]
}
func (it stringIterable) len() int {
	return len(it)
}

type nodeIterable []hclsyntax.Node

func (it nodeIterable) at(i int) interface{} {
	return it[i]
}

func (it nodeIterable) len() int {
	return len(it)
}

type exprIterable []hclsyntax.Expression

func (it exprIterable) at(i int) interface{} {
	return it[i]
}

func (it exprIterable) len() int {
	return len(it)
}

type objectConsItemIterable []hclsyntax.ObjectConsItem

func (it objectConsItemIterable) at(i int) interface{} {
	return it[i]
}

func (it objectConsItemIterable) len() int {
	return len(it)
}

// iterableMatches matches two lists. It uses a common algorithm to match
// wildcard patterns with any number of elements without recursion.
func (m *Matcher) iterableMatches(ns1, ns2 iterable, nf wildNameFunc, mf matchFunc) bool {
	i1, i2 := 0, 0
	next1, next2 := 0, 0

	// We need to keep a copy of m.values so that we can restart
	// with a different "any of" match while discarding any matches
	// we found while trying it.
	var oldMatches map[string]substitution
	backupMatches := func() {
		oldMatches = make(map[string]substitution, len(m.values))
		for k, v := range m.values {
			oldMatches[k] = v
		}
	}
	backupMatches()

	for i1 < ns1.len() || i2 < ns2.len() {
		if i1 < ns1.len() {
			n1 := ns1.at(i1)
			if _, any := nf(n1); any {
				// try to match zero or more at i2,
				// restarting at i2+1 if it fails
				next1 = i1
				next2 = i2 + 1
				i1++
				backupMatches()
				continue
			}
			if i2 < ns2.len() && mf(m, n1, ns2.at(i2)) {
				// ordinary match
				i1++
				i2++
				continue
			}
		}
		// mismatch, try to restart
		if 0 < next2 && next2 <= ns2.len() {
			i1 = next1
			i2 = next2
			m.values = oldMatches
			continue
		}
		return false
	}
	return true
}

// Node comparisons

func wildNameFromNode(in interface{}) (string, bool) {
	switch node := in.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		name, ok := variableExpr(node)
		if !ok {
			return "", false
		}
		return fromWildName(name)
	case *hclsyntax.Attribute:
		return fromWildName(node.Name)
	default:
		return "", false
	}
}

func matchNode(m *Matcher, x, y interface{}) bool {
	nx, ny := x.(hclsyntax.Node), y.(hclsyntax.Node)
	return m.node(nx, ny)
}

func (m *Matcher) attribute(x *hclsyntax.Attribute, y hclsyntax.Node) bool {
	if x == nil || y == nil {
		return x == y
	}
	if isWildAttr(x.Name, x.Expr) {
		// The wildcard attribute can only match attribute or block
		switch y := y.(type) {
		case *hclsyntax.Attribute,
			*hclsyntax.Block:
			name, _ := fromWildName(x.Name)
			return m.wildcardMatchNode(name, y)
		default:
			return false
		}
	}
	attrY, ok := y.(*hclsyntax.Attribute)
	return ok && m.node(x.Expr, attrY.Expr) &&
		m.potentialWildcardIdentEqual(x.Name, attrY.Name)
}

func (m *Matcher) block(x, y *hclsyntax.Block) bool {
	if x == nil || y == nil {
		return x == y
	}
	return m.potentialWildcardIdentEqual(x.Type, y.Type) &&
		m.potentialWildcardIdentsEqual(x.Labels, y.Labels) &&
		m.body(x.Body, y.Body)
}

func (m *Matcher) body(x, y *hclsyntax.Body) bool {
	if x == nil || y == nil {
		return x == y
	}

	// Sort the attributes/blocks to reserve the order in source
	bodyEltsX := sortBody(x)
	bodyEltsY := sortBody(y)
	return m.iterableMatches(nodeIterable(bodyEltsX), nodeIterable(bodyEltsY), wildNameFromNode, matchNode)
}

func (m *Matcher) exprs(exprs1, exprs2 []hclsyntax.Expression) bool {
	return m.iterableMatches(exprIterable(exprs1), exprIterable(exprs2), wildNameFromNode, matchNode)
}

// Operation comparisons

func (m *Matcher) operation(op1, op2 *hclsyntax.Operation) bool {
	if op1 == nil || op2 == nil {
		return op1 == op2
	}
	return op1.Impl == op2.Impl && op1.Type.Equals(op2.Type)
}

// ObjectConsItems comparisons

func wildNameFromObjectConsItem(in interface{}) (string, bool) {
	if node, ok := in.(hclsyntax.ObjectConsItem).KeyExpr.(*hclsyntax.ObjectConsKeyExpr); ok {
		name, ok := variableExpr(node.Wrapped)
		if !ok {
			return "", false
		}
		return fromWildName(name)
	}
	return "", false
}

func matchObjectConsItem(m *Matcher, x, y interface{}) bool {
	itemX, itemY := x.(hclsyntax.ObjectConsItem), y.(hclsyntax.ObjectConsItem)
	return m.objectConsItem(itemX, itemY)
}

func (m *Matcher) objectConsItem(item1, item2 hclsyntax.ObjectConsItem) bool {
	if key1, ok := item1.KeyExpr.(*hclsyntax.ObjectConsKeyExpr); ok {
		name, ok := variableExpr(key1.Wrapped)
		if ok && isWildAttr(name, item1.ValueExpr) {
			return m.wildcardMatchObjectConsItem(name, item2)
		}
	}
	return m.node(item1.KeyExpr, item2.KeyExpr) && m.node(item1.ValueExpr, item2.ValueExpr)
}

func (m *Matcher) objectConsItems(items1, items2 []hclsyntax.ObjectConsItem) bool {
	return m.iterableMatches(objectConsItemIterable(items1), objectConsItemIterable(items2), wildNameFromObjectConsItem, matchObjectConsItem)
}

// String comparisons

func wildNameFromString(in interface{}) (string, bool) {
	return fromWildName(in.(string))
}

func matchString(m *Matcher, x, y interface{}) bool {
	sx, sy := x.(string), y.(string)
	return m.potentialWildcardIdentEqual(sx, sy)
}

func (m *Matcher) potentialWildcardIdentEqual(identX, identY string) bool {
	if !isWildName(identX) {
		return identX == identY
	}
	name, _ := fromWildName(identX)
	return m.wildcardMatchString(name, identY)
}

func (m *Matcher) potentialWildcardIdentsEqual(identX, identY []string) bool {
	return m.iterableMatches(stringIterable(identX), stringIterable(identY), wildNameFromString, matchString)
}

// Traversal comparisons

func (m *Matcher) traversal(traversal1, traversal2 hcl.Traversal) bool {
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

func (m *Matcher) traverser(t1, t2 hcl.Traverser) bool {
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

// Wildcard matchers

func (m *Matcher) wildcardMatchNode(name string, node hclsyntax.Node) bool {
	// Wildcard never matches multiple attributes/blocks.
	// On one hand, it is because we have any wildcard, which already meets this requirement.
	// One the other hand, Go panics to use the attributes/blocks slice as map key.
	switch node.(type) {
	case hclsyntax.Attributes,
		hclsyntax.Blocks:
		return false
	}

	if name == "_" {
		// values are discarded, matches anything
		return true
	}
	prev, ok := m.values[name]
	if !ok {
		m.values[name] = newNodeSubstitution(node)
		return true
	}
	switch {
	case prev.String != nil:
		nodeVar, ok := variableExpr(node)
		return ok && nodeVar == *prev.String
	case prev.Node != nil:
		return m.node(prev.Node, node)
	case prev.ObjectConsItem != nil:
		return false
	default:
		panic("never reach here")
	}
}

func (m *Matcher) wildcardMatchString(name, target string) bool {
	if name == "_" {
		// values are discarded, matches anything
		return true
	}
	prev, ok := m.values[name]
	if !ok {
		m.values[name] = newStringSubstitution(target)
		return true
	}

	switch {
	case prev.String != nil:
		return *prev.String == target
	case prev.Node != nil:
		prevName, ok := variableExpr(prev.Node)
		return ok && prevName == target
	case prev.ObjectConsItem != nil:
		return false
	case prev.Traverser != nil:
		switch trav := (*prev.Traverser).(type) {
		case hcl.TraverseRoot:
			return trav.Name == target
		case hcl.TraverseAttr:
			return trav.Name == target
		default:
			return false
		}
	default:
		panic("never reach here")
	}
}

func (m *Matcher) wildcardMatchObjectConsItem(name string, item hclsyntax.ObjectConsItem) bool {
	if name == "_" {
		// values are discarded, matches anything
		return true
	}
	prev, ok := m.values[name]
	if !ok {
		m.values[name] = newObjectConsItemSubstitution(&item)
		return true
	}
	switch {
	case prev.String != nil:
		return false
	case prev.Node != nil:
		return false
	case prev.ObjectConsItem != nil:
		return m.objectConsItem(*prev.ObjectConsItem, item)
	case prev.Traverser != nil:
		return false
	default:
		panic("never reach here")
	}
}

func (m *Matcher) wildcardMatchTraverse(name string, trav hcl.Traverser) bool {
	if name == "_" {
		// values are discarded, matches anything
		return true
	}
	prev, ok := m.values[name]
	if !ok {
		m.values[name] = newTraverserSubstitution(trav)
		return true
	}
	switch {
	case prev.String != nil:
		switch trav := trav.(type) {
		case hcl.TraverseRoot:
			return trav.Name == *prev.String
		case hcl.TraverseAttr:
			return trav.Name == *prev.String
		default:
			return false
		}
	case prev.Node != nil:
		return false
	case prev.ObjectConsItem != nil:
		return false
	case prev.Traverser != nil:
		return m.traverser(trav, *prev.Traverser)
	default:
		panic("never reach here")
	}
}

// Two wildcard: expression wildcard ($) and attribute wildcard (@)
// - expression wildcard: $<ident> => hclgrep_<ident>
// - expression wildcard (any): $<ident> => hclgrep_any_<ident>
// - attribute wildcard : @<ident> => hclgrep-<index>_<ident> = hclgrepattr
// - attribute wildcard (any) : @<ident> => hclgrep_any-<index>_<ident> = hclgrepattr
const (
	wildPrefix    = "hclgrep_"
	wildExtraAny  = "any_"
	wildAttrValue = "hclgrepattr"
)

var wildattrCounters = map[string]int{}

func wildName(name string, any bool) string {
	prefix := wildPrefix
	if any {
		prefix += wildExtraAny
	}
	return prefix + name
}

func wildAttr(name string, any bool) string {
	attr := wildName(name, any) + "-" + strconv.Itoa(wildattrCounters[name]) + "=" + wildAttrValue
	wildattrCounters[name] += 1
	return attr
}

func isWildName(name string) bool {
	return strings.HasPrefix(name, wildPrefix)
}

func isWildAttr(key string, value hclsyntax.Expression) bool {
	v, ok := variableExpr(value)
	return ok && v == wildAttrValue && isWildName(key)
}

func fromWildName(name string) (ident string, any bool) {
	ident = strings.TrimPrefix(strings.Split(name, "-")[0], wildPrefix)
	return strings.TrimPrefix(ident, wildExtraAny), strings.HasPrefix(ident, wildExtraAny)
}

func variableExpr(node hclsyntax.Node) (string, bool) {
	vexp, ok := node.(*hclsyntax.ScopeTraversalExpr)
	if !(ok && len(vexp.Traversal) == 1 && !vexp.Traversal.IsRelative()) {
		return "", false
	}
	return vexp.Traversal.RootName(), true
}

func sortBody(body *hclsyntax.Body) []hclsyntax.Node {
	l := len(body.Blocks) + len(body.Attributes)
	m := make(map[int]hclsyntax.Node, l)
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
	out := make([]hclsyntax.Node, 0, l)
	for _, offset := range offsets {
		out = append(out, m[offset])
	}
	return out
}
