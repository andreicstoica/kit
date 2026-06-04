package liftoff

import (
	"os"
	"path/filepath"
	"strings"
)

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

// NewMigrations returns the Alembic version files added or modified in master
// between oldRev and newRev (basenames only). Returns nil when master didn't
// move (oldRev == newRev) or when either rev is empty — so callers run the
// upgrade only on a real fast-forward that landed migrations.
func (l Layout) NewMigrations(oldRev, newRev string) ([]string, error) {
	if oldRev == "" || newRev == "" || oldRev == newRev {
		return nil, nil
	}
	dir := alembicVersionsDir()
	out, err := Run(l.Master, "git", "diff", "--name-only", "--diff-filter=AM", oldRev, newRev, "--", dir)
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

// AlembicUpgradeHead runs `alembic upgrade head` in master's backend dir, using
// the venv's alembic (same resolution as pip in InstallBackend) so it works
// without an activated shell.
func (l Layout) AlembicUpgradeHead(onLine LineFn) error {
	backend := filepath.Join(l.Master, "backend")
	return RunStream(backend, venvBin("alembic"), []string{"upgrade", "head"}, onLine)
}
