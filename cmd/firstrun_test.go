package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupConfigExistsUsesConfiguredStateDir(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("KIT_STATE_DIR", stateDir)
	if err := os.WriteFile(filepath.Join(stateDir, "config.toml"), []byte("schema = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !setupConfigExists() {
		t.Fatal("setup config in KIT_STATE_DIR should suppress the first-run setup offer")
	}
}
