package liftoff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func TestCreateDBFromTemplateUsesFileCopyAndFallsBackWhenSourceBusy(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "args")
	t.Setenv("ARGS_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeExecutable(t, filepath.Join(bin, "createdb"), "#!/bin/sh\necho \"$@\" >> \"$ARGS_LOG\"\nexit 0\n")
	copied, err := CreateDBFromTemplate("dst", "liftoff", nil)
	if err != nil || !copied {
		t.Fatalf("copied=%v err=%v", copied, err)
	}
	got, _ := os.ReadFile(log)
	if strings.TrimSpace(string(got)) != "dst --template=liftoff --strategy=file_copy" {
		t.Fatalf("createdb args = %q", got)
	}

	writeExecutable(t, filepath.Join(bin, "createdb"), "#!/bin/sh\necho 'source database \"liftoff\" is being accessed by other users' >&2\nexit 1\n")
	var lines []string
	copied, err = CreateDBFromTemplate("dst", "liftoff", func(l string) { lines = append(lines, l) })
	if err != nil || copied {
		t.Fatalf("busy source: copied=%v err=%v, want fallback", copied, err)
	}
	if len(lines) == 0 || !strings.Contains(lines[len(lines)-1], "being accessed") {
		t.Fatalf("fallback reason not reported: %q", lines)
	}
}
