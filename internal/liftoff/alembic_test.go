package liftoff

import (
	"os"
	"path/filepath"
	"strings"
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

	// Editing an existing migration should not trigger an auto-upgrade; Alembic
	// won't re-run an already-applied revision.
	dir := filepath.Join(l.Master, alembicVersionsDir())
	writeFile(t, dir, "abc123_add_widgets.py", "def upgrade():\n    pass\n")
	runGit(t, l.Master, "add", ".")
	runGit(t, l.Master, "commit", "-m", "edit migration")
	rev4 := runGit(t, l.Master, "rev-parse", "HEAD")
	if got, _ := l.NewMigrations(rev3, rev4); len(got) != 0 {
		t.Errorf("NewMigrations flagged modified migration: %v", got)
	}
}

func TestNewMigrationsSkipsNonFastForward(t *testing.T) {
	l := newMasterRepo(t)
	base := runGit(t, l.Master, "rev-parse", "HEAD")
	revFeature := commitMigration(t, l, "feature_only.py", "def upgrade(): pass")

	runGit(t, l.Master, "checkout", "-B", "master", base)
	revMaster := commitMigration(t, l, "master_only.py", "def upgrade(): pass")

	if got, _ := l.NewMigrations(revFeature, revMaster); len(got) != 0 {
		t.Errorf("NewMigrations over non-fast-forward history = %v, want empty", got)
	}
}

func TestAlembicUpgradeHeadForcesMasterDBEnv(t *testing.T) {
	l := newMasterRepo(t)
	backend := filepath.Join(l.Master, "backend")
	if err := os.MkdirAll(backend, 0o755); err != nil {
		t.Fatal(err)
	}

	venv := filepath.Join(t.TempDir(), "venv")
	bin := filepath.Join(venv, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(t.TempDir(), "capture")
	fakeAlembic := filepath.Join(bin, "alembic")
	script := "#!/bin/sh\n" +
		"printf 'pwd=%s\ndb=%s\nenvironment=%s\nargs=%s\n' \"$PWD\" \"$SQLALCHEMY_DATABASE_NAME\" \"$environment\" \"$*\" > \"$CAPTURE\"\n"
	if err := os.WriteFile(fakeAlembic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("KIT_PY_VENV", venv)
	t.Setenv("CAPTURE", capture)
	t.Setenv("SQLALCHEMY_DATABASE_NAME", "liftoff_feature")
	t.Setenv("environment", "test")
	if err := l.AlembicUpgradeHead(nil); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{
		"pwd=" + backend,
		"db=liftoff",
		"environment=dev",
		"args=upgrade head",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("capture missing %q:\n%s", want, out)
		}
	}
}

func installFakeAlembicCurrent(t *testing.T, currentOut string) {
	t.Helper()
	venv := filepath.Join(t.TempDir(), "venv")
	bin := filepath.Join(venv, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeAlembic := filepath.Join(bin, "alembic")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  current) printf '%s\\n' \"$CURRENT\" ;;\n" +
		"  upgrade) echo \"Running upgrade\" ;;\n" +
		"  *) echo \"unexpected: $*\" >&2; exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(fakeAlembic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KIT_PY_VENV", venv)
	t.Setenv("CURRENT", currentOut)
}

func TestAlembicAtHead(t *testing.T) {
	l := newMasterRepo(t)
	backend := filepath.Join(l.Master, "backend")
	if err := os.MkdirAll(backend, 0o755); err != nil {
		t.Fatal(err)
	}

	installFakeAlembicCurrent(t, "a4b8c1e9d6f3 (head)")
	atHead, err := l.AlembicAtHead()
	if err != nil {
		t.Fatal(err)
	}
	if !atHead {
		t.Fatal("AlembicAtHead = false, want true when current shows (head)")
	}

	t.Setenv("CURRENT", "e58a3c2f9d10")
	atHead, err = l.AlembicAtHead()
	if err != nil {
		t.Fatal(err)
	}
	if atHead {
		t.Fatal("AlembicAtHead = true, want false when current is behind head")
	}
}

func TestAlembicAtHeadEmptyCurrentIsBehind(t *testing.T) {
	l := newMasterRepo(t)
	backend := filepath.Join(l.Master, "backend")
	if err := os.MkdirAll(backend, 0o755); err != nil {
		t.Fatal(err)
	}
	installFakeAlembicCurrent(t, "")

	atHead, err := l.AlembicAtHead()
	if err != nil {
		t.Fatal(err)
	}
	if atHead {
		t.Fatal("AlembicAtHead = true, want false when no revision is applied")
	}
}

func TestAlembicAtHeadIgnoresLogLines(t *testing.T) {
	l := newMasterRepo(t)
	backend := filepath.Join(l.Master, "backend")
	if err := os.MkdirAll(backend, 0o755); err != nil {
		t.Fatal(err)
	}
	installFakeAlembicCurrent(t, "INFO Context impl PostgresqlImpl.\na4b8c1e9d6f3 (head)")

	atHead, err := l.AlembicAtHead()
	if err != nil {
		t.Fatal(err)
	}
	if !atHead {
		t.Fatal("AlembicAtHead = false, want true when (head) appears among log lines")
	}
}

func TestAlembicAtHeadRefusesFeatureDBInMasterEnv(t *testing.T) {
	l := newMasterRepo(t)
	backend := filepath.Join(l.Master, "backend")
	if err := os.MkdirAll(backend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backend, ".env"), []byte("SQLALCHEMY_DATABASE_NAME=liftoff_feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	installFakeAlembicCurrent(t, "a4b8c1e9d6f3 (head)")

	_, err := l.AlembicAtHead()
	if err == nil || !strings.Contains(err.Error(), "refusing alembic") {
		t.Fatalf("AlembicAtHead error = %v, want refusal", err)
	}
}

func TestMasterBackendEnvDBName(t *testing.T) {
	l := newMasterRepo(t)
	backend := filepath.Join(l.Master, "backend")
	if err := os.MkdirAll(backend, 0o755); err != nil {
		t.Fatal(err)
	}

	name, found, err := l.MasterBackendEnvDBName()
	if err != nil || found || name != "" {
		t.Fatalf("missing .env = (%q, %v, %v), want (\"\", false, nil)", name, found, err)
	}

	if err := os.WriteFile(filepath.Join(backend, ".env"), []byte(
		"# comment\nSQLALCHEMY_DATABASE_NAME=\"liftoff\"\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	name, found, err = l.MasterBackendEnvDBName()
	if err != nil || !found || name != "liftoff" {
		t.Fatalf("quoted liftoff = (%q, %v, %v), want (liftoff, true, nil)", name, found, err)
	}
}

func TestAlembicUpgradeHeadRefusesFeatureDBInMasterEnv(t *testing.T) {
	l := newMasterRepo(t)
	backend := filepath.Join(l.Master, "backend")
	if err := os.MkdirAll(backend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backend, ".env"), []byte("SQLALCHEMY_DATABASE_NAME=liftoff_feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := l.AlembicUpgradeHead(nil)
	if err == nil || !strings.Contains(err.Error(), "refusing alembic") {
		t.Fatalf("AlembicUpgradeHead error = %v, want refusal", err)
	}
}
