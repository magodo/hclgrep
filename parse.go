package main

import (
	"bytes"
	"text/template"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

var blockTempl = template.Must(template.New("block").Parse(`type {{ . }} `))

func asBlock(src []byte) []byte {
	var buf bytes.Buffer
	if err := blockTempl.Execute(&buf, src); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func parse(src []byte, filename string, start hcl.Pos) (hclsyntax.Node, hcl.Diagnostics) {
	// try as expr
	if expr, diags := hclsyntax.ParseExpression(src, filename, start); !diags.HasErrors() {
		return expr, nil
	}

	// try as block
	if block, diags := hclsyntax.ParseConfig(asBlock(src), filename, start); !diags.HasErrors() {
		return block.Body.(*hclsyntax.Body).Blocks[0], nil
	}

	// try as file
	f, diags := hclsyntax.ParseConfig(src, filename, start)
	if diags.HasErrors() {
		return nil, diags
	}
	return f.Body.(*hclsyntax.Body), nil
}
