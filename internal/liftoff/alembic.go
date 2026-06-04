package liftoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const masterAlembicDBName = "liftoff"

// alembicVersionsDir is the path (relative to the master repo root) where
// Alembic version files land. Liftoff keeps alembic.ini in backend/ with
// script_location=api/migrations, so versions live under
// backend/api/migrations/versions. Override with KIT_ALEMBIC_VERSIONS_DIR.
func alembicVersionsDir() string {
	if v := os.Getenv("KIT_ALEMBIC_VERSIONS_DIR"); v != "" {
		return v
	}
	return "backend/api/migrations/versions"
}

// MasterHead returns master's current HEAD commit (full SHA).
func (l Layout) MasterHead() (string, error) {
	return Run(l.Master, "git", "rev-parse", "HEAD")
}

// MasterBranch returns the branch currently checked out in the master repo.
func (l Layout) MasterBranch() (string, error) {
	return Run(l.Master, "git", "branch", "--show-current")
}

// NewMigrations returns the Alembic version files added in master between
// oldRev and newRev (basenames only). Returns nil when master didn't move
// (oldRev == newRev), either rev is empty, or oldRev is not an ancestor of
// newRev — so callers run the upgrade only on a real fast-forward that landed
// migrations.
func (l Layout) NewMigrations(oldRev, newRev string) ([]string, error) {
	if oldRev == "" || newRev == "" || oldRev == newRev {
		return nil, nil
	}
	if _, err := Run(l.Master, "git", "merge-base", "--is-ancestor", oldRev, newRev); err != nil {
		return nil, nil
	}
	dir := alembicVersionsDir()
	out, err := Run(l.Master, "git", "diff", "--name-only", "--diff-filter=A", oldRev, newRev, "--", dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, ".py") || strings.HasSuffix(line, "__init__.py") {
			continue
		}
		files = append(files, filepath.Base(line))
	}
	return files, nil
}

// MasterBackendEnvDBName returns SQLALCHEMY_DATABASE_NAME from master's
// backend/.env. Missing files or keys are reported as found=false.
func (l Layout) MasterBackendEnvDBName() (name string, found bool, err error) {
	path := filepath.Join(l.Master, "backend", ".env")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && key == "SQLALCHEMY_DATABASE_NAME" {
			return strings.Trim(strings.TrimSpace(value), `"'`), true, nil
		}
	}
	return "", false, nil
}

// AlembicUpgradeHead runs `alembic upgrade head` in master's backend dir, using
// the venv's alembic (same resolution as pip in InstallBackend) so it works
// without an activated shell.
func (l Layout) AlembicUpgradeHead(onLine LineFn) error {
	if dbName, found, err := l.MasterBackendEnvDBName(); err != nil {
		return err
	} else if found && dbName != "" && dbName != masterAlembicDBName {
		return fmt.Errorf("refusing alembic upgrade: master backend/.env points at SQLALCHEMY_DATABASE_NAME=%s, want %s", dbName, masterAlembicDBName)
	}
	backend := filepath.Join(l.Master, "backend")
	return RunStreamEnv(backend, venvBin("alembic"), []string{"upgrade", "head"}, masterAlembicEnv(), onLine)
}

func masterAlembicEnv() []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		switch key {
		case "environment", "SQLALCHEMY_DATABASE_NAME", "SQLALCHEMY_DATABASE_URL", "SQLALCHEMY_DATABASE_URI", "DATABASE_URL":
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"environment=dev",
		"SQLALCHEMY_DATABASE_NAME="+masterAlembicDBName,
	)
}
