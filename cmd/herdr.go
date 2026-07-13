package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
)

// resolveHerdrLayout keeps reconnects quiet while making the first open
// explicit. A requested --layout always wins; otherwise only a worktree with
// no existing Herdr space gets the familiar simple/detailed picker.
func resolveHerdrLayout(name, path, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	exists, err := liftoff.HerdrWorkspaceExists(name, path)
	if err != nil {
		return "", err
	}
	if exists {
		return "", nil
	}
	return tui.PickHerdrLayout()
}
