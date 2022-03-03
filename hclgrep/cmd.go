package hclgrep

import (
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

type CmdName string

const (
	CmdNameMatch         CmdName = "x"
	CmdNameFilterMatch           = "g"
	CmdNameFilterUnMatch         = "v"
	CmdNameRx                    = "rx"
	CmdNameParent                = "p"
	CmdNameWrite                 = "w"
)

type Cmd struct {
	name  CmdName
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

type CmdValueString string

func (v CmdValueString) Value() interface{} { return v }

type strCmdFlag struct {
	name CmdName
	cmds *[]Cmd
}

func (o *strCmdFlag) String() string { return "" }
func (o *strCmdFlag) Set(val string) error {
	*o.cmds = append(*o.cmds, Cmd{name: o.name, src: val})
	return nil
}

func ParseArgs(args []string) ([]Option, []string, error) {
	flagSet := flag.NewFlagSet("hclgrep", flag.ContinueOnError)
	flagSet.Usage = usage

	var prefix bool
	flagSet.BoolVar(&prefix, "H", false, "prefix filename and byte offset for a match")

	var cmds []Cmd
	flagSet.Var(&strCmdFlag{
		name: CmdNameMatch,
		cmds: &cmds,
	}, string(CmdNameMatch), "")
	flagSet.Var(&strCmdFlag{
		name: CmdNameFilterMatch,
		cmds: &cmds,
	}, string(CmdNameFilterMatch), "")
	flagSet.Var(&strCmdFlag{
		name: CmdNameFilterUnMatch,
		cmds: &cmds,
	}, string(CmdNameFilterUnMatch), "")
	flagSet.Var(&strCmdFlag{
		name: CmdNameParent,
		cmds: &cmds,
	}, string(CmdNameParent), "")
	flagSet.Var(&strCmdFlag{
		name: CmdNameRx,
		cmds: &cmds,
	}, string(CmdNameRx), "")
	flagSet.Var(&strCmdFlag{
		name: CmdNameWrite,
		cmds: &cmds,
	}, string(CmdNameWrite), "")

	if err := flagSet.Parse(args); err != nil {
		return nil, nil, err
	}

	if len(cmds) < 1 {
		return nil, nil, fmt.Errorf("need at least one command")
	}

	for i, cmd := range cmds {
		switch cmd.name {
		case CmdNameWrite:
			if i != len(cmds)-1 {
				return nil, nil, fmt.Errorf("`-%s` must be the last command", cmd.name)
			}
			cmds[i].value = CmdValueString(cmd.src)
		case CmdNameRx:
			name, rx, err := parseRegexpAttr(cmd.src)
			if err != nil {
				return nil, nil, err
			}
			cmds[i].value = CmdValueRx{name: name, rx: *rx}
		case CmdNameParent:
			n, err := strconv.Atoi(cmd.src)
			if err != nil {
				return nil, nil, err
			}
			if n < 0 {
				return nil, nil, fmt.Errorf("the number follows `-%s` must >=0, got %d", cmd.name, n)
			}
			cmds[i].value = CmdValueLevel(n)
		default:
			node, err := compileExpr(cmd.src)
			if err != nil {
				return nil, nil, err
			}
			cmds[i].value = CmdValueNode{node}
		}
	}

	opts := []Option{OptionPrefixPosition(prefix)}
	for _, cmd := range cmds {
		opts = append(opts, OptionCmd(cmd))
	}
	return opts, flagSet.Args(), nil
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
	if !strings.HasPrefix(value, "^") {
		value = "^" + value
	}
	if !strings.HasSuffix(value, "$") {
		value = value + "$"
	}
	rx, err := regexp.Compile(value)
	return name, rx, err
}
