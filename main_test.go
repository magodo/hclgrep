package main

import (
	"fmt"
	"testing"
)

type wantErr string

func tokErr(msg string) wantErr {
	return wantErr("cannot tokenize expr: " + msg)
}

func parseErr(msg string) wantErr {
	return wantErr("cannot parse expr: " + msg)
}

type matches uint

var noMatch = matches(0)

func TestGrep(t *testing.T) {
	tests := []struct {
		expr, src string
		anyWant   interface{}
	}{
		{"{for $k, $v in map: $k => upper($v)}", "{for k, v in map: k => upper(v)}", matches(1)},
		{
			expr: `
@_

blk {
 @_
}
`,
			src: `
a = b

blk {
 blk1 {}
}
`,
			anyWant: matches(1),
		},
		{"$x = $x", "a = a", matches(1)},

		// literal expression
		{"1", "1", matches(1)},
		{"true", "false", noMatch},

		// literal expression (wildcard)
		{"$_", "1", matches(1)},
		{"$_", "false", matches(1)},

		// tuple cons expression
		{"[1, 2]", "[1, 3]", noMatch},
		{"[1, 2]", "[1, 2]", matches(1)},

		// tuple cons expression (wildcard)
		{"$_", "[1, 2, 3]", matches(1)},
		{"[1, $_, 3]", "[1, 2, 3]", matches(1)},
		{"[1, $_, 3]", "[1, 3]", noMatch},
		{"[1, $x, $x]", "[1, 2, 2]", matches(1)},
		{"[1, $x, $x]", "[1, 2, 3]", noMatch},

		// object const expression
		{"{a = b}", "{a = b}", matches(1)},
		{"{a = c}", "{a = b}", noMatch},
		{
			expr: `
		{
			a = b
			c = d
		}`,
			src: `
		{
			a = b
			c = d
		}`,
			anyWant: matches(1),
		},

		// object const expression (wildcard)
		{"$_", "{a = b}", matches(1)},
		{"{$x = $x}", "{a = a}", matches(1)},
		{"{$x = $x}", "{a = b}", noMatch},
		{
			expr: `
		{
			a = $x
			c = $x
		}`,
			src: `
		{
			a = b
			c = b
		}`,
			anyWant: matches(1),
		},
		{
			expr: `
		{
			a = $x
			c = $x
		}`,
			src: `
		{
			a = b
			c = d
		}`,
			anyWant: noMatch,
		},

		// template expression
		{`"a"`, `"a"`, matches(1)},
		{`"a"`, `"b"`, noMatch},
		{
			expr: `<<EOF
content
EOF
`,
			src: `<<EOF
content
EOF
`,
			anyWant: matches(1),
		},
		{
			expr: `<<EOF
content
EOF
`,
			src: `<<EOF
other content
EOF
`,
			anyWant: noMatch,
		},

		// template expression (wildcard)
		{`$_`, `"a"`, matches(1)},
		{
			expr: `$_`,
			src: `<<EOF
content
EOF
`,
			anyWant: matches(1),
		},

		// function call expression
		{"f1()", "f1()", matches(1)},
		{"f1()", "f2()", noMatch},
		{"f1()", "f1(arg)", noMatch},

		// function call expression (wildcard)
		{"$_", "f1()", matches(1)},
		{"$_()", "f1()", matches(1)},
		{"$_()", "f1(arg)", noMatch},
		{"f1($_)", "f1(arg)", matches(1)},
		{"$_($_)", "f1(arg)", matches(1)},
		{"f1($x, $x)", "f1(arg, arg)", matches(1)},
		{"f1($x, $x)", "f1(arg, arg2)", noMatch},

		// for expression
		{"[for i in list: i]", "[for i in list: i]", matches(1)},
		{"[for i in list: i]", "[for i in list: upper(i)]", noMatch},
		{"{for k, v in map: k => v}", "{for k, v in map: k => upper(v)}", noMatch},
		{"{for k, v in map: k => upper(v)}", "{for k, v in map: k => upper(v)}", matches(1)},

		// for expression (wildcard)
		{"$_", "{for k, v in map: k => upper(v)}", matches(1)},
		{"{for k, v in map: $k => upper($v)}", "{for k, v in map: k => upper(v)}", matches(1)},
		{"{for $k, $v in map: $k => upper($v)}", "{for k, v in map: k => upper(v)}", matches(1)},

		// index expression
		{"foo[a]", "foo[a]", matches(1)},
		{"foo[a]", "foo[b]", noMatch},

		// index expression (wildcard)
		{"$_", "foo[a]", matches(1)},
		{"foo[$x]", "foo[a]", matches(1)},

		// splat expression
		{"tuple.*.foo.bar[0]", "tuple.*.foo.bar[0]", matches(1)},
		{"tuple.*.foo.bar[0]", "tuple.*.bar.bar[0]", noMatch},
		{"tuple[*].foo.bar[0]", "tuple[*].foo.bar[0]", matches(1)},
		{"tuple[*].foo.bar[0]", "tuple[*].bar.bar[0]", noMatch},

		// splat expression (wildcard)
		{"$_", "tuple.*.foo.bar[0]", matches(1)},
		{"$_", "tuple[*].foo.bar[0]", matches(1)},

		// parenthese expression
		{"(a)", "(a)", matches(1)},
		{"(a)", "(b)", noMatch},

		// parenthese expression (wildcard)
		{"$_", "(a)", matches(1)},
		{"($_)", "(b)", matches(1)},

		// unary operation expression
		{"-1", "-1", matches(1)},
		{"-1", "1", noMatch},

		// unary operation expression (wildcard)
		{"$_", "-1", matches(1)},
		{"$_", "!true", matches(1)},

		// binary operation expression
		{"1+1", "1+1", matches(1)},
		{"1+1", "1-1", noMatch},

		// binary operation expression (wildcard)
		{"$_", "1+1", matches(1)},

		// conditional expression
		{"cond? 0:1", "cond? 0:1", matches(1)},
		{"cond? 0:1", "cond? 1:0", noMatch},

		// conditional expression (wildcard)
		{"$_", "cond? 0:1", matches(1)},
		{"$_? 0:1", "cond? 0:1", matches(1)},
		{"cond? 0:$_", "cond? 0:1", matches(1)},

		// scope traversal expression
		{"a", "a", matches(1)},
		{"a", "b", noMatch},
		{"a.attr", "a.attr", matches(1)},
		{"a.attr", "a.attr2", noMatch},
		{"a[0]", "a[0]", matches(1)},
		{"a[0]", "a[1]", noMatch},
		{"a.0", "a.0", matches(1)},
		{"a.0", "a[0]", matches(1)}, //index or legacy index are considered the same
		{"a.0", "a.1", noMatch},

		// scope traversal expression (wildcard)
		{"$_", "a", matches(1)},
		{"$_", "a.attr", matches(1)},
		{"$_", "a[0]", matches(1)},
		{"$_", "a.0", matches(1)},
		{"$_", "a.x.y.x", matches(1)},
		{"$_.$_", "a.x.y.x", noMatch},
		{"a.$_.$_.$_", "a.x.y.z", matches(1)},
		{"a.$x.$_.$x", "a.x.y.z", noMatch},
		{"a.$x.$_.$x", "a.x.y.x", matches(1)},
		{"$_.$x.$_.$x", "a.x.y.x", matches(1)},
		{"a[$x]", "a[1]", noMatch}, // This is due to the key of the traverser index is a cty.Value, which is not either a string or an ast node.

		// relative traversal expression
		{"sort()[0]", "sort()[0]", matches(1)},
		{"sort()[0]", "sort()[1]", noMatch},
		{"sort()[0]", "reverse()[0]", noMatch},

		// relative traversal expression (wildcard)
		{"$_", "sort()[0]", matches(1)},
		{"$_()[0]", "sort()[0]", matches(1)},
		{"$_()[0]", "sort(arg)[0]", noMatch},

		// TODO: object cons key expression
		// TODO: template join expression
		// TODO: template wrap expression
		// TODO: anonym symbol expression

		// body
		{
			expr: `
a = 1
block {
  b = 2
}
`,
			src: `
a = 1
block {
  b = 2
}
`,
			anyWant: matches(1),
		},
		{
			expr: `
a = 1
block {
  b = 2
}
`,
			src: `
a = 1
`,
			anyWant: noMatch,
		},

		// body (wildcard)
		{
			expr: `$_`,
			src: `
a = 1
block {
  b = 2
}
`,
			anyWant: matches(1),
		},
		{
			expr: `
blk {
  $_ {}
}
`,
			src: `
blk {
  blk1 {}
}
`,
			anyWant: matches(1),
		},

		// attribute
		{"a = a", "a = a", matches(1)},
		{"a = a", "a = b", noMatch},

		// attribute (wildcard)
		{"$_", "a = a", matches(1)},
		{"$x = $x", "a = a", matches(1)},
		{"$x = $x", "a = b", noMatch},

		// attributes
		{
			expr: `
a = b
c = d
`,
			src: `
a = b
c = d
`,
			anyWant: matches(1),
		},
		{
			expr: `
a = b
c = d
`,
			src: `
a = b
`,
			anyWant: noMatch,
		},

		// attributes (wildcard)
		{
			expr: `$_`,
			src: `
a = b
c = d
`,
			anyWant: matches(1),
		},
		{
			expr: `
a = $x
c = $x
`,
			src: `
a = b
c = d
`,
			anyWant: noMatch,
		},
		{
			expr: `
a = $x
c = $x
`,
			src: `
a = b
c = b
`,
			anyWant: matches(1),
		},
		{
			expr: `
a = $x
c = $x
`,
			src: `
a = b
c = b
`,
			anyWant: matches(1),
		},

		// block
		{
			expr: `blk {
	a = b
}`,
			src: `blk {
	a = b
}`,
			anyWant: matches(1),
		},
		{
			expr: `blk {
	a = b
	c = d
}`,
			src: `blk {
	a = b
}`,
			anyWant: noMatch,
		},

		// block (wildcard)
		{
			expr: "$_",
			src: `blk {
	a = b
}`,
			anyWant: matches(1),
		},
		{
			expr: `$_ {
    a = b
}`,
			src: `blk {
	a = b
}`,
			anyWant: matches(1),
		},
		{
			expr: `blk {
	a = $x
	c = $x
}`,
			src: `blk {
	a = b
	c = d
}`,
			anyWant: noMatch,
		},
		{
			expr: `blk {
	a = $x
	c = $x
}`,
			src: `blk {
	a = b
	c = b
}`,
			anyWant: matches(1),
		},

		// block body
		{
			expr: `{
	a = b
}`,
			src: `{
	a = b
}`,
			anyWant: matches(1),
		},
		{
			expr: `{
	a = b
}`,
			src: `{
	a = b
	c = d
}`,
			anyWant: noMatch,
		},

		// block body (wildcard)
		{
			expr: "$_",
			src: `{
	a = b
}`,
			anyWant: matches(1),
		},
		{
			expr: `{
	a = $x
	a = $x
}`,
			src: `{
	a = b
	c = d
}`,
			anyWant: noMatch,
		},
		{
			expr: `{
	a = $x
	c = $x
}`,
			src: `{
	a = b
	c = b
}`,
			anyWant: matches(1),
		},

		// blocks
		{
			expr: `blk1 {
	a = b
}

blk2 {
    c = d
}`,
			src: `blk1 {
	a = b
}

blk2 {
    c = d
}`,
			anyWant: matches(1),
		},
		{
			expr: `blk1 {
	a = b
}

blk2 {
    c = d
}`,
			src: `blk1 {
	a = b
}`,
			anyWant: noMatch,
		},

		// blocks (wildcard)
		{
			expr: `$_`,
			src: `blk1 {
	a = b
}

blk2 {
    c = d
}`,
			anyWant: matches(1),
		},
		{
			expr: `
$x {
	a = b
}

$x {
    c = d
}`,
			src: `
blk1 {
	a = b
}

blk2 {
    c = d
}`,
			anyWant: noMatch,
		},
		{
			expr: `
$x {
	a = b
}

$x {
    c = d
}`,
			src: `
blk1 {
	a = b
}

blk1 {
    c = d
}`,
			anyWant: matches(1),
		},

		// expr tokenize errors
		{"$", "", tokErr(":1,2-2: $ must be followed by ident, got TokenEOF")},

		// expr parse errors
		{"a = ", "", parseErr(":1,3-3: Missing expression; Expected the start of an expression, but found the end of the file.")},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
			grepTest(t, tc.expr, tc.src, tc.anyWant)
		})
	}
}

func grepTest(t *testing.T, expr, src string, anyWant interface{}) {
	terr := func(format string, a ...interface{}) {
		t.Errorf("%s | %s: %s", expr, src, fmt.Sprintf(format, a...))
	}
	match, err := grep(expr, src)
	switch want := anyWant.(type) {
	case wantErr:
		if err == nil {
			terr("wanted error %q, got none", want)
		} else if got := err.Error(); got != string(want) {
			terr("wanted error %q, got %q", want, got)
		}
	case matches:
		if err != nil {
			terr("unexpected error: %v", err)
			return
		}
		if match && want == 0 {
			terr("got unexpected match")
		} else if !match && want > 0 {
			terr("wanted match, got none")
		}
	default:
		panic(fmt.Sprintf("unexpected anyWant type: %T", anyWant))
	}
}
