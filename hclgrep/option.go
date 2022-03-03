package hclgrep

import "io"

type Option func(*Matcher)

func OptionCmd(cmd Cmd) Option {
	return func(m *Matcher) {
		m.cmds = append(m.cmds, cmd)
	}
}

func OptionPrefixPosition(include bool) Option {
	return func(m *Matcher) {
		m.prefix = include
	}
}

func OptionOutput(o io.Writer) Option {
	return func(m *Matcher) {
		m.out = o
	}
}
