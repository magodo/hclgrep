package hclgrep

import (
	"fmt"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func compileExpr(expr string) (hclsyntax.Node, error) {
	toks, err := tokenize(expr)
	if err != nil {
		return nil, fmt.Errorf("cannot tokenize expr: %v", err)
	}

	p := toks.Bytes()
	node, diags := parse(p, "", hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("cannot parse expr: %v", diags.Error())
	}
	return node, nil
}

func parse(src []byte, filename string, start hcl.Pos) (hclsyntax.Node, hcl.Diagnostics) {
	// try as expr
	if expr, diags := hclsyntax.ParseExpression(src, filename, start); !diags.HasErrors() {
		return expr, nil
	}

	// try as file
	f, diags := hclsyntax.ParseConfig(src, filename, start)
	if diags.HasErrors() {
		return nil, diags
	}
	// This is critical for parsing the pattern, as here actually wants the specified attribute or block,
	// but not the whole file body, given it is parsed as a file.
	return bodyContent(f.Body.(*hclsyntax.Body)), nil
}

func bodyContent(body *hclsyntax.Body) hclsyntax.Node {
	if body == nil {
		return nil
	}
	if len(body.Blocks) == 0 && len(body.Attributes) == 1 {
		var k string
		for key := range body.Attributes {
			k = key
			break
		}
		return body.Attributes[k]
	}
	if len(body.Blocks) == 1 && len(body.Attributes) == 0 {
		return body.Blocks[0]
	}
	return body
}
