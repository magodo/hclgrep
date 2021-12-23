package main

import "testing"

func TestGrep(t *testing.T) {
	tests := []struct {
		expr, src string
		wantMatch bool
	}{
		{"1", "1", true},
		{"a", "b", false},
		{"true", "false", false},

		{"&x", "abc", true},
		{"[1, &x, 3]", "[1, 2, 3]", true},
		{"[1, &x, 3]", "[1, 3]", false},
		{"{a = &x}", "{a = b}", true},
		{"{&x = &y}", "{a = b}", true},
		{"{&x = &x}", "{a = b}", false},
		{"{&x = &x}", "{a = a}", true},

		{"sort(&x)", "sort(a)", true},

		{"{for k, v in map: &k => upper(&v)}", "{for k, v in map: key => upper(val)}", true},
		{"foo[&x]", "foo[a]", true},
	}
	for _, tc := range tests {
		match, err := grep(tc.expr, tc.src)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}
		if match && !tc.wantMatch {
			t.Errorf("%s | %s: got unexpected match", tc.expr, tc.src)
		} else if !match && tc.wantMatch {
			t.Errorf("%s | %s: wanted match, got none", tc.expr, tc.src)
		}
	}
}
