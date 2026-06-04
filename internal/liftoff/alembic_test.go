package liftoff

import (
	"os"
	"path/filepath"
	"testing"
)

// commitMigration writes a file under the Alembic versions dir on master and
// commits it, returning the new HEAD.
func commitMigration(t *testing.T, l Layout, file, body string) string {
	t.Helper()
	dir := filepath.Join(l.Master, alembicVersionsDir())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, file, body)
	runGit(t, l.Master, "add", ".")
	runGit(t, l.Master, "commit", "-m", "migration "+file)
	return runGit(t, l.Master, "rev-parse", "HEAD")
}

func TestNewMigrations(t *testing.T) {
	l := newMasterRepo(t)
	base := runGit(t, l.Master, "rev-parse", "HEAD")

	// No movement → no-op.
	if got, _ := l.NewMigrations(base, base); len(got) != 0 {
		t.Errorf("NewMigrations(base, base) = %v, want empty (master didn't move)", got)
	}
	// Empty rev → no-op (HEAD lookup failed upstream).
	if got, _ := l.NewMigrations("", base); len(got) != 0 {
		t.Errorf("NewMigrations(\"\", base) = %v, want empty", got)
	}

	// Land a migration.
	rev1 := commitMigration(t, l, "abc123_add_widgets.py", "def upgrade(): pass")
	got, err := l.NewMigrations(base, rev1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "abc123_add_widgets.py" {
		t.Errorf("NewMigrations after one migration = %v, want [abc123_add_widgets.py]", got)
	}

	// A commit that touches no migrations → no-op.
	writeFile(t, l.Master, "README.md", "changed")
	runGit(t, l.Master, "add", ".")
	runGit(t, l.Master, "commit", "-m", "docs")
	rev2 := runGit(t, l.Master, "rev-parse", "HEAD")
	if got, _ := l.NewMigrations(rev1, rev2); len(got) != 0 {
		t.Errorf("NewMigrations over a non-migration commit = %v, want empty", got)
	}

	// __init__.py in the versions dir must be ignored.
	rev3 := commitMigration(t, l, "__init__.py", "")
	if got, _ := l.NewMigrations(rev2, rev3); len(got) != 0 {
		t.Errorf("NewMigrations flagged __init__.py: %v", got)
	}
}
