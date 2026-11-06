package liftoff

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestIdleCodeFingerprintDetectsRepeatedEditsToDirtyFile(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "--quiet")
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("base"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "file.txt")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "baseline")
	if err := os.WriteFile(file, []byte("edit"), 0600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(100, 0)
	if err := os.Chtimes(file, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	first, err := idleCodeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("next"), 0600); err != nil {
		t.Fatal(err)
	}
	stamp = stamp.Add(time.Second)
	if err := os.Chtimes(file, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	second, err := idleCodeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("second edit to already-dirty file did not reset the fingerprint")
	}
	stable, err := idleCodeFingerprint(dir)
	if err != nil || stable != second {
		t.Fatalf("unchanged worktree is not stable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := idleCodeFingerprint(dir)
	if err != nil || third == second {
		t.Fatalf("untracked file was missed: %v", err)
	}
}
