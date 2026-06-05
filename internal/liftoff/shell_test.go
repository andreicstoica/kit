package liftoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShellProfilePathByShell(t *testing.T) {
	home, _ := os.UserHomeDir()
	cases := []struct {
		shell string
		want  string
	}{
		{"/bin/bash", filepath.Join(home, ".bash_profile")},
		{"/opt/homebrew/bin/bash", filepath.Join(home, ".bash_profile")},
		{"/bin/zsh", filepath.Join(home, ".zshrc")},
		{"/usr/bin/fish", filepath.Join(home, ".zshrc")}, // unknown → zsh fallback
		{"", filepath.Join(home, ".zshrc")},              // unset → zsh fallback
	}
	for _, c := range cases {
		t.Setenv("SHELL", c.shell)
		if got := ShellProfilePath(); got != c.want {
			t.Errorf("SHELL=%q: ShellProfilePath() = %q, want %q", c.shell, got, c.want)
		}
	}
}

func TestShellName(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	if got := ShellName(); got != "bash" {
		t.Errorf("ShellName() = %q, want bash", got)
	}
	t.Setenv("SHELL", "/weird/tcsh")
	if got := ShellName(); got != "zsh" {
		t.Errorf("unknown shell should fall back to zsh, got %q", got)
	}
}

func TestDirOnPath(t *testing.T) {
	dir := "/some/kit/bin"
	t.Setenv("PATH", "/usr/bin:"+dir+":/bin")
	if !DirOnPath(dir) {
		t.Errorf("DirOnPath should find %q in PATH", dir)
	}
	// trailing slash on PATH entry must still match
	t.Setenv("PATH", "/usr/bin:"+dir+"/:/bin")
	if !DirOnPath(dir) {
		t.Errorf("DirOnPath should match despite trailing slash")
	}
	t.Setenv("PATH", "/usr/bin:/bin")
	if DirOnPath(dir) {
		t.Errorf("DirOnPath should not find %q absent from PATH", dir)
	}
}

func TestPathExportLine(t *testing.T) {
	got := PathExportLine("/home/x/.local/bin")
	want := `export PATH="/home/x/.local/bin:$PATH"`
	if got != want {
		t.Errorf("PathExportLine() = %q, want %q", got, want)
	}
}
