package tui

import (
	"errors"
	"runtime"
	"testing"
)

func TestRexBackgroundHint(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Rex hint is macOS only")
	}
	errMissing := errors.New("does not exist")
	cases := []struct {
		name       string
		appearance string
		style      string
		styleErr   error
		wantDark   bool
	}{
		{name: "explicit dark", appearance: "dark", wantDark: true},
		{name: "explicit light", appearance: "light", style: "Dark", wantDark: false},
		{name: "system dark", appearance: "system", style: "Dark", wantDark: true},
		{name: "system light", appearance: "system", styleErr: errMissing, wantDark: false},
		{name: "unset follows system", appearance: "", style: "Dark", wantDark: true},
	}
	orig := readDefaults
	t.Cleanup(func() { readDefaults = orig })
	t.Setenv("TERM_PROGRAM", "rex")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			readDefaults = func(args ...string) (string, error) {
				if args[0] == "-g" {
					return tc.style, tc.styleErr
				}
				return tc.appearance, nil
			}
			dark, ok := rexBackgroundHint()
			if !ok || dark != tc.wantDark {
				t.Fatalf("rexBackgroundHint() = (%v, %v), want (%v, true)", dark, ok, tc.wantDark)
			}
		})
	}
}

func TestRexBackgroundHintOutsideRex(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	if _, ok := rexBackgroundHint(); ok {
		t.Fatal("hint must not apply outside Rex")
	}
}
