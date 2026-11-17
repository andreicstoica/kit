package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func dbCommandFixture(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("KIT_ROOT", t.TempDir())
	t.Setenv("KIT_MASTER_DIR", "master")
	t.Setenv("KIT_STATE_DIR", t.TempDir())
	path := filepath.Join(os.Getenv("KIT_ROOT"), "feature")
	if err := os.MkdirAll(filepath.Join(path, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "backend", ".env"), []byte("SQLALCHEMY_DATABASE_NAME=liftoff_feature\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := liftoff.WithConfigLock(func(c *liftoff.Config) error {
		c.Worktrees["feature"] = liftoff.WorktreeMeta{Slot: 1, Path: path, DatabaseDeferred: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(t.TempDir(), "db-calls")
	t.Setenv("DB_COMMAND_CALLS", capture)
	t.Setenv("DB_COMMAND_BUSY", "")
	t.Setenv("DB_COMMAND_CREATED", filepath.Join(t.TempDir(), "created"))
	bin := t.TempDir()
	for name, body := range map[string]string{
		"createdb": `case "$*" in *--template=*) if [ "$DB_COMMAND_BUSY" = 1 ]; then echo busy >&2; exit 1; fi;; esac
touch "$DB_COMMAND_CREATED"`,
		"psql": `case "$*" in
  *pg_catalog.pg_control_system*)
    if [ -f "$DB_COMMAND_CREATED" ]; then printf '12345\t42\n'; else printf '12345\t\n'; fi
    exit 0;;
esac
if [ "$2" != postgres ]; then cat >/dev/null; fi`,
		"pg_dump": `exit 0`,
		"dropdb":  `exit 0`,
	} {
		script := "#!/bin/sh\nprintf '" + name + " %s\\n' \"$*\" >> \"$DB_COMMAND_CALLS\"\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path, capture
}

func TestDBCommandProvisionsDeferredDatabase(t *testing.T) {
	for _, mode := range []string{"empty", "copy"} {
		t.Run(mode, func(t *testing.T) {
			_, capture := dbCommandFixture(t)
			command := newDBCommand()
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs([]string{"feature", "--mode", mode})
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "database provisioned for feature") {
				t.Fatalf("missing result: %s", output.String())
			}
			calls, err := os.ReadFile(capture)
			if err != nil || strings.Contains(string(calls), "pg_dump") {
				t.Fatalf("unexpected database calls: %s err=%v", calls, err)
			}
			cfg, err := liftoff.LoadConfig()
			if err != nil || cfg.Worktrees["feature"].DatabaseDeferred || cfg.Worktrees["feature"].DatabaseName != "liftoff_feature" {
				t.Fatalf("database not ready: config=%+v err=%v", cfg, err)
			}
		})
	}
}

func TestDBCommandRejectsUnsafeOptions(t *testing.T) {
	for _, args := range [][]string{
		{"feature"},
		{"feature", "--mode", "none"},
		{"feature", "--mode", "unknown"},
		{"feature", "--mode", "empty", "--allow-pg-dump"},
		{"missing", "--mode", "empty"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, capture := dbCommandFixture(t)
			command := newDBCommand()
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs(args)
			if err := command.Execute(); err == nil {
				t.Fatal("invalid provision options succeeded")
			}
			if calls, err := os.ReadFile(capture); err == nil && len(calls) != 0 {
				t.Fatalf("invalid options ran database commands: %s", calls)
			}
		})
	}
}

func TestDBCommandNeverSilentlyDumps(t *testing.T) {
	_, capture := dbCommandFixture(t)
	t.Setenv("DB_COMMAND_BUSY", "1")
	command := newDBCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"feature", "--mode", "copy"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "no full dump attempted") {
		t.Fatalf("copy result = %v", err)
	}
	calls, readErr := os.ReadFile(capture)
	if readErr != nil || strings.Contains(string(calls), "pg_dump") || strings.Contains(string(calls), "createdb liftoff_feature\n") {
		t.Fatalf("busy copy fell back: %s err=%v", calls, readErr)
	}
}
