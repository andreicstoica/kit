package liftoff

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDressBackendInstallOverlapsDatabaseCopy(t *testing.T) {
	l, _ := dressLayout(t)
	if err := os.MkdirAll(filepath.Join(l.Master, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, l.Master, "backend/requirements.txt", "")
	writeFile(t, l.Master, "backend/.env", "SQLALCHEMY_DATABASE_NAME=liftoff\n")
	runGit(t, l.Master, "add", ".")
	runGit(t, l.Master, "commit", "-m", "backend fixture")
	runGit(t, l.Master, "push", "origin", l.MainBranch)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	marker := filepath.Join(t.TempDir(), "backend-installed")
	t.Setenv("BACKEND_MARKER", marker)
	stubDBBins(t, "", `#!/bin/sh
for i in $(seq 1 100); do
  [ -f "$BACKEND_MARKER" ] && exit 0
  sleep 0.02
done
echo 'backend install never ran during DB copy' >&2
exit 1
`, "", filepath.Join(t.TempDir(), "dropped"))
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "uv"), "#!/bin/sh\ntouch \"$BACKEND_MARKER\"\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(filepath.Dir(l.Master), "overlap")
	if err := runDressBlocking(l, DressPlan{Name: "overlap", Worktree: path, CloneDB: true, BackendDeps: true}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worktrees["overlap"].DatabaseName != "liftoff_overlap" || cfg.Worktrees["overlap"].Slot == 0 {
		t.Fatalf("lost ownership or slot: %+v", cfg.Worktrees["overlap"])
	}
}

func runDressBlocking(l Layout, p DressPlan) error {
	var errs []error
	for u := range l.RunDress(p) {
		if u.Status == StepFailed && u.Err != nil {
			errs = append(errs, u.Err)
		}
	}
	return errors.Join(errs...)
}

// stubDBBins prepends fakes for createdb/dropdb/psql/pg_dump. pgDump may be
// nil for a plain success; otherwise it is the script body.
func stubDBBins(t *testing.T, dropdbBody, pgDumpBody string, createdCapture, dropCapture string) {
	t.Helper()
	cloneByTemplate = false // these tests exercise the dump path
	t.Cleanup(func() { cloneByTemplate = true })
	bin := t.TempDir()
	if createdCapture != "" {
		writeExecutable(t, filepath.Join(bin, "createdb"), "#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$CREATE_CAPTURE\"\n")
		t.Setenv("CREATE_CAPTURE", createdCapture)
	} else {
		writeExecutable(t, filepath.Join(bin, "createdb"), "#!/bin/sh\nexit 0\n")
	}
	if dropdbBody == "" {
		dropdbBody = "#!/bin/sh\nprintf '%s\\n' \"$2\" >> \"$DROP_CAPTURE\"\n"
	}
	writeExecutable(t, filepath.Join(bin, "dropdb"), dropdbBody)
	t.Setenv("DROP_CAPTURE", dropCapture)
	if pgDumpBody == "" {
		pgDumpBody = "#!/bin/sh\nexit 0\n"
	}
	writeExecutable(t, filepath.Join(bin, "pg_dump"), pgDumpBody)
	writeExecutable(t, filepath.Join(bin, "psql"), "#!/bin/sh\ncat >/dev/null\nexit 0\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func dressLayout(t *testing.T) (Layout, string) {
	t.Helper()
	l := newMasterRepo(t)
	l.GtabDir = t.TempDir()
	return l, filepath.Join(filepath.Dir(l.Master), "fdb")
}

// A clone failure after a successful createdb must roll everything back:
// the created database is dropped, the checkout and branch are gone, and no
// stray config record remains.
func TestRunDress_FailAfterDB_RollsBackCleanly(t *testing.T) {
	l, _ := dressLayout(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	created := filepath.Join(t.TempDir(), "created")
	dropped := filepath.Join(t.TempDir(), "dropped")
	snapshot := filepath.Join(t.TempDir(), "config-snapshot.toml")
	t.Setenv("SNAPSHOT", snapshot)
	stubDBBins(t, "", `#!/bin/sh
cp "$KIT_STATE_DIR/config.toml" "$SNAPSHOT"
exit 1
`, created, dropped)
	path := filepath.Join(filepath.Dir(l.Master), "fdb-a")
	err := runDressBlocking(l, DressPlan{Name: "fdb-a", Worktree: path, CloneDB: true})
	if err == nil {
		t.Fatal("dress with failing clone should fail")
	}
	if got := readCapture(t, created); !strings.Contains(got, "liftoff_fdb_a") {
		t.Fatalf("createdb was not called for liftoff_fdb_a: %q", got)
	}
	if got := readCapture(t, dropped); !strings.Contains(got, "liftoff_fdb_a") {
		t.Fatalf("created database was not dropped on rollback: %q", got)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("worktree checkout leaked at %s", path)
	}
	if got := runGit(t, l.Master, "branch", "--list", "fdb-a"); got != "" {
		t.Fatal("branch fdb-a leaked after rollback")
	}
	// The ownership record must already be durable at clone time — before any
	// clone bytes flow — or a crash there loses the database's owner.
	snap, snapErr := os.ReadFile(snapshot)
	if snapErr != nil {
		t.Fatalf("clone-time config snapshot missing: %v", snapErr)
	}
	if !strings.Contains(string(snap), "liftoff_fdb_a") || !strings.Contains(string(snap), "database_name") {
		t.Fatalf("database ownership was not persisted before clone: %s", snap)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Worktrees["fdb-a"]; ok {
		t.Fatal("fully rolled-back dress must not leave a config record")
	}
}

// When rollback itself cannot drop the database, the record — including the
// exact database name — must be retained with CleanupPending for retry,
// not silently lost.
func TestRunDress_FailingDropdb_RetainsRecoveryRecord(t *testing.T) {
	l, _ := dressLayout(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	stubDBBins(t, "#!/bin/sh\nexit 1\n", "#!/bin/sh\nexit 1\n", "", filepath.Join(t.TempDir(), "dropped"))
	path := filepath.Join(filepath.Dir(l.Master), "fdb-b")
	if err := runDressBlocking(l, DressPlan{Name: "fdb-b", Worktree: path, CloneDB: true}); err == nil {
		t.Fatal("dress with failing clone and failing dropdb should fail")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("worktree checkout leaked at %s", path)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta, ok := cfg.Worktrees["fdb-b"]
	if !ok || !meta.CleanupPending {
		t.Fatalf("failed rollback must retain recovery metadata, got %+v exists=%v", meta, ok)
	}
	if meta.DatabaseName != "liftoff_fdb_b" {
		t.Fatalf("retained record lost exact database name: %+v", meta)
	}
	if meta.Path != path || meta.Branch != "fdb-b" {
		t.Fatalf("retained record lost checkout pointer: %+v", meta)
	}
}

// Slot allocation must merge into the existing record, not replace it:
// DatabaseName, paths, and session ownership recorded earlier must survive.
func TestPlanSteps_SlotAllocationPreservesMeta(t *testing.T) {
	l, _ := dressLayout(t)
	setStateDir(t)
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["keep-me"] = WorktreeMeta{
			DatabaseName: "liftoff_keep_me",
			Path:         "/tmp/keep-me",
			Branch:       "keep-me",
			RexID:        "rs1",
			RexLayout:    "default",
			HerdrID:      "w1",
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var slot int
	steps := l.planSteps(DressPlan{Name: "keep-me", Worktree: "/tmp/keep-me"}, &slot)
	if err := steps[len(steps)-1].run(func(string) {}); err != nil {
		t.Fatalf("slot allocation: %v", err)
	}
	if slot <= 0 {
		t.Fatalf("no slot allocated: %d", slot)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta := cfg.Worktrees["keep-me"]
	if meta.Slot != slot {
		t.Fatalf("slot not recorded: %+v", meta)
	}
	if meta.DatabaseName != "liftoff_keep_me" || meta.Path != "/tmp/keep-me" ||
		meta.Branch != "keep-me" || meta.RexID != "rs1" || meta.HerdrID != "w1" {
		t.Fatalf("slot allocation erased ownership metadata: %+v", meta)
	}
}

// A gtab file written by a successful parallel step must still be rolled back
// when a sibling parallel step fails — success iteration must precede failure
// handling, not abort on the first error in index order.
func TestRunDress_ParallelGtabSuccessNotLeaked(t *testing.T) {
	l, _ := dressLayout(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "uv"), "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(filepath.Dir(l.Master), "fdb-d")
	if err := runDressBlocking(l, DressPlan{Name: "fdb-d", Worktree: path, BackendDeps: true, Gtab: true}); err == nil {
		t.Fatal("dress with failing backend install should fail")
	}
	if _, statErr := os.Stat(l.GtabFile("fdb-d")); !os.IsNotExist(statErr) {
		t.Fatalf("gtab file leaked after parallel rollback: %s", l.GtabFile("fdb-d"))
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("worktree checkout leaked at %s", path)
	}
}

func TestRunDressCloneFailureWaitsForDependencyInstallBeforeRollback(t *testing.T) {
	l, _ := dressLayout(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	if err := os.MkdirAll(filepath.Join(l.Master, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, l.Master, "backend/requirements.txt", "")
	runGit(t, l.Master, "add", ".")
	runGit(t, l.Master, "commit", "-m", "backend fixture")
	runGit(t, l.Master, "push", "origin", l.MainBranch)
	markers := t.TempDir()
	t.Setenv("FAILED_MARKER", filepath.Join(markers, "db-failed"))
	t.Setenv("FINISHED_MARKER", filepath.Join(markers, "install-finished"))
	stubDBBins(t, `#!/bin/sh
[ -f "$FINISHED_MARKER" ] || exit 1
printf '%s\n' "$2" >> "$DROP_CAPTURE"
`, "#!/bin/sh\ntouch \"$FAILED_MARKER\"\nexit 1\n", "", filepath.Join(markers, "dropped"))
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "uv"), `#!/bin/sh
for i in $(seq 1 100); do
  if [ -f "$FAILED_MARKER" ]; then
    [ -f requirements.txt ] || exit 1
    touch "$FINISHED_MARKER"
    exit 0
  fi
  sleep 0.02
done
exit 1
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(filepath.Dir(l.Master), "joined-rollback")
	if err := runDressBlocking(l, DressPlan{Name: "joined-rollback", Worktree: path, CloneDB: true, BackendDeps: true, Gtab: true}); err == nil {
		t.Fatal("clone failure must be reported")
	}
	if _, err := os.Stat(filepath.Join(markers, "install-finished")); err != nil {
		t.Fatal("dependency install was interrupted by rollback", err)
	}
	if got := readCapture(t, filepath.Join(markers, "dropped")); !strings.Contains(got, "liftoff_joined_rollback") {
		t.Fatal("database rollback ran before install finished", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("worktree leaked after joined rollback", err)
	}
	if _, err := os.Stat(l.GtabFile("joined-rollback")); !os.IsNotExist(err) {
		t.Fatal("parallel gtab leaked after DB failure", err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Worktrees["joined-rollback"]; ok {
		t.Fatal("rollback left a resource ownership record")
	}
}
