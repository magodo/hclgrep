package main

import (
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type cmd struct {
	name  string
	src   string
	value CmdValue
}

type CmdValue interface {
	Value() interface{}
}

type CmdValueRx struct {
	name string
	rx   regexp.Regexp
}

func (v CmdValueRx) Value() interface{} { return v }

type CmdValueNode struct {
	hclsyntax.Node
}

func (v CmdValueNode) Value() interface{} { return v.Node }

type CmdValueLevel int

func (v CmdValueLevel) Value() interface{} { return v }

type strCmdFlag struct {
	name string
	cmds *[]cmd
}

func (o *strCmdFlag) String() string { return "" }
func (o *strCmdFlag) Set(val string) error {
	*o.cmds = append(*o.cmds, cmd{name: o.name, src: val})
	return nil
}

type prefixFlag struct {
	val bool
	set bool
}

func (o *prefixFlag) String() string { return "" }
func (o *prefixFlag) Set(val string) error {
	if val != "false" && val != "true" {
		return fmt.Errorf("flag can only be boolean")
	}
	o.val = val == "true"
	o.set = true
	return nil
}

func (o *prefixFlag) IsBoolFlag() bool { return true }

func (m *matcher) parseCmds(args []string) ([]cmd, []string, error) {
	eh := flag.ExitOnError
	if m.test {
		eh = flag.ContinueOnError
	}
	flagSet := flag.NewFlagSet("hclgrep", eh)
	flagSet.Usage = usage

	var prefixflag prefixFlag
	flagSet.Var(&prefixflag, "H", "prefix filename and byte offset for a match")

	var cmds []cmd
	flagSet.Var(&strCmdFlag{
		name: "x",
		cmds: &cmds,
	}, "x", "")
	flagSet.Var(&strCmdFlag{
		name: "p",
		cmds: &cmds,
	}, "p", "")
	flagSet.Var(&strCmdFlag{
		name: "rx",
		cmds: &cmds,
	}, "rx", "")

	if err := flagSet.Parse(args); err != nil {
		return nil, nil, err
	}

	files := flagSet.Args()

	m.prefix = prefixflag.val
	if !prefixflag.set {
		m.prefix = len(files) >= 2
	}

	if len(cmds) < 1 {
		return nil, nil, fmt.Errorf("need at least one command")
	}

	for i, cmd := range cmds {
		switch cmd.name {
		case "x":
			node, err := compileExpr(cmd.src)
			if err != nil {
				return nil, nil, err
			}
			cmds[i].value = CmdValueNode{node}
		case "rx":
			name, rx, err := parseRegexpAttr(cmd.src)
			if err != nil {
				return nil, nil, err
			}
			cmds[i].value = CmdValueRx{name: name, rx: *rx}
		case "p":
			n, err := strconv.Atoi(cmd.src)
			if err != nil {
				return nil, nil, err
			}
			if n < 0 {
				return nil, nil, fmt.Errorf("the number follows `-p` must >=0, got %d", n)
			}
			cmds[i].value = CmdValueLevel(n)
		}
	}
	return cmds, files, nil
}

func parseAttr(attr string) (string, string, error) {
	tokens, diags := hclsyntax.LexExpression([]byte(attr), "", hcl.InitialPos)
	if diags.HasErrors() {
		return "", "", fmt.Errorf(diags.Error())
	}
	next := func() hclsyntax.Token {
		tok := tokens[0]
		tokens = tokens[1:]
		return tok
	}
	tok := next()
	if tok.Type != hclsyntax.TokenIdent {
		return "", "", fmt.Errorf("%v: attribute must starts with an ident, got %q", tok.Range, tok.Type)
	}
	name := string(tok.Bytes)
	if tok := next(); tok.Type != hclsyntax.TokenEqual {
		return "", "", fmt.Errorf(`%v: attribute name must be followed by "=", got %q`, tok.Range, tok.Type)
	}
	if tok := next(); tok.Type != hclsyntax.TokenOQuote {
		return "", "", fmt.Errorf("%v: attribute value must enclose within quotes", tok.Range)
	}
	tok = next()
	if tok.Type != hclsyntax.TokenQuotedLit {
		return "", "", fmt.Errorf("%v: attribute value must enclose within quotes", tok.Range)
	}
	value := string(tok.Bytes)
	if tok := next(); tok.Type != hclsyntax.TokenCQuote {
		return "", "", fmt.Errorf("%v: attribute value must enclose within quotes", tok.Range)
	}
	if tok := next(); tok.Type != hclsyntax.TokenEOF {
		return "", "", fmt.Errorf("%v: invalid content after attribute value", tok.Range)
	}
	return name, value, nil
}

func parseRegexpAttr(attr string) (string, *regexp.Regexp, error) {
	name, value, err := parseAttr(attr)
	if err != nil {
		return "", nil, fmt.Errorf("cannot parse attribute: %v", err)
	}
	if !strings.HasPrefix(value, "&") {
		value = "^" + value
	}
	if !strings.HasSuffix(value, "$") {
		value = value + "$"
	}
	rx, err := regexp.Compile(value)
	return name, rx, err
}
