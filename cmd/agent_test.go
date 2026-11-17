package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func executeAgent(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newAgentCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestAgentRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"agent", "install"})
	if err != nil || cmd.CommandPath() != "kit agent install" {
		t.Fatalf("agent install not discoverable: %v, %v", cmd, err)
	}
}

func TestAgentPrintsEmbeddedSkillWithoutSetupOrSideEffects(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := &cobra.Command{
		Use: "kit",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("setup/update hook must not run")
		},
	}
	root.AddCommand(newAgentCommand())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"agent"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), agentSkill) || !strings.HasPrefix(out.String(), "---\nname: kit-kickoff\n") {
		t.Fatalf("expected raw embedded SKILL.md, got %q", out.String())
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("printing created files: %v, %v", entries, err)
	}
}

func TestAgentInstallDefaultUsesTemporaryHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	out, err := executeAgent(t, "install")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".fx", "skills", "kit-kickoff", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, agentSkill) {
		t.Fatalf("default skill missing or wrong: %q, %v", data, err)
	}
	if !strings.Contains(out, path) {
		t.Fatalf("install did not report the path: %q", out)
	}
}

func TestAgentInstallExplicitPathIsCompleteSkillDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := t.TempDir()
	t.Chdir(cwd)
	dir := filepath.Join("client", "skills", "custom-kickoff")
	out, err := executeAgent(t, "install", "--path", dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cwd, dir, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, agentSkill) || !strings.Contains(out, path) {
		t.Fatalf("explicit install: %q, %q, %v", data, out, err)
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("explicit path touched default home: %v, %v", entries, err)
	}
}

func TestAgentInstallRefusesDifferentContentWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	original := []byte("user-owned instructions\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := executeAgent(t, "install", "--path", dir)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected actionable overwrite refusal, got %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("existing skill changed: %q, %v", data, err)
	}
}

func TestAgentInstallIdenticalContentDoesNotRewrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, agentSkill, 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Unix(123456789, 0)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, force := range []bool{false, true} {
		args := []string{"install", "--path", dir}
		if force {
			args = append(args, "--force")
		}
		if _, err := executeAgent(t, args...); err != nil {
			t.Fatal(err)
		}
		after, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) || before.Mode() != after.Mode() {
			t.Fatalf("identical skill rewritten (force=%v): before=%v, after=%v", force, before, after)
		}
	}
}

func TestAgentInstallForceReplacesOnlySkill(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("older instructions"), 0644); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAgent(t, "install", "--path", dir, "--force"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, agentSkill) {
		t.Fatalf("forced install failed: %q, %v", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("install left temporary files: %v, %v", entries, err)
	}
	data, err = os.ReadFile(sentinel)
	if err != nil || string(data) != "keep" {
		t.Fatalf("unrelated file changed: %q, %v", data, err)
	}
}

func TestAgentInstallForceDoesNotFollowFileSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "user-owned.md")
	if err := os.WriteFile(target, []byte("keep original"), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAgent(t, "install", "--path", dir); err == nil {
		t.Fatal("different symlink content must require --force")
	}
	if _, err := executeAgent(t, "install", "--path", dir, "--force"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep original" {
		t.Fatalf("symlink target overwritten: %q, %v", data, err)
	}
	data, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(data, agentSkill) {
		t.Fatalf("skill entry not replaced: %q, %v", data, err)
	}
}

func TestAgentRejectsInvalidArgumentsWithoutInstalling(t *testing.T) {
	for _, args := range [][]string{{"extra"}, {"install", "extra"}, {"install", "--path", ""}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if _, err := executeAgent(t, args...); err == nil {
				t.Fatalf("expected invalid arguments: %v", args)
			}
			entries, err := os.ReadDir(home)
			if err != nil || len(entries) != 0 {
				t.Fatalf("invalid invocation installed files: %v, %v", entries, err)
			}
		})
	}
}

func TestAgentInstallReportsInvalidDestination(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "SKILL.md"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAgent(t, "install", "--path", dir, "--force"); err == nil {
		t.Fatal("must not overwrite an existing directory")
	}
}
