package liftoff

import "testing"

func TestOpenTargetKind(t *testing.T) {
	cases := []struct {
		name string
		c    EditorCandidate
		want OpenTargetKind
	}{
		{"editor", EditorCandidate{Binary: "zed"}, OpenTargetEditor},
		{"ghostty", WorkspaceCandidate(), OpenTargetGhosttyWorkspace},
		{"herdr", HerdrCandidate(), OpenTargetHerdr},
		{"skip", SkipCandidate(), OpenTargetSkip},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.Kind(); got != tc.want {
				t.Fatalf("Kind() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOpenCandidatesIncludesHerdrWhenInstalled(t *testing.T) {
	cands := OpenCandidates()
	if !HerdrAvailable() {
		for _, c := range cands {
			if c.Kind() == OpenTargetHerdr {
				t.Fatal("Herdr should not appear when the CLI is missing")
			}
		}
		return
	}
	found := false
	for _, c := range cands {
		if c.Kind() == OpenTargetHerdr {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("OpenCandidates should include Herdr when the CLI is installed")
	}
}

func TestLoneEditor(t *testing.T) {
	zed := EditorCandidate{Name: "Zed", Binary: "zed", Installed: true}
	cursor := EditorCandidate{Name: "Cursor", Binary: "cursor", Installed: true}
	ghostty := WorkspaceCandidate()
	herdr := HerdrCandidate()
	skip := SkipCandidate()

	cases := []struct {
		name string
		in   []EditorCandidate
		want string // expected sole editor binary, "" = nil
	}{
		{"single editor", []EditorCandidate{zed}, "zed"},
		{"single editor + skip", []EditorCandidate{zed, skip}, "zed"},
		{"two editors", []EditorCandidate{zed, cursor}, ""},
		{"editor + ghostty", []EditorCandidate{zed, ghostty}, ""},
		{"editor + herdr", []EditorCandidate{zed, herdr}, ""},
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

func TestPickerCandidatesIncludesSkip(t *testing.T) {
	cands := PickerCandidates(true)
	found := false
	for _, c := range cands {
		if c.Kind() == OpenTargetSkip {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("PickerCandidates(true) should append skip")
	}
}

func TestPickerCandidatesOmitsSkip(t *testing.T) {
	for _, c := range PickerCandidates(false) {
		if c.Kind() == OpenTargetSkip {
			t.Fatal("PickerCandidates(false) should not include skip")
		}
	}
}
