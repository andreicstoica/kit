package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

func enterTestLayout(t *testing.T) liftoff.Layout {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("KIT_ROOT", filepath.Join(root, "kits"))
	t.Setenv("KIT_MASTER_DIR", "main-checkout")
	t.Setenv("KIT_STATE_DIR", filepath.Join(root, "state"))
	t.Setenv("KIT_RUN_DIR", filepath.Join(root, "run"))
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	bin := t.TempDir()
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	return liftoff.DefaultLayout()
}

func addEnterCheckout(t *testing.T, layout liftoff.Layout, name, path string) {
	t.Helper()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-b", "master", layout.Master)
	git("-C", layout.Master, "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-m", "init")
	if path != layout.Master {
		git("-C", layout.Master, "worktree", "add", "-b", name, path)
	}
}

func enterTestBinary(t *testing.T, name, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func TestEnterSessionIDValidation(t *testing.T) {
	for _, id := range []string{"123e4567-e89b-12d3-a456-426614174000", "abc_123-DEF"} {
		if err := validateFXSessionID(id); err != nil {
			t.Errorf("valid session %q: %v", id, err)
		}
	}
	for _, id := range []string{"", "last", "--help", "-c", "_id", "a b", "a\nb", "a\r", "a\x00b", "a;b", "a'", "a/child", "$(id)", "é"} {
		t.Run(fmt.Sprintf("%q", id), func(t *testing.T) {
			if err := validateFXSessionID(id); err == nil {
				t.Fatalf("accepted invalid session %q", id)
			}
		})
	}
}

func TestEnterRequiresExplicitSession(t *testing.T) {
	for _, args := range [][]string{
		{"feature", "--print"},
		{"feature", "--session=", "--print"},
		{"feature", "--session=--help", "--print"},
		{"feature", "--session=last", "--print"},
	} {
		cmd := newEnterCmd(func() bool { return false }, nil)
		cmd.SilenceErrors, cmd.SilenceUsage = true, true
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "session") {
			t.Errorf("args %q: want session error, got %v", args, err)
		}
	}
}

func TestEnterPrintResolvesWorktree(t *testing.T) {
	for _, kind := range []string{"canonical", "legacy", "master", "moved"} {
		t.Run(kind, func(t *testing.T) {
			layout := enterTestLayout(t)
			name, path := "feature", layout.WorktreePath("feature")
			switch kind {
			case "legacy":
				path = layout.LegacyWorktreePath(name)
			case "master":
				name, path = "master", layout.Master
			case "moved":
				path = filepath.Join(t.TempDir(), "moved-checkout")
				cfg := &liftoff.Config{Worktrees: map[string]liftoff.WorktreeMeta{name: {Path: path}}}
				if err := cfg.Save(); err != nil {
					t.Fatal(err)
				}
			}
			addEnterCheckout(t, layout, name, path)
			var out bytes.Buffer
			cmd := newEnterCmd(func() bool { t.Fatal("--print checked terminal"); return false }, nil)
			cmd.SetOut(&out)
			cmd.SetArgs([]string{name, "--session", "saved-id", "--print"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("cd -- '%s' && fx --resume 'saved-id'\n", path)
			if out.String() != want {
				t.Fatalf("output = %q, want %q", out.String(), want)
			}
		})
	}
}

func TestEnterUsesRecordedCheckoutOverCanonicalReplacement(t *testing.T) {
	layout := enterTestLayout(t)
	moved := filepath.Join(layout.Root, "moved-checkout")
	addEnterCheckout(t, layout, "feature", moved)
	canonical := layout.WorktreePath("feature")
	if out, err := exec.Command("git", "-C", layout.Master, "worktree", "add", "-b", "unrelated", canonical).CombinedOutput(); err != nil {
		t.Fatalf("add replacement: %v: %s", err, out)
	}
	cfg := &liftoff.Config{Worktrees: map[string]liftoff.WorktreeMeta{"feature": {Path: moved, Branch: "feature"}}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := newEnterCmd(func() bool { return false }, nil)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"feature", "--session", "saved-id", "--print"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("cd -- '%s' && fx --resume 'saved-id'\n", moved); out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestEnterPrintFromCwdAndGitTrackedPath(t *testing.T) {
	layout := enterTestLayout(t)
	path := filepath.Join(filepath.Dir(layout.Root), "tracked-feature")
	addEnterCheckout(t, layout, "tracked-feature", path)
	subdir := filepath.Join(path, "nested")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(subdir)
	var out bytes.Buffer
	cmd := newEnterCmd(func() bool { return false }, nil)
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--session", "saved-id", "--print"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("cd -- '%s' && fx --resume 'saved-id'\n", path); out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestEnterPrintIsReadOnlyAndShellQuoted(t *testing.T) {
	layout := enterTestLayout(t)
	path := filepath.Join(layout.Root, "root's $(touch injected); directory", "feature")
	t.Setenv("KIT_ROOT", filepath.Dir(path))
	layout = liftoff.DefaultLayout()
	addEnterCheckout(t, layout, "feature", path)
	beforeCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "fx-called")
	t.Setenv("ENTER_TEST_FX_MARKER", marker)
	enterTestBinary(t, "fx", "printf called > \"$ENTER_TEST_FX_MARKER\"\nprintf '%s\\n' \"$PWD\" \"$@\"\n")
	var out bytes.Buffer
	cmd := newEnterCmd(func() bool { return false }, func(string, []string, []string) error {
		t.Fatal("--print tried to replace the process")
		return nil
	})
	cmd.SetOut(&out)
	root := &cobra.Command{Use: "kit", PersistentPreRunE: func(*cobra.Command, []string) error {
		t.Fatal("--print ran a potentially mutating root hook")
		return nil
	}}
	root.AddCommand(cmd)
	root.SetArgs([]string{"enter", "feature", "--session", "saved-id", "--print"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	afterCwd, err := os.Getwd()
	if err != nil || afterCwd != beforeCwd {
		t.Fatalf("cwd changed: %q -> %q, %v", beforeCwd, afterCwd, err)
	}
	for _, file := range []string{marker, os.Getenv("KIT_STATE_DIR"), os.Getenv("KIT_RUN_DIR")} {
		if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("--print mutated %s: %v", file, err)
		}
	}

	shell := exec.Command("/bin/sh", "-c", out.String())
	shell.Dir = t.TempDir()
	got, err := shell.CombinedOutput()
	if err != nil {
		t.Fatalf("printed command failed: %v\n%s", err, got)
	}
	if want := path + "\n--resume\nsaved-id\n"; string(got) != want {
		t.Fatalf("printed command = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(shell.Dir, "injected")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shell injection created a file: %v", err)
	}
}

func TestEnterRejectsNoninteractiveResume(t *testing.T) {
	cmd := newEnterCmd(func() bool { return false }, func(string, []string, []string) error {
		t.Fatal("noninteractive enter attempted exec")
		return nil
	})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"feature", "--session", "saved-id"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--print") {
		t.Fatalf("want noninteractive --print guidance, got %v", err)
	}
}

func TestEnterRejectsInvalidDestination(t *testing.T) {
	for _, kind := range []string{"missing", "file", "master-missing", "no-name", "invalid-name", "extra-name"} {
		t.Run(kind, func(t *testing.T) {
			layout := enterTestLayout(t)
			args := []string{"feature"}
			switch kind {
			case "file":
				if err := os.MkdirAll(layout.Root, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(layout.WorktreePath("feature"), []byte("not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "master-missing":
				args = []string{"master"}
			case "no-name":
				args = nil
			case "invalid-name":
				args = []string{"../../outside"}
			case "extra-name":
				args = []string{"feature", "another"}
			}
			cmd := newEnterCmd(func() bool { return false }, nil)
			cmd.SilenceErrors, cmd.SilenceUsage = true, true
			cmd.SetArgs(append(args, "--session", "saved-id", "--print"))
			if err := cmd.Execute(); err == nil {
				t.Fatal("accepted invalid destination")
			}
		})
	}
}

func TestEnterExecUsesDestinationAndExactSession(t *testing.T) {
	layout := enterTestLayout(t)
	path := layout.WorktreePath("feature")
	addEnterCheckout(t, layout, "feature", path)
	binary := enterTestBinary(t, "fx", "exit 99\n")
	t.Setenv("PWD", "stale-parent-directory")
	t.Setenv("ENTER_TEST_KEEP", "preserved")
	beforeCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	replacementError := errors.New("injected exec failure")
	called := 0
	cmd := newEnterCmd(func() bool { return true }, func(gotBinary string, args, env []string) error {
		called++
		if gotBinary != binary || !reflect.DeepEqual(args, []string{"fx", "--resume", "saved-id"}) {
			t.Errorf("exec = %q %q", gotBinary, args)
		}
		cwd, err := os.Getwd()
		if err != nil || cwd != path {
			t.Errorf("exec cwd = %q, %v; want %q", cwd, err, path)
		}
		pwdCount, kept := 0, false
		for _, entry := range env {
			if strings.HasPrefix(entry, "PWD=") {
				pwdCount++
				if entry != "PWD="+path {
					t.Errorf("exec environment has stale cwd: %s", entry)
				}
			}
			kept = kept || entry == "ENTER_TEST_KEEP=preserved"
		}
		if pwdCount != 1 || !kept {
			t.Errorf("exec environment: PWD count %d, preserved variable %t", pwdCount, kept)
		}
		return replacementError
	})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"feature", "--session", "saved-id"})
	if err := cmd.Execute(); !errors.Is(err, replacementError) {
		t.Fatalf("want exec error, got %v", err)
	}
	if called != 1 {
		t.Fatalf("exec called %d times, want once", called)
	}
	afterCwd, err := os.Getwd()
	if err != nil || afterCwd != beforeCwd {
		t.Fatalf("exec failure did not restore cwd: %q -> %q, %v", beforeCwd, afterCwd, err)
	}
	if os.Getenv("PWD") != "stale-parent-directory" {
		t.Fatal("exec failure mutated the parent environment")
	}
}

func TestEnterMissingFXDoesNotChangeCwd(t *testing.T) {
	layout := enterTestLayout(t)
	addEnterCheckout(t, layout, "feature", layout.WorktreePath("feature"))
	beforeCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := newEnterCmd(func() bool { return true }, func(string, []string, []string) error {
		t.Fatal("missing fx attempted exec")
		return nil
	})
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"feature", "--session", "saved-id"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "find fx on PATH") {
		t.Fatalf("want missing fx error, got %v", err)
	}
	afterCwd, err := os.Getwd()
	if err != nil || afterCwd != beforeCwd {
		t.Fatalf("missing fx changed cwd: %q -> %q, %v", beforeCwd, afterCwd, err)
	}
}
