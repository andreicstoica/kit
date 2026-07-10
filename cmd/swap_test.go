package cmd

import (
	"strings"
	"testing"
)

func TestSwapRejectsWorkspaceAndHerdrTogether(t *testing.T) {
	// -w and -H both mean "skip the editor and do X"; passing both must be an
	// explicit error, not a silent preference. The guard runs before any
	// worktree resolution, so no layout setup is needed.
	origWorkspace, origHerdr := swapWorkspace, swapHerdr
	t.Cleanup(func() { swapWorkspace, swapHerdr = origWorkspace, origHerdr })

	swapWorkspace, swapHerdr = true, true
	err := swapCmd.RunE(swapCmd, nil)
	if err == nil {
		t.Fatal("expected an error when both -w and -H are set")
	}
	if !strings.Contains(err.Error(), "--workspace") || !strings.Contains(err.Error(), "--herdr") {
		t.Fatalf("error should name both flags, got %q", err)
	}
}
