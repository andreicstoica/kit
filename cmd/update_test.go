package cmd

import "testing"

func TestBaseVersion(t *testing.T) {
	cases := map[string]string{
		"v0.1.4-3-gabc":       "v0.1.4",
		"v0.1.4-3-gabc-dirty": "v0.1.4",
		"v0.3.9-rc.1":         "v0.3.9-rc.1",
		"v0.3.9-rc.1-2-gabc":  "v0.3.9-rc.1",
		"v0.3.9":              "v0.3.9",
	}
	for input, want := range cases {
		if got := baseVersion(input); got != want {
			t.Errorf("baseVersion(%q) = %q, want %q", input, got, want)
		}
	}
}
