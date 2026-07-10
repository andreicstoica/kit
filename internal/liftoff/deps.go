package liftoff

import (
	"os"
	"path/filepath"
)

// InstallBackend runs `uv pip install --python <venv>/bin/python -q
// -r requirements.txt -r requirements_test.txt` inside the worktree's backend/
// dir. uv (same maker as ruff) replaced pip for dramatically faster cold
// installs; upstream switched CI + Dockerfiles in liftoff-app #7677.
//
// `--python` targets the configured venv's interpreter directly, so installs
// land in the right env even when the user hasn't run
// `source ~/.envs/py314/bin/activate` in the shell that launched kit.
func InstallBackend(worktree string, onLine LineFn) error {
	dir := filepath.Join(worktree, "backend")
	venvPy := venvBin("python")
	return RunStream(dir, "uv", []string{
		"pip", "install", "-q",
		"--python", venvPy,
		"-r", "requirements.txt",
		"-r", "requirements_test.txt",
	}, onLine)
}

// venvBin resolves a binary inside KIT_PY_VENV (default ~/.envs/py314).
// Falls back to the bare name so callers degrade to PATH lookup when no
// venv is configured.
func venvBin(name string) string {
	venv := os.Getenv("KIT_PY_VENV")
	if venv == "" {
		home, _ := os.UserHomeDir()
		venv = filepath.Join(home, ".envs", "py314")
	}
	candidate := filepath.Join(venv, "bin", name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return name
}

// InstallFrontendApp runs `yarn install --pure-lockfile --silent` in frontend/app.
func InstallFrontendApp(worktree string, onLine LineFn) error {
	return runYarn(filepath.Join(worktree, "frontend", "app"), onLine)
}

// InstallFrontendAdmin runs `yarn install --pure-lockfile --silent` in frontend/admin.
func InstallFrontendAdmin(worktree string, onLine LineFn) error {
	return runYarn(filepath.Join(worktree, "frontend", "admin"), onLine)
}

func runYarn(dir string, onLine LineFn) error {
	return RunStream(dir, "yarn", []string{"install", "--pure-lockfile", "--silent"}, onLine)
}
