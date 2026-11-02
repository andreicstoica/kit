package liftoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type TerminalBackend string

const (
	BackendHerdr TerminalBackend = "herdr"
	BackendRex   TerminalBackend = "rex"
)

// WorkspaceBackend defaults to Rex; kit-herdr explicitly selects the fallback.
// A bad config is an error, never permission to act on a different runtime.
func WorkspaceBackend() (TerminalBackend, error) {
	value := os.Getenv("KIT_WORKSPACE_BACKEND")
	if value == "" && filepath.Base(os.Args[0]) == "kit-herdr" {
		value = string(BackendHerdr)
	}
	if value == "" {
		cfg, err := LoadConfig()
		if err != nil {
			return "", err
		}
		value = cfg.Settings.WorkspaceBackend
	}
	if value == "" {
		return BackendRex, nil
	}
	return ParseTerminalBackend(value)
}

func ParseTerminalBackend(value string) (TerminalBackend, error) {
	switch TerminalBackend(value) {
	case BackendHerdr, BackendRex:
		return TerminalBackend(value), nil
	default:
		return "", fmt.Errorf("unknown workspace backend %q; choose rex or herdr", value)
	}
}

// CloseManagedWorkspaces is the cleanup boundary, independent of the selected
// viewer. Switching backends must not make old resource ownership disappear.
// Only persisted mappings authorize background cleanup; explicit user close
// can additionally resolve an unmapped workspace through its selected backend.
func CloseManagedWorkspaces(name, path string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	meta := cfg.Worktrees[name]
	errs := make([]error, 2)
	var closing sync.WaitGroup
	if meta.RexID != "" || (name == "master" && cfg.Settings.RexMasterSession != "") {
		closing.Add(1)
		go func() {
			defer closing.Done()
			if err := CloseRex(name, path); err != nil {
				errs[0] = fmt.Errorf("close Rex: %w", err)
			}
		}()
	}
	if meta.HerdrID != "" || meta.HerdrSpace != "" {
		closing.Add(1)
		go func() {
			defer closing.Done()
			if err := CloseHerdr(name, path); err != nil {
				errs[1] = fmt.Errorf("close Herdr: %w", err)
			}
		}()
	}
	closing.Wait()
	return errors.Join(errs...)
}
