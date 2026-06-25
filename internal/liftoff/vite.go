package liftoff

import (
	"errors"
	"os"
	"path/filepath"
)

// frontendDir maps a frontend service to its worktree-relative dir.
// Backend services (api, celery, …) return "" — they have no Vite cache.
func frontendDir(svc Service) string {
	switch svc {
	case SvcApp:
		return filepath.Join("frontend", "app")
	case SvcAdmin:
		return filepath.Join("frontend", "admin")
	}
	return ""
}

// ViteCacheDir returns the Vite dep-optimizer cache dir for a frontend
// service (<worktree>/frontend/<x>/node_modules/.vite), or "" for backend
// services.
//
// Note: node_modules is symlinked to master (see LinkNodeModules), so this
// path resolves into a cache SHARED by every worktree. Clearing it forces a
// dep re-optimize for all worktrees on their next dev start — which is also
// why a plain restart never fixes stale-dep HMR breakage: bouncing the
// process leaves the shared optimizer cache untouched.
func ViteCacheDir(worktreePath string, svc Service) string {
	sub := frontendDir(svc)
	if sub == "" {
		return ""
	}
	return filepath.Join(worktreePath, sub, "node_modules", ".vite")
}

// ClearViteCache removes the Vite cache dir for a frontend service, the
// equivalent of `rm -rf node_modules/.vite`. Returns the path it cleared, or
// "" when the service has no cache (backend) or the dir was already absent. A
// missing dir is not an error.
func ClearViteCache(worktreePath string, svc Service) (string, error) {
	dir := ViteCacheDir(worktreePath, svc)
	if dir == "" {
		return "", nil
	}
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	return dir, nil
}
