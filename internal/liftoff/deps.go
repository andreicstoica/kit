package liftoff

import "path/filepath"

// InstallBackend runs `pip install -q -r requirements.txt -r requirements_test.txt`
// inside the worktree's backend/ dir.
func InstallBackend(worktree string, onLine LineFn) error {
	dir := filepath.Join(worktree, "backend")
	return RunStream(dir, "pip", []string{
		"install", "-q",
		"-r", "requirements.txt",
		"-r", "requirements_test.txt",
	}, onLine)
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
