package hclgrep

import (
	"fmt"
	"os"
)

var usage = func() {
	fmt.Fprintf(os.Stderr, `usage: hclgrep [options] commands [FILE...]

hclgrep performs a query on the given HCL(v2) files.

An option is one of the following:

    -H                  prefix the filename and byte offset of a match (defaults to "true" when reading from multiple files)

A command is one of the following:

	-%s  pattern         find all nodes matching a pattern
	-%s  pattern         discard nodes not matching a pattern
	-%s  pattern         discard nodes matching a pattern
	-%s  number          navigate up a number of node parents
	-%s name="regexp"   filter nodes by regexp against wildcard value of "name"
	-%s  name            print the wildcard node only (must be the last command)

A pattern is a piece of HCL code which may include wildcards. It can be:

- A body (zero or more attributes, and zero or more blocks)
- An expression

There are two types of wildcards can be used in a pattern, depending on the scope it resides in:

- Attribute wildcard ("@"): represents an attribute, a block or an object element
- Expression wildcard ("$"): represents an expression or a place that a string is accepted (i.e. as a block type, block label)

The wildcards are followed by a name. Each wildcard with the same name must match the same node/string, excluding "_". Example:

    $x.$_ = $x # assignment of self to a field in self

The wildcard name is only recorded for "-x" command or "-g" command (the first match in DFS).

If "*" is before the name, it will match any number of nodes. Example:

    [$*_] # any number of elements in a tuple

    resource foo "name" {
        @*_  # any number of attributes/blocks inside the resource block body
    }
`, CmdNameMatch, CmdNameFilterMatch, CmdNameFilterUnMatch, CmdNameParent, CmdNameRx, CmdNameWrite)
}
