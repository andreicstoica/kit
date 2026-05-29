package cmd

import "testing"

func TestSemverNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.1.7", "v0.1.6", true},  // genuine update
		{"v0.1.5", "v0.1.6", false}, // stale cache behind us — the reported bug
		{"v0.1.6", "v0.1.6", false}, // equal
		{"v0.2.0", "v0.1.9", true},  // minor bump
		{"v1.0.0", "v0.9.9", true},  // major bump
		{"v0.1.10", "v0.1.9", true},   // numeric, not lexical
		{"v0.1.6.1", "v0.1.6", true},  // 4-part hotfix > 3-part base
		{"v0.1.6", "v0.1.6.1", false}, // base < its hotfix
		{"v0.1.6.2", "v0.1.6.1", true},
		{"", "v0.1.6", false}, // empty cache → no nag
		{"garbage", "v0.1.6", false},
	}
	for _, c := range cases {
		if got := semverNewer(c.a, c.b); got != c.want {
			t.Errorf("semverNewer(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
