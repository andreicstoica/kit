package liftoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestViteCacheDir(t *testing.T) {
	wt := "/home/u/liftoff/feat"
	cases := []struct {
		svc  Service
		want string
	}{
		{SvcApp, "/home/u/liftoff/feat/frontend/app/node_modules/.vite"},
		{SvcAdmin, "/home/u/liftoff/feat/frontend/admin/node_modules/.vite"},
		{SvcAPI, ""},
		{SvcCelery, ""},
		{SvcBeat, ""},
	}
	for _, c := range cases {
		if got := ViteCacheDir(wt, c.svc); got != c.want {
			t.Errorf("ViteCacheDir(%s) = %q, want %q", c.svc, got, c.want)
		}
	}
}

func TestClearViteCache_Backend(t *testing.T) {
	cleared, err := ClearViteCache(t.TempDir(), SvcAPI)
	if err != nil {
		t.Fatal(err)
	}
	if cleared != "" {
		t.Errorf("backend service should clear nothing, got %q", cleared)
	}
}

func TestClearViteCache_MissingDirIsNoop(t *testing.T) {
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, "frontend/app/node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No .vite present.
	cleared, err := ClearViteCache(wt, SvcApp)
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if cleared != "" {
		t.Errorf("nothing to clear, got %q", cleared)
	}
}

func TestClearViteCache_RemovesExisting(t *testing.T) {
	wt := t.TempDir()
	cache := filepath.Join(wt, "frontend/app/node_modules", ".vite", "deps")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "stale.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleared, err := ClearViteCache(wt, SvcApp)
	if err != nil {
		t.Fatal(err)
	}
	if cleared == "" {
		t.Fatal("expected a cleared path")
	}
	if _, err := os.Stat(cleared); !os.IsNotExist(err) {
		t.Errorf(".vite still present after clear: %v", err)
	}
	// node_modules itself must survive.
	if _, err := os.Stat(filepath.Join(wt, "frontend/app/node_modules")); err != nil {
		t.Errorf("node_modules should survive: %v", err)
	}
}

// node_modules is symlinked to master, so clearing must follow the link and
// remove the real .vite in master while leaving the symlink and master's other
// deps intact.
func TestClearViteCache_FollowsSymlinkToMaster(t *testing.T) {
	root := t.TempDir()
	master := filepath.Join(root, "master", "frontend/app", "node_modules")
	if err := os.MkdirAll(filepath.Join(master, ".vite", "deps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(master, "react"), 0o755); err != nil {
		t.Fatal(err)
	}

	wt := filepath.Join(root, "wt")
	if err := os.MkdirAll(filepath.Join(wt, "frontend/app"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(wt, "frontend/app", "node_modules")
	if err := os.Symlink(master, link); err != nil {
		t.Fatal(err)
	}

	if _, err := ClearViteCache(wt, SvcApp); err != nil {
		t.Fatal(err)
	}
	// Real .vite in master gone.
	if _, err := os.Stat(filepath.Join(master, ".vite")); !os.IsNotExist(err) {
		t.Errorf("master .vite should be gone: %v", err)
	}
	// Symlink and sibling dep intact.
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink should survive intact: fi=%v err=%v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(master, "react")); err != nil {
		t.Errorf("sibling dep react should survive: %v", err)
	}
}
