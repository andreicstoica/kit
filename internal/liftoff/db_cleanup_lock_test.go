package liftoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunDressExplicitExistingRecordNeverMutates(t *testing.T) {
	l, path := dressLayout(t)
	setStateDir(t)
	capture := stubModeDBCommands(t)
	prior := WorktreeMeta{Slot: 99, Path: path, DatabaseName: "liftoff_valuable"}
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["fdb"] = prior
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err := runDressBlocking(l, DressPlan{Name: "fdb", Worktree: path, DatabaseMode: DatabaseModeEmpty})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("existing record accepted: %v", err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta := cfg.Worktrees["fdb"]
	if meta.Slot != prior.Slot || meta.Path != prior.Path || meta.DatabaseName != prior.DatabaseName || readCapture(t, capture) != "" {
		t.Fatalf("existing ownership changed: %+v calls=%s", meta, readCapture(t, capture))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rejected creation touched checkout: %v", err)
	}
}

func TestRunDressSerializesLegacyAndExplicitPlans(t *testing.T) {
	l, path := dressLayout(t)
	setStateDir(t)
	setRunDir(t)
	if err := writeIsolatedDatabaseEnv(l.Master, "liftoff"); err != nil {
		t.Fatal(err)
	}
	capture := stubModeDBCommands(t)
	barrier := t.TempDir()
	t.Setenv("DB_CREATE_BARRIER", barrier)
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "createdb"), `#!/bin/sh
printf 'createdb %s\n' "$*" >> "$DB_MODE_CALLS"
touch "$DB_CREATE_BARRIER/started"
while [ ! -f "$DB_CREATE_BARRIER/release" ]; do sleep 0.01; done
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	legacyDone := make(chan error, 1)
	go func() { legacyDone <- runDressBlocking(l, DressPlan{Name: "fdb", Worktree: path, CloneDB: true}) }()
	release := func() { _ = os.WriteFile(filepath.Join(barrier, "release"), nil, 0o600) }
	defer release()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(barrier, "started")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("legacy database creation never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	explicitDone := make(chan error, 1)
	go func() {
		explicitDone <- runDressBlocking(l, DressPlan{Name: "fdb", Worktree: path, DatabaseMode: DatabaseModeEmpty})
	}()
	select {
	case err := <-explicitDone:
		t.Fatalf("explicit plan ran while legacy creation held lock: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	release()
	select {
	case err := <-legacyDone:
		if err != nil {
			t.Fatalf("legacy winner failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("legacy winner deadlocked")
	}
	select {
	case err := <-explicitDone:
		if err == nil || !strings.Contains(err.Error(), "already registered") {
			t.Fatalf("explicit loser did not preserve winner: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("explicit loser deadlocked")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if meta := cfg.Worktrees["fdb"]; meta.DatabaseName != "liftoff_fdb" || meta.Slot == 0 || meta.CleanupPending {
		t.Fatalf("winner's ownership lost: %+v", meta)
	}
	if strings.Contains(readCapture(t, capture), "dropdb") || strings.Count(readCapture(t, capture), "createdb ") != 1 {
		t.Fatalf("loser created or dropped a database: %s", readCapture(t, capture))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("loser removed winner checkout: %v", err)
	}
}

func TestDatabaseProvisionSerializesCleanup(t *testing.T) {
	for _, cleanup := range []string{"wash", "reconcile"} {
		t.Run(cleanup, func(t *testing.T) {
			l := newMasterRepo(t)
			setStateDir(t)
			setRunDir(t)
			l.GtabDir = t.TempDir()
			path := addWorktree(t, l, "feature")
			if err := writeIsolatedDatabaseEnv(path, DBName("feature")); err != nil {
				t.Fatal(err)
			}
			if err := WithConfigLock(func(c *Config) error {
				c.Worktrees["feature"] = WorktreeMeta{Slot: 99, Path: path, Branch: "feature", DatabaseDeferred: true}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			capture := stubModeDBCommands(t)
			barrier := t.TempDir()
			t.Setenv("DB_CREATE_BARRIER", barrier)
			bin := t.TempDir()
			writeExecutable(t, filepath.Join(bin, "createdb"), `#!/bin/sh
printf 'createdb %s\n' "$*" >> "$DB_MODE_CALLS"
touch "$DB_CREATE_BARRIER/started"
while [ ! -f "$DB_CREATE_BARRIER/release" ]; do sleep 0.01; done
touch "$DB_CREATE_BARRIER/exists"
`)
			writeExecutable(t, filepath.Join(bin, "psql"), `#!/bin/sh
if [ "$2" = postgres ]; then
  if [ -f "$DB_CREATE_BARRIER/exists" ]; then printf 'liftoff_feature\n'; fi
else
  cat >/dev/null
fi
`)
			writeExecutable(t, filepath.Join(bin, "dropdb"), `#!/bin/sh
printf 'dropdb %s\n' "$*" >> "$DB_MODE_CALLS"
rm -f "$DB_CREATE_BARRIER/exists"
`)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			provisionDone := make(chan error, 1)
			go func() { provisionDone <- l.ProvisionDatabase("feature", DatabaseModeCopy, false, nil) }()
			release := func() { _ = os.WriteFile(filepath.Join(barrier, "release"), nil, 0o600) }
			defer release()
			deadline := time.Now().Add(2 * time.Second)
			for {
				if _, err := os.Stat(filepath.Join(barrier, "started")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("fake template creation never started")
				}
				time.Sleep(5 * time.Millisecond)
			}
			if cleanup == "reconcile" {
				if err := os.RemoveAll(path); err != nil {
					t.Fatal(err)
				}
			}
			cleanupDone := make(chan error, 1)
			go func() {
				if cleanup == "wash" {
					cleanupDone <- l.RunWashBlocking(WashPlan{Name: "feature", Branch: "feature", WorktreePath: path, DropDB: true})
				} else {
					cleanupDone <- l.ReconcileOrphan(OrphanCandidate{Name: "feature", Path: path}, nil)
				}
			}()
			select {
			case err := <-cleanupDone:
				t.Fatalf("cleanup ran during template creation: %v", err)
			case <-time.After(75 * time.Millisecond):
			}
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			meta, ok := cfg.Worktrees["feature"]
			if !ok || !meta.DatabaseDeferred || meta.DatabaseName != "" || meta.CleanupPending {
				t.Fatalf("cleanup mutated provisioning state: %+v exists=%v", meta, ok)
			}
			release()
			select {
			case err := <-provisionDone:
				if (err != nil) != (cleanup == "reconcile") {
					t.Fatalf("unexpected provision result: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("provision did not finish after release")
			}
			select {
			case err := <-cleanupDone:
				if err != nil {
					t.Fatalf("serialized cleanup failed: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cleanup deadlocked after provisioning")
			}
			cfg, err = LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := cfg.Worktrees["feature"]; ok {
				t.Fatalf("provision resurrected cleaned-up record: %+v", cfg.Worktrees["feature"])
			}
			if strings.Count(readCapture(t, capture), "dropdb --if-exists liftoff_feature") != 1 {
				t.Fatalf("owned database was not cleaned up exactly once: %s", readCapture(t, capture))
			}
		})
	}
}
