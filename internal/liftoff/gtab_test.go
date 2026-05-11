package liftoff

import (
	"os"
	"strings"
	"testing"
)

func TestWriteGtab(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIT_GTAB_DIR", dir)
	l := DefaultLayout()
	wt := "/Users/acs/liftoff/voice-agent"
	path, err := l.WriteGtab("voice-agent", wt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "/voice-agent.applescript") {
		t.Errorf("unexpected path: %s", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		`set initial working directory of cfg1 to "` + wt + `"`,
		`set initial working directory of cfg2 to "` + wt + `"`,
		`set command of cfg2 to "kit log voice-agent --wait"`,
		`perform action "set_tab_title:🎤 voice-agent"`,
		`perform action "set_tab_title:logs"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("template missing %q\n--- got ---\n%s", want, s)
		}
	}
}

func TestWriteGtabDetailed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIT_GTAB_DIR", dir)
	l := DefaultLayout()
	wt := "/Users/acs/liftoff/voice-agent"
	if _, err := l.WriteGtabLayout("voice-agent", wt, GtabDetailed); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(l.GtabFile("voice-agent"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		`set initial working directory of cfg2 to "` + wt + `/frontend/app"`,
		`set initial working directory of cfgSplit2 to "` + wt + `/frontend/admin"`,
		`set initial working directory of cfg3 to "` + wt + `/backend"`,
		`set initial working directory of cfg4 to "` + wt + `/backend"`,
		`perform action "set_tab_title:frontend"`,
		`perform action "set_tab_title:backend"`,
		`perform action "set_tab_title:celery"`,
		`perform action "set_tab_title:logs"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("detailed template missing %q", want)
		}
	}
}

func TestRemoveGtab_Missing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIT_GTAB_DIR", dir)
	l := DefaultLayout()
	if err := l.RemoveGtab("does-not-exist"); err != nil {
		t.Errorf("RemoveGtab(missing) = %v, want nil", err)
	}
}
