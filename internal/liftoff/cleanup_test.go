package liftoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRex writes a `rex` stub that serves one session (id s1) and records
// kills to capturePath. Returns the temp bin dir to prepend to PATH.
func fakeRex(t *testing.T, capturePath string) string {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "rex"), `#!/bin/sh
# The real CLI accepts global flags before the subcommand; skip them.
while [ "${1#--}" != "$1" ]; do shift; done
if [ "$1" = "do" ]; then
    printf '{"sessions":[{"session_id":"s1","label":"stub","windows":[]}]}'
    exit 0
fi
if [ "$1" = "kill" ]; then
    printf '%s\n' "$2" >> "$CAPTURE"
    exit 0
fi
exit 1
`)
	t.Setenv("CAPTURE", capturePath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
}

func fakeDropdb(t *testing.T, capturePath string) {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "dropdb"), "#!/bin/sh\ntest \"$1\" = \"--if-exists\" || exit 1\nprintf '%s\\n' \"$2\" >> \"$CAPTURE\"\n")
	t.Setenv("CAPTURE", capturePath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func readCapture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatal(err)
	}
	return string(data)
}

// Wash must clean the Rex backend too: a saved RexID authorizes Rex cleanup
// even though Herdr is untouched/absent.
func TestRunWash_CleansSavedRexWorkspace(t *testing.T) {
	l := newMasterRepo(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	capture := filepath.Join(t.TempDir(), "killed")
	fakeRex(t, capture)
	path := addWorktree(t, l, "demo")
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["demo"] = WorktreeMeta{Slot: 4, Path: path, Branch: "demo", RexID: "s1", RexLayout: "default"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := l.RunWashBlocking(WashPlan{Name: "demo", Branch: "demo", WorktreePath: path}); err != nil {
		t.Fatalf("wash with saved Rex workspace: %v", err)
	}
	if got := readCapture(t, capture); !strings.Contains(got, "s1") {
		t.Fatalf("Rex session was not closed during wash; kills = %q", got)
	}
}

// Wash must not remove a checkout owned by another worktree when the plan
// path does not match the plan identity (ExpectedHead is unset in the TUI
// flow, so this guard is the last line of defense).
func TestRunWash_RefusesForeignWorktreePath(t *testing.T) {
	l := newMasterRepo(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	pathA := addWorktree(t, l, "keep-a")
	pathB := addWorktree(t, l, "keep-b")
	writeFile(t, pathB, "precious.txt", "do not delete")
	if err := l.RunWashBlocking(WashPlan{Name: "keep-a", Branch: "keep-a", WorktreePath: pathB}); err == nil {
		t.Fatal("wash accepted a worktree path owned by another worktree")
	}
	_ = pathA
	if _, err := os.Stat(filepath.Join(pathB, "precious.txt")); err != nil {
		t.Fatalf("foreign checkout was removed: %v", err)
	}
	if got := runGit(t, l.Master, "rev-parse", "--verify", "keep-b"); got == "" {
		t.Fatal("foreign branch was deleted")
	}
}

// DBName collides for "a-b" vs "a_b" (both liftoff_a_b). A drop authorized
// only by a name guess must be refused when another record owns the name.
func TestOwnedDBName_RefusesCollision(t *testing.T) {
	setStateDir(t)
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["a_b"] = WorktreeMeta{Slot: 1, DatabaseName: "liftoff_a_b"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveCleanupDB("a-b", ""); err == nil {
		t.Fatal("guessed DB name owned by another worktree was accepted")
	} else if !strings.Contains(err.Error(), "a_b") {
		t.Fatalf("ownership error should name the owning worktree, got: %v", err)
	}
	if db, skip, err := resolveCleanupDB("a_b", ""); err != nil || skip || db != "liftoff_a_b" {
		t.Fatalf("resolveCleanupDB(a_b) = %q, skip=%v, %v; want liftoff_a_b", db, skip, err)
	}
}

func TestResolveCleanupDB_RefusesUnprovenGuess(t *testing.T) {
	setStateDir(t)
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["plain"] = WorktreeMeta{Slot: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveCleanupDB("plain", ""); err == nil {
		t.Fatal("missing checkout/env proof must refuse guessed database")
	}
}

func TestResolveCleanupDB_RefusesSharedDatabase(t *testing.T) {
	setStateDir(t)
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["bad"] = WorktreeMeta{DatabaseName: "liftoff"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveCleanupDB("bad", ""); err == nil {
		t.Fatal("corrupt record must not authorize dropping shared liftoff database")
	}
}

// A persisted DatabaseName is exact: it wins over the derived guess, but a
// second record claiming the same database still blocks the drop.
func TestResolveCleanupDB_PersistedName(t *testing.T) {
	setStateDir(t)
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["custom"] = WorktreeMeta{Slot: 1, DatabaseName: "liftoff_special"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if db, skip, err := resolveCleanupDB("custom", ""); err != nil || skip || db != "liftoff_special" {
		t.Fatalf("persisted name should win, got %q skip=%v err=%v", db, skip, err)
	}
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["squatter"] = WorktreeMeta{Slot: 2, DatabaseName: "liftoff_special"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveCleanupDB("custom", ""); err == nil {
		t.Fatal("doubly-claimed exact database name was accepted")
	}
}

// Legacy proof: when the checkout's env points at another database (e.g. it
// never cloned one), a derived-name drop is skipped, not forced.
func TestResolveCleanupDB_EnvMismatchSkips(t *testing.T) {
	setStateDir(t)
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "backend", ".env"), []byte("SQLALCHEMY_DATABASE_NAME=liftoff\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, skip, err := resolveCleanupDB("plain", wt)
	if err != nil || !skip || db != "" {
		t.Fatalf("env pointing elsewhere should skip, got %q skip=%v err=%v", db, skip, err)
	}
	if err := os.WriteFile(filepath.Join(wt, "backend", ".env"), []byte("SQLALCHEMY_DATABASE_NAME=liftoff_plain\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if db, skip, err := resolveCleanupDB("plain", wt); err != nil || skip || db != "liftoff_plain" {
		t.Fatalf("matching env should authorize, got %q skip=%v err=%v", db, skip, err)
	}
	if db, skip, err := resolveCleanupDB("plain", ""); err != nil || skip || db != "liftoff_plain" {
		t.Fatalf("persisted proof should survive checkout deletion, got %q skip=%v err=%v", db, skip, err)
	}
}

// Wash must not drop a database owned by another worktree, and must retain
// the config record for retry instead of freeing it.
func TestRunWash_DisputedDBRetainsConfig(t *testing.T) {
	l := newMasterRepo(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	capture := filepath.Join(t.TempDir(), "dropped")
	fakeDropdb(t, capture)
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["a_b"] = WorktreeMeta{Slot: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "gone")
	err := l.RunWashBlocking(WashPlan{Name: "a-b", Branch: "a-b", WorktreePath: missing, DropDB: true})
	if err == nil {
		t.Fatal("wash dropped a database owned by another worktree")
	}
	if got := readCapture(t, capture); got != "" {
		t.Fatalf("disputed database was dropped: %q", got)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta, ok := cfg.Worktrees["a-b"]
	if !ok || !meta.CleanupPending {
		t.Fatalf("failed cleanup must retain recovery metadata, got %+v exists=%v", meta, ok)
	}
	if _, ok := cfg.Worktrees["a_b"]; !ok {
		t.Fatal("owning worktree record must be preserved")
	}
}

// Wash must skip (not fail, not drop) when the checkout's env proves it uses
// a different database — e.g. it never cloned one and still points at master.
func TestRunWash_SkipsDropWhenEnvPointsElsewhere(t *testing.T) {
	l := newMasterRepo(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	capture := filepath.Join(t.TempDir(), "dropped")
	fakeDropdb(t, capture)
	path := addWorktree(t, l, "plain")
	if err := os.MkdirAll(filepath.Join(path, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "backend", ".env"), []byte("SQLALCHEMY_DATABASE_NAME=liftoff\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.RunWashBlocking(WashPlan{Name: "plain", Branch: "plain", WorktreePath: path, DropDB: true}); err != nil {
		t.Fatalf("wash with foreign-env database should skip the drop: %v", err)
	}
	if got := readCapture(t, capture); got != "" {
		t.Fatalf("unowned database was dropped: %q", got)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Worktrees["plain"]; ok {
		t.Fatal("successful wash must free the slot")
	}
}

// Reconcile must not drop a database owned by a live worktree, even when the
// orphan's guessed DB name exists.
func TestReconcileOrphan_DisputedDBRetainsConfig(t *testing.T) {
	l := newMasterRepo(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	l.GtabDir = t.TempDir()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "pg_dump"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "psql"), "#!/bin/sh\nprintf 'liftoff_a_b\\n'\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	capture := filepath.Join(t.TempDir(), "dropped")
	fakeDropdb(t, capture)
	livePath := addWorktree(t, l, "a_b")
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["a_b"] = WorktreeMeta{Slot: 1, Path: livePath, Branch: "a_b"}
		c.Worktrees["a-b"] = WorktreeMeta{Slot: 2, Path: filepath.Join(t.TempDir(), "gone"), Branch: "a-b"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	candidates, err := l.FindOrphanedWorktrees()
	if err != nil || len(candidates) != 1 || candidates[0].Name != "a-b" {
		t.Fatalf("orphan candidates = %+v, %v; want only a-b", candidates, err)
	}
	if err := l.ReconcileOrphan(candidates[0], nil); err == nil {
		t.Fatal("reconcile dropped a database owned by a live worktree")
	}
	if got := readCapture(t, capture); got != "" {
		t.Fatalf("disputed database was dropped: %q", got)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Worktrees["a-b"]; !ok {
		t.Fatal("failed reconcile must retain the orphan record for retry")
	}
}

// Reconcile must clean a saved Rex session and succeed without Herdr.
func TestReconcileOrphan_CleansSavedRexWorkspace(t *testing.T) {
	l := newMasterRepo(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	l.GtabDir = t.TempDir()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "pg_dump"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "psql"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	capture := filepath.Join(t.TempDir(), "killed")
	fakeRex(t, capture)
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["orphan-x"] = WorktreeMeta{Slot: 4, Path: filepath.Join(t.TempDir(), "gone"), Branch: "orphan-x", RexID: "s1"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	candidates, err := l.FindOrphanedWorktrees()
	if err != nil || len(candidates) != 1 {
		t.Fatalf("orphan candidates = %+v, %v; want one", candidates, err)
	}
	if !candidates[0].HasRex {
		t.Fatalf("orphan candidate should report the saved Rex session: %+v", candidates[0])
	}
	if err := l.ReconcileOrphan(candidates[0], nil); err != nil {
		t.Fatalf("reconcile with saved Rex workspace: %v", err)
	}
	if got := readCapture(t, capture); !strings.Contains(got, "s1") {
		t.Fatalf("Rex session was not closed during reconcile; kills = %q", got)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Worktrees["orphan-x"]; ok {
		t.Fatal("orphan config record retained after successful cleanup")
	}
}

// A Rex-only mapping authorizes managed-workspace cleanup; an unmapped name
// does not.
func TestHasSavedManagedWorkspace(t *testing.T) {
	setStateDir(t)
	if hasSavedManagedWorkspace("nobody") {
		t.Fatal("unmapped name must not authorize workspace cleanup")
	}
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["rex-only"] = WorktreeMeta{Slot: 1, RexID: "s1"}
		c.Worktrees["herdr-only"] = WorktreeMeta{Slot: 2, HerdrID: "w1"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !hasSavedManagedWorkspace("rex-only") {
		t.Fatal("saved RexID must authorize managed-workspace cleanup")
	}
	if !hasSavedManagedWorkspace("herdr-only") {
		t.Fatal("saved HerdrID must authorize managed-workspace cleanup")
	}
}
