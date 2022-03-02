package hclgrep

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type wantErr string

func tokErr(msg string) wantErr {
	return wantErr("cannot tokenize expr: " + msg)
}

func parseErr(msg string) wantErr {
	return wantErr("cannot parse expr: " + msg)
}

func attrErr(msg string) wantErr {
	return wantErr("cannot parse attribute: " + msg)
}

func otherErr(msg string) wantErr {
	return wantErr(msg)
}

func TestMatch(t *testing.T) {
	tests := []struct {
		args []string
		src  string
		want interface{}
	}{
		// literal expression
		{[]string{"-x", "1"}, "1", 1},
		{[]string{"-x", "true"}, "false", 0},

		// literal expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = 1", 1},
		{[]string{"-x", "x = $_"}, "x = false", 1},
		{[]string{"-x", "x = $*_"}, "x = false", 1},

		// tuple cons expression
		{[]string{"-x", "[1, 2]"}, "[1, 3]", 0},
		{[]string{"-x", "[1, 2]"}, "[1, 2]", 1},

		// tuple cons expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = [1, 2, 3]", 1},
		{[]string{"-x", "[1, $_, 3]"}, "[1, 2, 3]", 1},
		{[]string{"-x", "[1, $_, 3]"}, "[1, 3]", 0},
		{[]string{"-x", "[1, $x, $x]"}, "[1, 2, 2]", 1},
		{[]string{"-x", "[1, $x, $x]"}, "[1, 2, 3]", 0},
		{
			args: []string{"-x", `
[
	$x,
	1,
	$x,
]`},
			src: `
[
	2,
	1,
	2,
]`,
			want: 1,
		},
		{
			args: []string{"-x", `
[
	$x,
	1,
	$x,
]`},
			src:  `[2, 1, 2]`,
			want: 1,
		},
		{[]string{"-x", "[1, $*_]"}, "[1, 2, 3]", 1},
		{[]string{"-x", "[$*_, 1]"}, "[1, 2, 3]", 0},
		{[]string{"-x", "[$*_]"}, "[]", 1},
		{[]string{"-x", "[$*_, $x]"}, "[1, 2, 3]", 1},

		// object const expression
		{[]string{"-x", "{a = b}"}, "{a = b}", 1},
		{[]string{"-x", "{a = c}"}, "{a = b}", 0},
		{
			args: []string{"-x", `
		{
			a = b
			c = d
		}`},
			src: `
		{
			a = b
			c = d
		}`,
			want: 1,
		},

		// object const expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = {a = b}", 1},
		{[]string{"-x", "{$x = $x}"}, "{a = a}", 1},
		{[]string{"-x", "{$x = $x}"}, "{a = b}", 0},
		{
			args: []string{"-x", `
		{
			a = $x
			c = $x
		}`},
			src: `
		{
			a = b
			c = b
		}`,
			want: 1,
		},
		{
			args: []string{"-x", `
		{
			a = $x
			c = $x
		}`},
			src: `
		{
			a = b
			c = d
		}`,
			want: 0,
		},
		{
			args: []string{"-x", `
		{
			$_ = $_
			$_ = $_
		}`},
			src: `
		{
			a = b
			c = d
		}`,
			want: 1,
		},
		{
			args: []string{"-x", `
		{
			@_
			@_
		}`},
			src: `
		{
			a = b
			c = d
		}`,
			want: 1,
		},
		{
			args: []string{"-x", `
		{
			@*_
		}`},
			src: `
		{
			a = b
			c = d
		}`,
			want: 1,
		},
		{
			args: []string{"-x", `
		{
			@*_
			e = f
		}`},
			src: `
		{
			a = b
			c = d
			e = f
		}`,
			want: 1,
		},

		// template expression
		{[]string{"-x", `"a"`}, `"a"`, 1},
		{[]string{"-x", `"a"`}, `"b"`, 0},
		{
			args: []string{"-x", `<<EOF
content
EOF
`},
			src: `<<EOF
content
EOF
`,
			want: 1,
		},
		{
			args: []string{"-x", `<<EOF
content
EOF
`},
			src: `<<EOF
other content
EOF
`,
			want: 0,
		},

		// template expression (wildcard)
		{[]string{"-x", `x= $_`}, `x = "a"`, 1},
		{
			args: []string{"-x", "x = $_"},
			src: `x = <<EOF
content
EOF
`,
			want: 1,
		},

		// function call expression
		{[]string{"-x", "f1()"}, "f1()", 1},
		{[]string{"-x", "f1()"}, "f2()", 0},
		{[]string{"-x", "f1()"}, "f1(arg)", 0},

		// function call expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = f1()", 1},
		{[]string{"-x", "$_()"}, "f1()", 1},
		{[]string{"-x", "$_()"}, "f1(arg)", 0},
		{[]string{"-x", "f1($_)"}, "f1(arg)", 1},
		{[]string{"-x", "$_($_)"}, "f1(arg)", 1},
		{[]string{"-x", "f1($x, $x)"}, "f1(arg, arg)", 1},
		{[]string{"-x", "f1($x, $x)"}, "f1(arg, arg2)", 0},
		{[]string{"-x", "f1($*_)"}, "f1(arg, arg2)", 1},
		{[]string{"-x", "f1($*_, arg1)"}, "f1(arg, arg2)", 0},

		// for expression
		{[]string{"-x", "[for i in list: i]"}, "[for i in list: i]", 1},
		{[]string{"-x", "[for i in list: i]"}, "[for i in list: upper(i)]", 0},
		{[]string{"-x", "{for k, v in map: k => v}"}, "{for k, v in map: k => upper(v)}", 0},
		{[]string{"-x", "{for k, v in map: k => upper(v)}"}, "{for k, v in map: k => upper(v)}", 1},

		// for expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = {for k, v in map: k => upper(v)}", 1},
		{[]string{"-x", "{for k, v in map: $k => upper($v)}"}, "{for k, v in map: k => upper(v)}", 1},
		{[]string{"-x", "{for $k, $v in map: $k => upper($v)}"}, "{for k, v in map: k => upper(v)}", 1},

		// index expression
		{[]string{"-x", "foo[a]"}, "foo[a]", 1},
		{[]string{"-x", "foo[a]"}, "foo[b]", 0},

		// index expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = foo[a]", 1},
		{[]string{"-x", "foo[$x]"}, "foo[a]", 1},
		{[]string{"-x", "foo[$*x]"}, "foo[a]", 1},
		{[]string{"-x", "a[$x]"}, "a[1]", 1},
		{[]string{"-x", "foo()[$x]"}, "foo()[1]", 1},
		{[]string{"-x", "[1,2,3][$x]"}, "[1,2,3][1]", 1},
		{[]string{"-x", `"abc"[$x]`}, `"abc"[0]`, 1},
		{[]string{"-x", `x[0][$x]`}, `x[0][0]`, 1},
		{[]string{"-x", `x[$x][$x]`}, `x[0][0]`, 1},
		{[]string{"-x", `x[$x][$x]`}, `x[0][1]`, 0},

		// splat expression
		{[]string{"-x", "tuple.*.foo.bar[0]"}, "tuple.*.foo.bar[0]", 1},
		{[]string{"-x", "tuple.*.foo.bar[0]"}, "tuple.*.bar.bar[0]", 0},
		{[]string{"-x", "tuple[*].foo.bar[0]"}, "tuple[*].foo.bar[0]", 1},
		{[]string{"-x", "tuple[*].foo.bar[0]"}, "tuple[*].bar.bar[0]", 0},

		// splat expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = tuple.*.foo.bar[0]", 1},
		{[]string{"-x", "x = $_"}, "x = tuple[*].foo.bar[0]", 1},
		{[]string{"-x", "x = $*_"}, "x = tuple[*].foo.bar[0]", 1},

		// parenthese expression
		{[]string{"-x", "(a)"}, "(a)", 1},
		{[]string{"-x", "(a)"}, "(b)", 0},

		// parenthese expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = (a)", 1},
		{[]string{"-x", "($_)"}, "(b)", 1},
		{[]string{"-x", "($*_)"}, "(b)", 1},

		// unary operation expression
		{[]string{"-x", "-1"}, "-1", 1},
		{[]string{"-x", "-1"}, "1", 0},

		// unary operation expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = -1", 1},
		{[]string{"-x", "x = $_"}, "x = !true", 1},
		{[]string{"-x", "x = $*_"}, "x = !true", 1},

		// binary operation expression
		{[]string{"-x", "1+1"}, "1+1", 1},
		{[]string{"-x", "1+1"}, "1-1", 0},

		// binary operation expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = 1+1", 1},
		{[]string{"-x", "x = $*_"}, "x = 1+1", 1},

		// conditional expression
		{[]string{"-x", "cond? 0:1"}, "cond? 0:1", 1},
		{[]string{"-x", "cond? 0:1"}, "cond? 1:0", 0},

		// conditional expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = cond? 0:1", 1},
		{[]string{"-x", "$_? 0:1"}, "cond? 0:1", 1},
		{[]string{"-x", "cond? 0:$_"}, "cond? 0:1", 1},
		{[]string{"-x", "cond? 0:$*_"}, "cond? 0:1", 1},

		// scope traversal expression
		{[]string{"-x", "a"}, "a", 1},
		{[]string{"-x", "a"}, "b", 0},
		{[]string{"-x", "a.attr"}, "a.attr", 1},
		{[]string{"-x", "a.attr"}, "a.attr2", 0},
		{[]string{"-x", "a[0]"}, "a[0]", 1},
		{[]string{"-x", "a[0]"}, "a[1]", 0},
		{[]string{"-x", "a.0"}, "a.0", 1},
		{[]string{"-x", "a.0"}, "a[0]", 1}, //index or legacy index are considered the same
		{[]string{"-x", "a.0"}, "a.1", 0},

		// scope traversal expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = a", 1},
		{[]string{"-x", "x = $_"}, "x = a.attr", 1},
		{[]string{"-x", "x = $_"}, "x = a[0]", 1},
		{[]string{"-x", "x = $_"}, "x = a.0", 1},
		{[]string{"-x", "x = $_"}, "x = a.x.y.x", 1},
		{[]string{"-x", "$_.$_"}, "a.x.y.x", 0},
		{[]string{"-x", "a.$_.$_.$_"}, "a.x.y.z", 1},
		{[]string{"-x", "a.$x.$_.$x"}, "a.x.y.z", 0},
		{[]string{"-x", "a.$x.$_.$x"}, "a.x.y.x", 1},
		{[]string{"-x", "$_.$x.$_.$x"}, "a.x.y.x", 1},
		{[]string{"-x", "a.$x.$*_.$x"}, "a.x.y.z", 0},

		// relative traversal expression
		{[]string{"-x", "sort()[0]"}, "sort()[0]", 1},
		{[]string{"-x", "sort()[0]"}, "sort()[1]", 0},
		{[]string{"-x", "sort()[0]"}, "reverse()[0]", 0},

		// relative traversal expression (wildcard)
		{[]string{"-x", "x = $_"}, "x = sort()[0]", 1},
		{[]string{"-x", "$_()[0]"}, "sort()[0]", 1},
		{[]string{"-x", "$_()[0]"}, "sort(arg)[0]", 0},
		{[]string{"-x", "$*_()[0]"}, "sort(arg)[0]", 0},

		// TODO: object cons key expression
		// TODO: template join expression
		// TODO: template wrap expression
		// TODO: anonym symbol expression

		// attribute
		{[]string{"-x", "a = a"}, "a = a", 1},
		{[]string{"-x", "a = a"}, "a = b", 0},

		// attribute (wildcard)
		{[]string{"-x", "$x = $x"}, "a = a", 1},
		{[]string{"-x", "$x = $x"}, "a = b", 0},
		{[]string{"-x", "$x = $*_"}, "a = b", 1},

		// attributes
		{
			args: []string{"-x", `
a = b
c = d
`},
			src: `
a = b
c = d
`,
			want: 1,
		},
		{
			args: []string{"-x", `
a = b
c = d
`},
			src: `
a = b
`,
			want: 0,
		},

		// attributes (wildcard)
		{
			args: []string{"-x", `
@x
@y
`},
			src: `
a = b
c = d
`,
			want: 1,
		},
		{
			args: []string{"-x", `
a = $x
c = $x
`},
			src: `
a = b
c = d
`,
			want: 0,
		},
		{
			args: []string{"-x", `
a = $x
c = $x
`},
			src: `
a = b
c = b
`,
			want: 1,
		},
		{
			args: []string{"-x", `
a = $x
c = $x
`},
			src: `
a = b
c = b
`,
			want: 1,
		},
		{
			args: []string{"-x", `@*_`},
			src: `
a = b
c = d
`,
			want: 2,
		},
		{
			args: []string{"-x", `
@*_
e = f
`},
			src: `
a = b
c = d
e = f
`,
			want: 1,
		},

		// block
		{
			args: []string{"-x", `blk {
	a = b
}`},
			src: `blk {
	a = b
}`,
			want: 1,
		},
		{
			args: []string{"-x", `blk {
	a = b
	c = d
}`},
			src: `blk {
	a = b
}`,
			want: 0,
		},

		// block (wildcard)
		{
			args: []string{"-x", `$_ {
    a = b
}`},
			src: `blk {
	a = b
}`,
			want: 1,
		},
		{
			args: []string{"-x", `blk {
	a = $x
	c = $x
}`},
			src: `blk {
	a = b
	c = d
}`,
			want: 0,
		},
		{
			args: []string{"-x", `blk {
	a = $x
	c = $x
}`},
			src: `blk {
	a = b
	c = b
}`,
			want: 1,
		},
		{
			args: []string{"-x", `$a {
	a = $x
	b = ""
}`},
			src: `
blk1 {
	blk2 {
		a = file("./a.txt")
		b = ""
	}
}
`,
			want: 1,
		},
		{
			args: []string{"-x", `$*_ {
	a = b
}`},
			src: `type label1 label2 {
	a = b
}`,
			want: 0,
		},
		{
			args: []string{"-x", `type $*_ {
	a = b
}`},
			src: `type label1 label2 {
	a = b
}`,
			want: 1,
		},

		// blocks
		{
			args: []string{"-x", `blk1 {
	a = b
}

blk2 {
    c = d
}`},
			src: `blk1 {
	a = b
}

blk2 {
    c = d
}`,
			want: 1,
		},
		{
			args: []string{"-x", `blk1 {
	a = b
}

blk2 {
    c = d
}`},
			src: `blk1 {
	a = b
}`,
			want: 0,
		},

		// blocks (wildcard)
		{
			args: []string{"-x", `
$x {
	a = b
}

$x {
    c = d
}`},
			src: `
blk1 {
	a = b
}

blk2 {
    c = d
}`,
			want: 0,
		},
		{
			args: []string{"-x", `
$x {
	a = b
}

$x {
    c = d
}`},
			src: `
blk1 {
	a = b
}

blk1 {
    c = d
}`,
			want: 1,
		},
		{
			args: []string{"-x", `
@*_

$x {
    c = d
}`},
			src: `
blk1 {}
blk1 {}

blk1 {
    c = d
}`,
			want: 1,
		},
		{
			args: []string{"-x", `$_`},
			src: `
blk1 {}
blk1 {}`,
			want: 5, // 1 toplevel body + 2* (1 body + 1 block)
		},

		// body
		{
			args: []string{"-x", `
a = 1
block {
  b = 2
}
`},
			src: `
a = 1
block {
  b = 2
}
`,
			want: 1,
		},
		{
			args: []string{"-x", `
a = 1
block {
  b = 2
}
`},
			src: `
a = 1
`,
			want: 0,
		},

		// body (wildcard)
		{
			args: []string{"-x", `blk {
  @_
  @_
}`},
			src: `
blk {
	a = 1
	block {
	  b = 2
	}
}
`,
			want: 1,
		},
		{
			args: []string{"-x", `@x`},
			src: `
blk {
	a = 1
	block {
	  b = 2
	}
}
`,
			want: 4,
		},
		{
			args: []string{"-x", `
blk {
  $_ {}
}
`},
			src: `
blk {
  blk1 {}
}
`,
			want: 1,
		},
		{
			args: []string{"-x", `
@_

blk {
 @_
}
`},
			src: `
a = b

blk {
 blk1 {}
}
`,
			want: 1,
		},
		{
			args: []string{"-x", `
@x

blk {
 @x
}
`},
			src: `
a = b

blk {
 a = b
}
`,
			want: 1,
		},
		{
			args: []string{"-x", `
@x

blk {
 @x
}
`},
			src: `
a = b

blk {
 a = c
}
`,
			want: 0,
		},
		{
			args: []string{"-x", `
@x

blk {
 @x
}
`},
			src: `
blk1 {}

blk {
 blk1 {}
}
`,
			want: 1,
		},
		{
			args: []string{"-x", `
@x

blk {
 @x
}
`},
			src: `
a = b

blk {
 blk1 {}
}
`,
			want: 0,
		},
		{
			args: []string{"-x", `
@*_

blk {
 @x
}
`},
			src: `
a = b
blk1 {}

blk {
 blk1 {}
}
`,
			want: 1,
		},

		// expr tokenize errors
		{[]string{"-x", "$"}, "", tokErr(":1,2-2: wildcard must be followed by ident, got TokenEOF")},

		// expr parse errors
		{[]string{"-x", "a = "}, "", parseErr(":1,3-3: Missing expression; Expected the start of an expression, but found the end of the file.")},

		// no command
		{[]string{}, "", otherErr("need at least one command")},

		// empty source
		{[]string{"-x", ""}, "", 1},
		{[]string{"-x", "\t"}, "", 1},
		{[]string{"-x", "a"}, "", 0},

		// "-p"
		{
			args: []string{"-p", "0"},
			src: `
blk {
  x = 1
}`,
			want: `blk {
  x = 1
}`,
		},
		{
			args: []string{"-p", "1"},
			src: `
blk {
  x = 1
}`,
			want: 0,
		},
		{
			args: []string{"-p", "-1"},
			src: `
blk {
  x = 1
}`,
			want: wantErr("the number follows `-p` must >=0, got -1"),
		},
		{
			args: []string{"-x", "x = 1", "-p", "1"},
			src: `
blk {
  x = 1
}`,
			want: `{
  x = 1
}`,
		},
		{
			args: []string{"-x", "x = 1", "-p", "2"},
			src: `
blk {
  x = 1
}`,
			want: `blk {
  x = 1
}`,
		},

		// "-rx"
		{
			args: []string{"-x", "x = $a", "-rx", `a="1"`},
			src:  `x = 1`,
			want: `x = 1`,
		},
		{
			args: []string{"-x", "x = $a", "-rx", `a="f.."`},
			src:  `x = "foo"`,
			want: `x = "foo"`,
		},
		{
			args: []string{"-x", "x = $a", "-rx", `a="true"`},
			src:  `x = true`,
			want: `x = true`,
		},
		{
			args: []string{"-x", "x = $a", "-rx", `a="false"`},
			src:  `x = true`,
			want: 0,
		},
		{
			args: []string{"-x", "x = $a", "-rx", `a="\*"`},
			src:  `x = "*"`,
			want: 1,
		},
		{
			args: []string{"-x", "x = $a", "-rx", `a="\*"`},
			src:  `x = 123`,
			want: 0,
		},
		{
			args: []string{"-x", "x = $a", "-rx", `nonexist="false"`},
			src:  `x = true`,
			want: 0,
		},
		{
			args: []string{"-x", "x = $a", "-rx", ``},
			src:  ``,
			want: attrErr(":1,1-1: attribute must starts with an ident, got \"TokenEOF\""),
		},
		{
			args: []string{"-x", "x = $a", "-rx", `a1`},
			src:  ``,
			want: attrErr(":1,3-3: attribute name must be followed by \"=\", got \"TokenEOF\""),
		},
		{
			args: []string{"-x", "x = $a", "-rx", `a1=abc`},
			src:  ``,
			want: attrErr(":1,4-7: attribute value must enclose within quotes"),
		},
		{
			args: []string{"-x", "x = $a", "-rx", `a1="abc`},
			src:  ``,
			want: attrErr(":1,8-8: attribute value must enclose within quotes"),
		},
		{
			args: []string{"-x", "x = $a", "-rx", `a1="abc"tail`},
			src:  ``,
			want: attrErr(":1,9-13: invalid content after attribute value"),
		},

		// "-v"
		{
			args: []string{"-x", "blk {@*_}", "-v", `a = $_`},
			src: `blk {
	a = 1
}

blk {
	b = 1
}`,
			want: `blk {
	b = 1
}`,
		},
		// `-v` pattern won't record wildcard name
		{
			args: []string{"-x", "blk {@*_}", "-v", `a = $x`, "-rx", `x="1"`},
			src: `blk {
	a = 1
}

blk {
	b = 1
}`,
			want: 0,
		},

		// "-g"
		{
			args: []string{"-x", "blk {@*_}", "-g", `a = $_`},
			src: `blk {
	a = 1
}

blk {
	b = 1
}`,
			want: `blk {
	a = 1
}`,
		},
		// `-g` pattern records wildcard name
		{
			args: []string{"-x", "blk {@*_}", "-g", `a = $x`, "-rx", `x="1"`},
			src: `blk {
	a = 1
}

blk {
	b = 1
}`,
			want: `blk {
	a = 1
}`,
		},
		// short circut of -g pattern match, the recorded wildcard name is the first match (DFS)
		{
			args: []string{"-x", "blk {@*_}", "-g", `a = $x`, "-rx", `x="1"`},
			src: `blk {
	a = 1
	nest {
		a = 2
	}
}`,
			want: 1,
		},
		{
			args: []string{"-x", "blk {@*_}", "-g", `a = $x`, "-rx", `x="2"`},
			src: `blk {
	a = 1
	nest {
		a = 2
	}
}`,
			want: 0,
		},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
			matchTest(t, tc.args, tc.src, tc.want)
		})
	}
}

func matchTest(t *testing.T, args []string, src string, anyWant interface{}) {
	tfatalf := func(format string, a ...interface{}) {
		t.Fatalf("%v | %s: %s", args, src, fmt.Sprintf(format, a...))
	}
	m := &Matcher{
		Out:  io.Discard,
		test: true,
	}
	cmds, _, err := m.ParseCmds(args)
	switch want := anyWant.(type) {
	case wantErr:
		if err == nil {
			tfatalf("wanted error %q, got none", want)
		} else if got := err.Error(); got != string(want) {
			tfatalf("wanted error %q, got %q", want, got)
		}
		return
	}

	if err != nil {
		tfatalf("unexpected error: %v", err)
	}

	matches := matchStrs(m, cmds, src)
	switch want := anyWant.(type) {
	case int:
		if len(matches) != want {
			tfatalf("wanted %d matches, got=%d", want, len(matches))
		}
	case string:
		if l := len(matches); l != 1 {
			if l == 0 {
				tfatalf("no match")
			} else {
				tfatalf("unexpected multiple matches %d", len(matches))
			}
		}
		m := matches[0]
		got := string(m.Range().SliceBytes([]byte(src)))
		if want != got {
			tfatalf("wanted:\n%s\ngot:\n%s\n", want, got)
		}
	default:
		panic(fmt.Sprintf("unexpected anyWant type: %T", anyWant))
	}
}

func matchStrs(m *Matcher, cmds []Cmd, src string) []hclsyntax.Node {
	srcNode, err := parse([]byte(src), "", hcl.InitialPos)
	if err != nil {
		panic(fmt.Sprintf("parsing source node: %v", err))
	}
	return m.matches(cmds, srcNode)
}

func TestFile(t *testing.T) {
	tests := []struct {
		args []string
		src  string
		want interface{}
	}{
		// invalid -H value
		{[]string{"-H=foo"}, "", otherErr(`invalid boolean value "foo" for -H: flag can only be boolean`)},

		// reading from stdin without -H
		{[]string{"-x", "foo = bar"}, "foo = bar", "foo = bar\n"},
		// reading from stdin with -H
		{[]string{"-H", "-x", "foo = bar"}, "foo = bar", `:1,1-10:
foo = bar
`},
		// reading from stdin with -H=false
		{[]string{"-H=false", "-x", "foo = bar"}, "foo = bar", "foo = bar\n"},

		// reading from one file without -H
		{[]string{"-x", "foo = bar", "file"}, "foo = bar", "foo = bar\n"},
		// reading from one file with -H
		{[]string{"-H", "-x", "foo = bar", "file"}, "foo = bar", `:1,1-10:
foo = bar
`},
		// reading from one file with -H=false
		{[]string{"-H=false", "-x", "foo = bar", "file"}, "foo = bar", "foo = bar\n"},

		// reading from one multiple files without -H
		{[]string{"-x", "foo = bar", "file1", "file2"}, "foo = bar", `:1,1-10:
foo = bar
`},
		// reading from one multiple files with -H
		{[]string{"-H", "-x", "foo = bar", "file1", "file2"}, "foo = bar", `:1,1-10:
foo = bar
`},
		// reading from one multiple files with -H=false
		{[]string{"-H=false", "-x", "foo = bar", "file1", "file2"}, "foo = bar", "foo = bar\n"},

		// -w only prints nothing
		{[]string{"-w", "abc"}, "foo = bar", ""},
		// -w is not the last command
		{[]string{"-x", "foo = $a", "-w", "a", "-x", "foo = $a"}, "foo = bar", otherErr("`-w` must be the last command")},
		// -w
		{[]string{"-x", "foo = $a", "-w", "a"}, "foo = bar", "bar\n"},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("%02d", i), func(t *testing.T) {
			fileTest(t, tc.args, tc.src, tc.want)
		})
	}
}

func fileTest(t *testing.T, args []string, src string, anyWant interface{}) {
	tfatalf := func(format string, a ...interface{}) {
		t.Fatalf("%v | %s: %s", args, src, fmt.Sprintf(format, a...))
	}
	buf := bytes.NewBufferString("")
	m := &Matcher{
		Out:  buf,
		b:    []byte(src),
		test: true,
	}
	cmds, _, err := m.ParseCmds(args)
	switch want := anyWant.(type) {
	case wantErr:
		if err == nil {
			tfatalf("wanted error %q, got none", want)
		} else if got := err.Error(); got != string(want) {
			tfatalf("wanted error %q, got %q", want, got)
		}
		return
	}
	if err != nil {
		tfatalf("unexpected error: %v", err)
	}

	if err := m.File(cmds, "", bytes.NewBufferString(src)); err != nil {
		tfatalf("m.file() error: %v", err)
	}
	switch want := anyWant.(type) {
	case string:
		got := buf.String()
		if want != got {
			tfatalf("wanted:\n%s\ngot:\n%s\n", want, got)
		}
	default:
		panic(fmt.Sprintf("unexpected anyWant type: %T", anyWant))
	}
}
