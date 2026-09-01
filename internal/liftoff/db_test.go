package liftoff

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	writeExecutable(t, filepath.Join(bin, "dropdb"), "#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$CAPTURE\"\n")
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

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatal(err)
	}
}
