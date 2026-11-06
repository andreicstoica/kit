package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestRunPauseHeadlessUnadoptedErrors(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "unadopted"), 0o755); err != nil {
		t.Fatal(err)
	}
	layout := liftoff.Layout{Root: root, Master: root}
	var out bytes.Buffer
	err := RunPauseHeadless(layout, PauseConfig{Name: "unadopted"}, &out)
	if err == nil {
		t.Fatal("want error for unadopted worktree, got nil")
	}
	if !strings.Contains(err.Error(), "no slot") {
		t.Errorf("err = %v, want no-slot error", err)
	}
}

func TestRunningServicesUnadoptedErrors(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	if _, err := runningServices("unadopted", nil); err == nil {
		t.Fatal("want error, got nil")
	}
}
