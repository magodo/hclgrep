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

		{"&x", "1", true},
		{"&x", "abc", true},
		{"&x", "[1,2,3]", true},
		{"&x", "{a = b}", true},

		{"[1, &x, 3]", "[1, 2, 3]", true},
		{"[1, &x, 3]", "[1, 3]", false},
		{"{a = &x}", "{a = b}", true},
		{"{&x = &y}", "{a = b}", true},
		{"{&x = &x}", "{a = b}", false},
		{"{&x = &x}", "{a = a}", true},
		{
			expr: `{
  &x = &y
  &y = &x
}`,
			src: `{
  a = b
  b = a
}`,
			wantMatch: true},

		{"sort(&x)", "sort(a)", true},

		{"{for k, v in map: &k => upper(&v)}", "{for k, v in map: k => upper(v)}", true},
		{"{for &k, &v in map: &k => upper(&v)}", "{for k, v in map: k => upper(v)}", true},

		{"foo[&x]", "foo[a]", true},
		{"foo[&x]", "foo[1]", false}, // This is due to the key of the traverser index is a cty.Value, which is not either a string or an ast node.
		{"&a[count.index]", "var.subnet_ids[count.index]", true},

		{"&x()", "sort()", true},
		{"&x(&y)", "sort([1,2,3])", true},
		{"&x([])", "sort([1,2,3])", false},
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
