package fin128

import "testing"

// The normalization rule is exactly four things - case, spaces, hyphens, underscores - and nothing else. A parser that
// wanted more would be inventing aliases.
func TestNormalize(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Half Up", "halfup"},
		{"HALF-UP", "halfup"},
		{"half_up", "halfup"},
		{"  half up  ", "halfup"},
		{"ACT/365F", "act/365f"},
		{"30/360", "30/360"},
		{"", ""},
	} {
		if got := normalize(tc.in); got != tc.want {
			t.Errorf("normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
