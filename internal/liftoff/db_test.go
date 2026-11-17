package liftoff

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCloneDBStopsOnRestoreError(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "pg_dump"), "#!/bin/sh\nprintf 'invalid SQL;\\n'\n")
	writeExecutable(t, filepath.Join(bin, "psql"), `#!/bin/sh
while IFS= read -r line; do :; done
if [ "$1" = "-X" ] && [ "$2" = "-v" ] && [ "$3" = "ON_ERROR_STOP=1" ]; then
    exit 3
fi
exit 0
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := CloneDB("source", "target", nil); err == nil || !strings.Contains(err.Error(), "psql restore: exit status 3") {
		t.Fatalf("want restore failure, got %v", err)
	}
}

func TestDropDBMissingDatabase(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "dropdb"), `#!/bin/sh
if [ "$1" = "--if-exists" ] && [ "$2" = "liftoff_missing" ]; then
    exit 0
fi
echo 'dropdb: error: database "liftoff_missing" does not exist' >&2
exit 1
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := DropDB("liftoff_missing", nil); err != nil {
		t.Fatalf("DropDB missing database: %v", err)
	}
}

func TestSweepOldTestDBs(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	capture := filepath.Join(bin, "dropped")
	old := time.Now().Add(-25 * time.Hour).Unix()
	recent := time.Now().Add(-time.Hour).Unix()
	psql := fmt.Sprintf(`#!/bin/sh
cat <<'EOF'
liftoff_test_db_old	%d
liftoff_e2e_old	%d
liftoff_candidate_share_bug_test_db	%d
liftoff_test_db_recent	%d
liftoff_stray_newline_name	%d
liftoff_feature	%d
other_test_db	%d
EOF
`, old, old, old, recent, old, old, old)
	writeExecutable(t, filepath.Join(bin, "psql"), psql)
	writeExecutable(t, filepath.Join(bin, "dropdb"), "#!/bin/sh\ntest \"$1\" = \"--if-exists\" || exit 1\nprintf '%s\\n' \"$2\" >> \"$CAPTURE\"\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CAPTURE", capture)
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	config.Worktrees["e2e-old"] = WorktreeMeta{}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	removed, errs := SweepOldTestDBs(24 * time.Hour)
	if len(errs) != 0 {
		t.Fatalf("SweepOldTestDBs errors = %v", errs)
	}
	if removed != 2 {
		t.Fatalf("SweepOldTestDBs removed = %d, want 2", removed)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(data))
	want := []string{
		"liftoff_test_db_old",
		"liftoff_candidate_share_bug_test_db",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("dropped databases = %v, want %v", got, want)
	}
}

func TestDBNameFromEnv(t *testing.T) {
	wt := t.TempDir()
	if _, found, err := DBNameFromEnv(wt); err != nil || found {
		t.Fatalf("missing env should report found=false, got found=%v err=%v", found, err)
	}
	backend := filepath.Join(wt, "backend")
	if err := os.MkdirAll(backend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backend, ".env"), []byte("# comment\nSQLALCHEMY_DATABASE_NAME=\"liftoff_feat_x\"\nOTHER=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, found, err := DBNameFromEnv(wt); err != nil || !found || got != "liftoff_feat_x" {
		t.Fatalf("DBNameFromEnv = %q, found=%v, err=%v", got, found, err)
	}
	if err := os.WriteFile(filepath.Join(backend, ".env"), []byte("OTHER=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := DBNameFromEnv(wt); err != nil || found {
		t.Fatalf("missing key should report found=false, got found=%v err=%v", found, err)
	}
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCloneDBStopsProducerWhenRestoreFails(t *testing.T) {
	bin := t.TempDir()
	pidPath := filepath.Join(bin, "dump-pid")
	t.Setenv("DUMP_PID", pidPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeExecutable(t, filepath.Join(bin, "pg_dump"), "#!/bin/sh\necho $$ > \"$DUMP_PID\"\nexec sleep 30\n")
	writeExecutable(t, filepath.Join(bin, "psql"), "#!/bin/sh\nwhile [ ! -s \"$DUMP_PID\" ]; do sleep 0.01; done\nexit 1\n")
	err := CloneDB("source", "destination", nil)
	if err == nil || !strings.Contains(err.Error(), "psql restore") {
		t.Fatalf("restore error=%v", err)
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if process, err := os.FindProcess(pid); err == nil {
			_ = process.Kill()
		}
	}()
	if IsAlive(pid) {
		t.Fatal("failed restore left pg_dump running")
	}
}
