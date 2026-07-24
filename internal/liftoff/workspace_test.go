package liftoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEditorDefsIncludeHangarBundleVariants(t *testing.T) {
	var hangar *EditorCandidate
	for _, candidate := range editorDefs() {
		if candidate.Binary == "hangar" {
			candidate := candidate
			hangar = &candidate
			break
		}
	}
	if hangar == nil {
		t.Fatal("Hangar editor candidate is missing")
	}
	if hangar.Name != "Hangar" || hangar.App != "Hangar Dev.app" || !hangar.PreferApp {
		t.Fatalf("unexpected Hangar candidate: %+v", *hangar)
	}
	if len(hangar.AlternateApps) != 1 || hangar.AlternateApps[0] != "Hangar.app" {
		t.Fatalf("Hangar stable bundle fallback = %v", hangar.AlternateApps)
	}
}

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

func TestPreferredEditorEnvOverridesConfig(t *testing.T) {
	setStateDir(t)
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.Settings.Editor = "false"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	if got, source := preferredEditor(); got != "false" || source != "from config.toml" {
		t.Fatalf("config preference = %q (%s)", got, source)
	}
	t.Setenv("KIT_EDITOR", "sh")
	if got, source := preferredEditor(); got != "sh" || source != "from $KIT_EDITOR" {
		t.Fatalf("env preference = %q (%s)", got, source)
	}
}

func TestPromoteKnownEditorDoesNotDuplicate(t *testing.T) {
	defs := promoteEditor(editorDefs(), "hangar", "from config.toml")
	count := 0
	for _, candidate := range defs {
		if candidate.Binary == "hangar" {
			count++
		}
	}
	if count != 1 || defs[0].Binary != "hangar" {
		t.Fatalf("promoted defs contain %d Hangar entries: %+v", count, defs)
	}
}

func TestInstalledEditorsPromotesConfigEditor(t *testing.T) {
	setStateDir(t)
	c, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.Settings.Editor = "sh"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KIT_EDITOR", "")

	editors := InstalledEditors()
	if len(editors) == 0 || editors[0].Binary != "sh" {
		t.Fatalf("config editor should be promoted to front, got %+v", editors)
	}
}

func TestInstalledEditorsDetectsKnownCLIWithoutApp(t *testing.T) {
	setStateDir(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "hangar")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	for _, editor := range InstalledEditors() {
		if editor.Binary == "hangar" {
			// A locally installed app may make this use open on developer Macs;
			// either way, the known CLI must make Hangar discoverable.
			return
		}
	}
	t.Fatal("known Hangar CLI was not detected")
}

func TestTouchLastUsedNameMasterIsNoop(t *testing.T) {
	// master has no config entry; this must not panic or error.
	TouchLastUsedName("master")
}
