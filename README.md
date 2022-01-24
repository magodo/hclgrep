# hclgrep

![workflow](https://github.com/magodo/hclgrep/actions/workflows/go.yml/badge.svg)

Search for HCL(v2) using syntax tree.

The idea is heavily inspired by https://github.com/mvdan/gogrep.

## Install

    go install github.com/magodo/hclgrep@latest

## Usage

    usage: hclgrep [options] commands [FILE...]

An option is one of the following:

    -H                  prefix the filename and byte offset of a match

A command is one of the following:

    -x  pattern         find all nodes matching a pattern
    -p  number          navigate up a number of node parents
    -rx name="regexp"   filter nodes by regexp against wildcard value of "name"

A pattern is a piece of HCL code which may include wildcards. It can be:

- A [body](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md#bodies) (zero or more attributes, and zero or more blocks)
- An [expression](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md#expressions)

There are two types of wildcards can be used in a pattern, depending on the scope it resides in:

- Attribute wildcard ("@"): represents an [attribute](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md#attribute-definitions), a [block](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md#blocks), or an [object element](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md#collection-values)
- Expression wildcard ("$"): represents an [expression](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md#expressions) or a place that a string is accepted (i.e. as a block type, block label)

The wildcards are followed by a name. Each wildcard with the same name must match the same node/string, excluding "_". Example:

    $x.$_ = $x # assignment of self to a field in self

If "*" is before the name, it will match **any** number of nodes. Example:

    [$*_] # any number of elements in a tuple

    resource foo "name" {
        @*_  # any number of attributes/blocks inside the resource block body
    }

## Example

```shell
$ hclgrep -x 'dynamic $_ {@*_}' main.tf                     # Grep dynamic blocks used in Terraform config
$ hclgrep -x 'var.$_[count.index]' main.tf                  # Grep potential mis-used "count" in Terraform config
$ hclgrep -x 'module $_ {@*_}' -x 'source = $_' main.tf     # Grep module source addresses in Terraform config

# Grep AzureRM Terraform network security rule resource which allows 22 port for inbound traffic
$ hclgrep -x 'resource azurerm_network_security_rule $_ {@*_}' \
    -x 'direction = "Inbound"' -p 1 \
    -x 'access = "Allow"' -p 1 \
    -x 'source_port_range = "22"' -p 1 \
    main.tf 
```

Especially, to grep for the evaluated Terraform configurations, run following command in the root module (given there is no output variables defined):

```
$ terraform show -no-color | sed --expression 's;(sensitive value);"";' | hclgrep -x '<pattern>'
```

## Limitation

- The **any** expression wildcard (`$*`) doesn't work inside a traversal.
- The **any** wildcard doesn't remember the matched wildcard name.
