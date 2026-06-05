package cmd

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newSyncMasterRepo(t *testing.T) liftoff.Layout {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	if rp, err := filepath.EvalSymlinks(root); err == nil {
		root = rp
	}
	master := filepath.Join(root, "master")
	remote := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(master, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}

	runTestGit(t, remote, "init", "--bare", "-b", "master")
	runTestGit(t, master, "init", "-b", "master")
	runTestGit(t, master, "config", "user.email", "test@example.com")
	runTestGit(t, master, "config", "user.name", "Test")
	runTestGit(t, master, "config", "commit.gpgsign", "false")
	writeTestFile(t, master, "README.md", "init")
	runTestGit(t, master, "add", ".")
	runTestGit(t, master, "commit", "-m", "init")
	runTestGit(t, master, "remote", "add", "origin", remote)
	runTestGit(t, master, "push", "-u", "origin", "master")

	return liftoff.Layout{Master: master, MainBranch: "master"}
}

func setupSyncBackend(t *testing.T, l liftoff.Layout) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(l.Master, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func installFakeAlembic(t *testing.T, currentOut string, upgradeFail bool) {
	t.Helper()
	venv := filepath.Join(t.TempDir(), "venv")
	bin := filepath.Join(venv, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  current) printf '%s\\n' \"$FAKE_ALEMBIC_CURRENT\" ;;\n" +
		"  upgrade)\n" +
		"    if [ \"$FAKE_ALEMBIC_UPGRADE_FAIL\" = \"1\" ]; then\n" +
		"      echo \"upgrade failed\" >&2\n" +
		"      exit 1\n" +
		"    fi\n" +
		"    echo \"Running upgrade fake -> head\"\n" +
		"    ;;\n" +
		"  *) echo \"unexpected: $*\" >&2; exit 1 ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(bin, "alembic"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KIT_PY_VENV", venv)
	t.Setenv("FAKE_ALEMBIC_CURRENT", currentOut)
	if upgradeFail {
		t.Setenv("FAKE_ALEMBIC_UPGRADE_FAIL", "1")
	} else {
		t.Setenv("FAKE_ALEMBIC_UPGRADE_FAIL", "0")
	}
}

func commitSyncMigration(t *testing.T, l liftoff.Layout, file string) string {
	t.Helper()
	dir := filepath.Join(l.Master, "backend", "api", "migrations", "versions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, file, "def upgrade(): pass")
	runTestGit(t, l.Master, "add", ".")
	runTestGit(t, l.Master, "commit", "-m", "migration "+file)
	return runTestGit(t, l.Master, "rev-parse", "HEAD")
}

func TestPrintMigrateSummary(t *testing.T) {
	layout := liftoff.Layout{MainBranch: "master"}
	cases := []struct {
		name string
		in   migrateResult
		want []string
		not  []string
	}{
		{
			name: "skipped no-migrate",
			in:   migrateResult{status: migrateSkippedNoMigrate},
			want: []string{"master DB migration skipped", "--no-migrate"},
			not:  []string{"feature DBs were not migrated"},
		},
		{
			name: "skipped not trunk",
			in:   migrateResult{status: migrateSkippedNotTrunk, branch: "shared-animated-chip-list"},
			want: []string{
				"master DB migration skipped",
				"master on shared-animated-chip-list",
				"want master",
			},
			not: []string{"feature DBs were not migrated"},
		},
		{
			name: "already at head",
			in:   migrateResult{status: migrateAtHead},
			want: []string{
				"master DB (liftoff) already at head",
				"feature DBs were not migrated",
			},
		},
		{
			name: "upgraded",
			in:   migrateResult{status: migrateUpgraded},
			want: []string{
				"master DB (liftoff) upgraded to head",
				"feature DBs were not migrated",
			},
		},
		{
			name: "failed with error",
			in: migrateResult{
				status: migrateFailed,
				err:    errTest("connection refused"),
			},
			want: []string{
				"master DB migration failed",
				"connection refused",
				"feature DBs were not migrated",
			},
		},
		{
			name: "failed without error",
			in:   migrateResult{status: migrateFailed},
			want: []string{"master DB migration failed"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(func() {
				printMigrateSummary(layout, tc.in)
			})
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("output missing %q:\n%s", want, out)
				}
			}
			for _, not := range tc.not {
				if strings.Contains(out, not) {
					t.Fatalf("output unexpectedly contains %q:\n%s", not, out)
				}
			}
		})
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

func TestRunMasterMigrateSkipsWhenNotOnTrunk(t *testing.T) {
	l := newSyncMasterRepo(t)
	setupSyncBackend(t, l)
	installFakeAlembic(t, "e58a3c2f9d10", false)

	runTestGit(t, l.Master, "checkout", "-b", "shared-animated-chip-list")

	got := runMasterMigrate(l, "")
	if got.status != migrateSkippedNotTrunk {
		t.Fatalf("status = %v, want migrateSkippedNotTrunk", got.status)
	}
	if got.branch != "shared-animated-chip-list" {
		t.Fatalf("branch = %q, want shared-animated-chip-list", got.branch)
	}
}

func TestRunMasterMigrateAlreadyAtHead(t *testing.T) {
	l := newSyncMasterRepo(t)
	setupSyncBackend(t, l)
	installFakeAlembic(t, "a4b8c1e9d6f3 (head)", false)

	got := runMasterMigrate(l, runTestGit(t, l.Master, "rev-parse", "HEAD"))
	if got.status != migrateAtHead {
		t.Fatalf("status = %v, want migrateAtHead", got.status)
	}
}

func TestRunMasterMigrateUpgradesWhenBehind(t *testing.T) {
	l := newSyncMasterRepo(t)
	setupSyncBackend(t, l)
	installFakeAlembic(t, "e58a3c2f9d10", false)

	out := captureStdout(func() {
		got := runMasterMigrate(l, runTestGit(t, l.Master, "rev-parse", "HEAD"))
		if got.status != migrateUpgraded {
			t.Fatalf("status = %v, want migrateUpgraded", got.status)
		}
	})
	for _, want := range []string{
		"master DB behind head",
		"alembic upgrade head",
		"Running upgrade fake -> head",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunMasterMigrateUpgradeFailure(t *testing.T) {
	l := newSyncMasterRepo(t)
	setupSyncBackend(t, l)
	installFakeAlembic(t, "e58a3c2f9d10", true)

	got := runMasterMigrate(l, "")
	if got.status != migrateFailed {
		t.Fatalf("status = %v, want migrateFailed", got.status)
	}
	if got.err == nil {
		t.Fatal("err = nil, want upgrade error")
	}
}

func TestRunMasterMigrateFailsWhenMasterEnvPointsAtFeatureDB(t *testing.T) {
	l := newSyncMasterRepo(t)
	backend := filepath.Join(l.Master, "backend")
	if err := os.MkdirAll(backend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(backend, ".env"),
		[]byte("SQLALCHEMY_DATABASE_NAME=liftoff_feature\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	installFakeAlembic(t, "e58a3c2f9d10", false)

	got := runMasterMigrate(l, "")
	if got.status != migrateFailed {
		t.Fatalf("status = %v, want migrateFailed", got.status)
	}
	if got.err == nil || !strings.Contains(got.err.Error(), "refusing alembic") {
		t.Fatalf("err = %v, want refusal", got.err)
	}
}

func TestRunMasterMigrateListsNewMigrationsFromSync(t *testing.T) {
	l := newSyncMasterRepo(t)
	setupSyncBackend(t, l)
	installFakeAlembic(t, "e58a3c2f9d10", false)

	oldHead := runTestGit(t, l.Master, "rev-parse", "HEAD")
	commitSyncMigration(t, l, "a4b8c1e9d6f3_add_admin_searches.py")

	out := captureStdout(func() {
		got := runMasterMigrate(l, oldHead)
		if got.status != migrateUpgraded {
			t.Fatalf("status = %v, want migrateUpgraded", got.status)
		}
	})
	for _, want := range []string{
		"1 new migration(s) on master",
		"a4b8c1e9d6f3_add_admin_searches.py",
		"Running upgrade fake -> head",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "master DB behind head") {
		t.Fatalf("expected new-migration title, got behind-head title:\n%s", out)
	}
}

func TestRunMasterMigrateBehindWithoutNewSyncMigrations(t *testing.T) {
	l := newSyncMasterRepo(t)
	setupSyncBackend(t, l)
	installFakeAlembic(t, "e58a3c2f9d10", false)

	head := runTestGit(t, l.Master, "rev-parse", "HEAD")
	out := captureStdout(func() {
		got := runMasterMigrate(l, head)
		if got.status != migrateUpgraded {
			t.Fatalf("status = %v, want migrateUpgraded", got.status)
		}
	})
	if !strings.Contains(out, "master DB behind head") {
		t.Fatalf("output missing behind-head title:\n%s", out)
	}
	if strings.Contains(out, "new migration(s) on master") {
		t.Fatalf("unexpected new-migration title:\n%s", out)
	}
}
