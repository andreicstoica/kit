package liftoff

import (
	"os/exec"
	"testing"
)

func TestLoneEditor(t *testing.T) {
	zed := EditorCandidate{Name: "Zed", Binary: "zed", Installed: true}
	cursor := EditorCandidate{Name: "Cursor", Binary: "cursor", Installed: true}
	ghostty := EditorCandidate{Name: "Ghostty", Binary: WorkspaceSentinel, Installed: true}
	skip := EditorCandidate{Name: "Skip", Binary: SkipSentinel, Installed: true}

	cases := []struct {
		name string
		in   []EditorCandidate
		want string // expected sole editor binary, "" = nil
	}{
		{"single editor", []EditorCandidate{zed}, "zed"},
		{"single editor + skip", []EditorCandidate{zed, skip}, "zed"},
		{"two editors", []EditorCandidate{zed, cursor}, ""},
		{"editor + ghostty", []EditorCandidate{zed, ghostty}, ""},
		{"none", nil, ""},
		{"only ghostty", []EditorCandidate{ghostty}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LoneEditor(tc.in)
			if tc.want == "" {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil || got.Binary != tc.want {
				t.Fatalf("want %q, got %+v", tc.want, got)
			}
		})
	}
}

func TestResolveEditorUnknownReturnsNil(t *testing.T) {
	if c := ResolveEditor("definitely-not-an-editor-xyz"); c != nil {
		t.Fatalf("unknown editor should resolve to nil, got %+v", c)
	}
}

func TestInstalledEditorsPromotesKitEditor(t *testing.T) {
	// Point KIT_EDITOR at a binary guaranteed on PATH so it's detected.
	bin := "sh"
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("%s not on PATH", bin)
	}
	t.Setenv("KIT_EDITOR", bin)
	eds := InstalledEditors()
	if len(eds) == 0 || eds[0].Binary != bin {
		t.Fatalf("KIT_EDITOR should be promoted to front, got %+v", eds)
	}
}

func TestTouchLastUsedNameMasterIsNoop(t *testing.T) {
	// master has no config entry; this must not panic or error.
	TouchLastUsedName("master")
}
