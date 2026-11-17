package liftoff

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeDatabaseIdentityPSQL = `
for query do :; done
case "$*" in
  *'DROP DATABASE IF EXISTS'*)
    target=${query#'DROP DATABASE IF EXISTS "'}
    target=${target%'"'}
    case "$*" in *"<> '$DB_CLUSTER_ID'"*) ;; *) exit 1;; esac
    if [ "$DB_RENAMED" = 1 ]; then exit 1; fi
    if [ -f "$DB_IDENTITY_DIR/$target" ]; then
      oid=$(cat "$DB_IDENTITY_DIR/$target")
      case "$*" in *"oid <> $oid)"*) ;; *) exit 1;; esac
    fi
    exec dropdb --if-exists "$target"
    ;;
  *'pg_catalog.pg_control_system()'*)
    case "$query" in *'WHERE oid = '*)
      if [ "$DB_RENAMED" = 1 ]; then printf '%s\t42\n' "$DB_CLUSTER_ID"; else printf '%s\t\n' "$DB_CLUSTER_ID"; fi
      exit 0;;
    esac
    target=${query##*"WHERE datname = '"}
    target=${target%%"'"*}
    if [ "$DB_IDENTITY_FAIL" = 1 ] || { [ "$DB_IDENTITY_FAIL_AFTER_CREATE" = 1 ] && [ -f "$DB_IDENTITY_DIR/$target" ]; }; then exit 1; fi
    oid=
    if [ -f "$DB_IDENTITY_DIR/$target" ]; then oid=$(cat "$DB_IDENTITY_DIR/$target"); fi
    if [ "$DB_EXISTS" = "$target" ]; then oid=42; fi
    printf '%s\t%s\n' "$DB_CLUSTER_ID" "$oid"
    exit 0
    ;;
esac
`

func setupFakeDatabaseIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("DB_IDENTITY_DIR", t.TempDir())
	t.Setenv("DB_CLUSTER_ID", "12345")
	t.Setenv("DB_IDENTITY_FAIL", "")
	t.Setenv("DB_IDENTITY_FAIL_AFTER_CREATE", "")
	t.Setenv("DB_RENAMED", "")
}

func TestCreateDatabasePersistsIntentBeforeCommand(t *testing.T) {
	l := deferredDatabaseLayout(t)
	capture := stubModeDBCommands(t)
	bin := t.TempDir()
	snapshot := filepath.Join(t.TempDir(), "intent")
	t.Setenv("INTENT_SNAPSHOT", snapshot)
	writeExecutable(t, filepath.Join(bin, "createdb"), `#!/bin/sh
cp "$KIT_STATE_DIR/config.toml" "$INTENT_SNAPSHOT"
printf '42\n' > "$DB_IDENTITY_DIR/$1"
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := l.ProvisionDatabase("feature", DatabaseModeEmpty, false, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(snapshot)
	if err != nil || !strings.Contains(string(data), "database_create_intent") || strings.Contains(string(data), "database_name =") {
		t.Fatalf("intent not durable before create: %s err=%v", data, err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta := cfg.Worktrees["feature"]
	if meta.DatabaseCreateIntent != nil || meta.DatabaseIdentity == nil || meta.DatabaseIdentity.ClusterID != "12345" || meta.DatabaseIdentity.OID != 42 {
		t.Fatalf("verified creation identity not persisted: %+v", meta)
	}
	if strings.Contains(readCapture(t, capture), "dropdb") {
		t.Fatal("successful creation dropped a database")
	}
}

func TestCreationIdentityUncertaintyNeverClaimsOrDropsDatabase(t *testing.T) {
	for _, entry := range []string{"provision", "design"} {
		t.Run(entry, func(t *testing.T) {
			var l Layout
			var path string
			name := "feature"
			if entry == "provision" {
				l = deferredDatabaseLayout(t)
				path = l.WorktreePath(name)
			} else {
				l, path = dressLayout(t)
				name = "fdb"
				setStateDir(t)
			}
			capture := stubModeDBCommands(t)
			t.Setenv("DB_IDENTITY_FAIL_AFTER_CREATE", "1")
			var err error
			if entry == "provision" {
				err = l.ProvisionDatabase(name, DatabaseModeEmpty, false, nil)
			} else {
				err = runDressBlocking(l, DressPlan{Name: name, Worktree: path, DatabaseMode: DatabaseModeEmpty})
			}
			if err == nil || !strings.Contains(err.Error(), "unverified") || !strings.Contains(err.Error(), "resolve it manually") {
				t.Fatalf("uncertain creation lacked recovery instructions: %v", err)
			}
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			meta, ok := cfg.Worktrees[name]
			if !ok || meta.DatabaseCreateIntent == nil || meta.DatabaseName != "" || meta.DatabaseIdentity != nil {
				t.Fatalf("uncertain ownership claimed or lost: %+v exists=%v", meta, ok)
			}
			if strings.Contains(readCapture(t, capture), "dropdb") || strings.Contains(readCapture(t, capture), "DROP DATABASE") {
				t.Fatalf("uncertain database dropped: %s", readCapture(t, capture))
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("uncertain creation removed recovery checkout: %v", err)
			}
		})
	}
}

func TestInterruptedCreationBlocksEveryCleanupPath(t *testing.T) {
	l := deferredDatabaseLayout(t)
	capture := stubModeDBCommands(t)
	if err := beginDatabaseCreation("feature"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("DB_IDENTITY_DIR"), "liftoff_feature"), []byte("42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := resolveCleanupDB("feature", l.WorktreePath("feature")); err == nil {
		t.Fatal("interrupted create became cleanup ownership")
	}
	if err := dropOwnedDatabase("feature", "liftoff_feature", nil); err == nil {
		t.Fatal("interrupted create dropped unverified database")
	}
	if err := l.RunWashBlocking(WashPlan{Name: "feature", WorktreePath: l.WorktreePath("feature"), DropDB: false}); err == nil {
		t.Fatal("wash deallocated uncertain ownership with DropDB disabled")
	}
	if err := l.ReconcileOrphan(OrphanCandidate{Name: "feature"}, nil); err == nil {
		t.Fatal("reconcile deallocated uncertain ownership")
	}
	if err := requireProvisionedDatabase("feature", []Service{SvcAPI}); err == nil {
		t.Fatal("interrupted create allowed backend startup")
	}
	if err := l.ProvisionDatabase("feature", DatabaseModeEmpty, false, nil); err == nil {
		t.Fatal("interrupted create allowed reprovisioning")
	}
	cfg, err := LoadConfig()
	if err != nil || cfg.Worktrees["feature"].DatabaseCreateIntent == nil || cfg.Worktrees["feature"].DatabaseName != "" {
		t.Fatalf("interrupted intent was lost or claimed: %+v err=%v", cfg, err)
	}
	if strings.Contains(readCapture(t, capture), "dropdb") || strings.Contains(readCapture(t, capture), "DROP DATABASE") {
		t.Fatalf("interrupted database dropped: %s", readCapture(t, capture))
	}
}

func TestOwnedDatabaseRejectsChangedClusterOrDatabase(t *testing.T) {
	for _, changed := range []string{"cluster", "database", "renamed", "permission"} {
		t.Run(changed, func(t *testing.T) {
			l := deferredDatabaseLayout(t)
			capture := stubModeDBCommands(t)
			if err := l.ProvisionDatabase("feature", DatabaseModeEmpty, false, nil); err != nil {
				t.Fatal(err)
			}
			switch changed {
			case "cluster":
				t.Setenv("DB_CLUSTER_ID", "67890")
			case "database":
				if err := os.WriteFile(filepath.Join(os.Getenv("DB_IDENTITY_DIR"), "liftoff_feature"), []byte("99\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "renamed":
				if err := os.Remove(filepath.Join(os.Getenv("DB_IDENTITY_DIR"), "liftoff_feature")); err != nil {
					t.Fatal(err)
				}
				t.Setenv("DB_RENAMED", "1")
			case "permission":
				t.Setenv("DB_IDENTITY_FAIL", "1")
			}
			if _, _, err := resolveCleanupDB("feature", l.WorktreePath("feature")); err == nil {
				t.Fatal("unverified database accepted for cleanup")
			}
			if changed != "permission" {
				if err := dropOwnedDatabase("feature", "liftoff_feature", nil); err == nil {
					t.Fatal("same-session guard did not reject changed identity")
				}
			}
			if strings.Contains(readCapture(t, capture), "dropdb") {
				t.Fatalf("unrelated database dropped: %s", readCapture(t, capture))
			}
			if _, err := os.Stat(l.WorktreePath("feature")); err != nil {
				t.Fatalf("unverified cleanup removed checkout: %v", err)
			}
		})
	}
}

func TestOwnedDatabaseDropChecksIdentityInSameSession(t *testing.T) {
	l := deferredDatabaseLayout(t)
	capture := stubModeDBCommands(t)
	if err := l.ProvisionDatabase("feature", DatabaseModeEmpty, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := dropOwnedDatabase("feature", "liftoff_feature", nil); err != nil {
		t.Fatal(err)
	}
	calls := readCapture(t, capture)
	if !strings.Contains(calls, "ON_ERROR_STOP=1") || !strings.Contains(calls, "system_identifier::text") || !strings.Contains(calls, "oid <> 42") || !strings.Contains(calls, `DROP DATABASE IF EXISTS "liftoff_feature"`) {
		t.Fatalf("drop omitted a same-session identity guard: %s", calls)
	}
}

func TestDatabaseIdentityPermissionFailurePreventsCreate(t *testing.T) {
	l := deferredDatabaseLayout(t)
	capture := stubModeDBCommands(t)
	t.Setenv("DB_IDENTITY_FAIL", "1")
	err := l.ProvisionDatabase("feature", DatabaseModeEmpty, false, nil)
	if err == nil || !strings.Contains(err.Error(), "requires EXECUTE") {
		t.Fatalf("permission failure not actionable: %v", err)
	}
	if strings.Contains(readCapture(t, capture), "createdb") {
		t.Fatal("database created without cluster identity access")
	}
	cfg, err := LoadConfig()
	if err != nil || cfg.Worktrees["feature"].DatabaseCreateIntent != nil {
		t.Fatalf("permission failure created an intent: %+v err=%v", cfg, err)
	}
}

func TestLegacyDatabaseDropCompatibility(t *testing.T) {
	deferredDatabaseLayout(t)
	capture := stubModeDBCommands(t)
	if err := WithConfigLock(func(c *Config) error {
		meta := c.Worktrees["feature"]
		meta.DatabaseName = "liftoff_feature"
		meta.DatabaseDeferred = false
		c.Worktrees["feature"] = meta
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var lines []string
	if err := dropOwnedDatabase("feature", "liftoff_feature", func(line string) { lines = append(lines, line) }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(lines, " "), "legacy database ownership") || !strings.Contains(readCapture(t, capture), "dropdb --if-exists liftoff_feature") {
		t.Fatalf("legacy cleanup path not explicit: lines=%v calls=%s", lines, readCapture(t, capture))
	}
}

func TestUnverifiedDatabaseRollbackDoesNotInventOwnership(t *testing.T) {
	deferredDatabaseLayout(t)
	stubModeDBCommands(t)
	if err := beginDatabaseCreation("feature"); err != nil {
		t.Fatal(err)
	}
	if err := rollbackProvisionDatabase("feature", "liftoff_feature", errors.New("interrupted")); err == nil {
		t.Fatal("uncertain rollback succeeded")
	}
	cfg, err := LoadConfig()
	if err != nil || cfg.Worktrees["feature"].DatabaseName != "" || cfg.Worktrees["feature"].DatabaseCreateIntent == nil {
		t.Fatalf("rollback invented ownership: %+v err=%v", cfg, err)
	}
}
