package main

import (
	"bytes"
	"text/template"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

var blockBodyTempl = template.Must(template.New("block_body").Parse(`type {{ . }} `))

func asBlockBody(src []byte) []byte {
	var buf bytes.Buffer
	if err := blockBodyTempl.Execute(&buf, src); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func parse(src []byte, filename string, start hcl.Pos, maybeBlockBody bool) (hclsyntax.Node, hcl.Diagnostics) {
	// try as expr
	if expr, diags := hclsyntax.ParseExpression(src, filename, start); !diags.HasErrors() {
		return expr, nil
	}

	if maybeBlockBody {
		// try as block body
		block, diags := hclsyntax.ParseConfig(asBlockBody(src), filename, start)
		if diags.HasErrors() {
			return nil, diags
		}
		return block.Body.(*hclsyntax.Body).Blocks[0].Body, nil
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
