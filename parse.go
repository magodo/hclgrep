package main

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

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
