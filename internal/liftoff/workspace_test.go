package liftoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoneEditor(t *testing.T) {
	zed := EditorCandidate{Name: "Zed", Binary: "zed", Installed: true}
	cursor := EditorCandidate{Name: "Cursor", Binary: "cursor", Installed: true}
	ghostty := EditorCandidate{Name: "Ghostty", Binary: WorkspaceSentinel, Installed: true}
	herdr := EditorCandidate{Name: "herdr", Binary: HerdrSentinel, Installed: true}
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
		{"editor + herdr", []EditorCandidate{zed, herdr}, ""},
		{"only herdr", []EditorCandidate{herdr}, ""},
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

func TestHerdrServerUp(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"running", "status: running\n", true},
		{"not running", "status: not running\n", false},
		{"empty", "", false},
		{"garbage", "garbage", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := herdrServerUp(tc.in); got != tc.want {
				t.Fatalf("herdrServerUp(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// installFakeHerdr writes a shell-script `herdr` into a temp dir prepended to
// PATH, so herdr-detecting code sees it without a real install. The fake logs
// every invocation's args to the returned log path and answers `status server`
// with the given status line. Skips on Windows (the fake is a shell script).
func installFakeHerdr(t *testing.T, statusLine string) (logPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake herdr is a shell script")
	}
	tmp := t.TempDir()
	logPath = filepath.Join(tmp, "invocations.log")
	script := "#!/bin/sh\necho \"$@\" >> " + logPath + "\necho \"" + statusLine + "\"\n"
	if err := os.WriteFile(filepath.Join(tmp, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestOpenHerdrServerDownReturnsHint(t *testing.T) {
	logPath := installFakeHerdr(t, "status: not running")
	err := OpenHerdr("notebook", t.TempDir())
	if err == nil {
		t.Fatal("OpenHerdr should error when the server is down")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("error should mention the server is not running, got %q", err)
	}
	// The precheck failed, so `herdr workspace create` must never have run.
	if log, _ := os.ReadFile(logPath); strings.Contains(string(log), "workspace") {
		t.Fatalf("herdr workspace create was invoked despite the down server:\n%s", log)
	}
}

func TestInstalledEditorsIncludesHerdrWhenOnPath(t *testing.T) {
	hasHerdr := func(eds []EditorCandidate) bool {
		for _, e := range eds {
			if e.Binary == HerdrSentinel {
				return true
			}
		}
		return false
	}

	installFakeHerdr(t, "status: running")
	if !hasHerdr(InstalledEditors()) {
		t.Fatal("herdr on PATH should yield a HerdrSentinel candidate")
	}

	// With herdr off PATH the candidate must not appear (herdr is opt-in).
	t.Setenv("PATH", t.TempDir())
	if hasHerdr(InstalledEditors()) {
		t.Fatal("herdr off PATH should not yield a HerdrSentinel candidate")
	}
}
