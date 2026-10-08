package liftoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceBackendSelection(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	t.Setenv("KIT_WORKSPACE_BACKEND", "")
	backend, err := WorkspaceBackend()
	if err != nil || backend != BackendRex {
		t.Fatalf("default = %q, %v; want Rex", backend, err)
	}
	if err := WithConfigLock(func(c *Config) error {
		c.Settings.WorkspaceBackend = "rex"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	backend, err = WorkspaceBackend()
	if err != nil || backend != BackendRex {
		t.Fatalf("configured backend = %q, %v", backend, err)
	}
	t.Setenv("KIT_WORKSPACE_BACKEND", "herdr")
	backend, err = WorkspaceBackend()
	if err != nil || backend != BackendHerdr {
		t.Fatalf("override = %q, %v", backend, err)
	}
	t.Setenv("KIT_WORKSPACE_BACKEND", "typo")
	if _, err := WorkspaceBackend(); err == nil {
		t.Fatal("unknown backend silently selected a runtime")
	}
}

func TestWorkspaceBackendHerdrInvocationOverridesSavedRexChoice(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	t.Setenv("KIT_WORKSPACE_BACKEND", "")
	old := os.Args
	os.Args = []string{"/some/install/kit-herdr", "focus", "feature"}
	t.Cleanup(func() { os.Args = old })
	if err := WithConfigLock(func(c *Config) error { c.Settings.WorkspaceBackend = "rex"; return nil }); err != nil {
		t.Fatal(err)
	}
	backend, err := WorkspaceBackend()
	if err != nil || backend != BackendHerdr {
		t.Fatalf("fallback invocation = %q, %v; want Herdr", backend, err)
	}
	// An explicit environment override remains authoritative for scripts.
	t.Setenv("KIT_WORKSPACE_BACKEND", "rex")
	backend, err = WorkspaceBackend()
	if err != nil || backend != BackendRex {
		t.Fatalf("explicit env = %q, %v", backend, err)
	}
}

func TestWorkspaceBackendRejectsUnreadableConfig(t *testing.T) {
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	t.Setenv("KIT_WORKSPACE_BACKEND", "")
	if err := os.WriteFile(filepath.Join(configDir(), "config.toml"), []byte("[broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := WorkspaceBackend(); err == nil {
		t.Fatal("invalid config must not select a destructive runtime fallback")
	}
}
