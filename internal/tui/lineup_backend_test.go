package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestLineupUsesSelectedRexBackendNotHerdr(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "master")
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KIT_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("KIT_RUN_DIR", filepath.Join(dir, "run"))
	t.Setenv("KIT_WORKSPACE_BACKEND", "rex")
	t.Setenv("PATH", bin)
	herdrCalled := filepath.Join(dir, "herdr-called")
	fixtures := map[string]string{
		"git":   "#!/bin/sh\ncase \"$*\" in *'worktree list'*) printf 'worktree " + root + "\\nHEAD abc\\nbranch refs/heads/master\\n\\n';; esac\n",
		"rex":   "#!/bin/sh\nprintf '%s\\n' '{\"sessions\":[{\"session_id\":\"rex-owned\",\"label\":\"master\",\"windows\":[{\"window_id\":\"window1\",\"blocks\":[{\"block_id\":\"pane1\"},{\"block_id\":\"pane2\"}]}]}]}'\n",
		"herdr": "#!/bin/sh\nprintf called > '" + herdrCalled + "'\nexit 1\n",
	}
	for name, fixture := range fixtures {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(fixture), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := liftoff.WithConfigLock(func(c *liftoff.Config) error {
		c.Settings.RexMasterSession = "rex-owned"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err := RenderLineup(liftoff.Layout{Master: root, Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "REX") || !strings.Contains(out, "1 tab") || !strings.Contains(out, "2 panes") {
		t.Fatalf("lineup did not report selected Rex workspace: %s", out)
	}
	if _, err := os.Stat(herdrCalled); !os.IsNotExist(err) {
		t.Fatal("Rex lineup queried Herdr instead of its selected runtime")
	}
}
