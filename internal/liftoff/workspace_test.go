package liftoff

import (
	"os/exec"
	"testing"
)

func TestInstalledEditorsPromotesKitEditor(t *testing.T) {
	// Point KIT_EDITOR at a binary guaranteed on PATH so it's detected.
	bin := "sh"
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("%s not on PATH", bin)
	}
	t.Setenv("KIT_EDITOR", bin)
	eds := InstalledEditors()
	if len(eds) == 0 || eds[0].Binary != bin {
		t.Fatalf("KIT_EDITOR should be promoted to front, got %+v", eds)
	}
}
