package liftoff

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func stubModeDBCommands(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	setupFakeDatabaseIdentity(t)
	capture := filepath.Join(t.TempDir(), "calls")
	t.Setenv("DB_MODE_CALLS", capture)
	t.Setenv("DB_TEMPLATE_FAIL", "")
	t.Setenv("DB_DUMP_FAIL", "")
	t.Setenv("DB_DROP_FAIL", "")
	t.Setenv("DB_EXISTS", "")
	t.Setenv("DB_OWNERSHIP_SNAPSHOT", "")
	writeExecutable(t, filepath.Join(bin, "createdb"), `#!/bin/sh
printf 'createdb %s\n' "$*" >> "$DB_MODE_CALLS"
case "$*" in
  *--template=*)
    if [ "$DB_TEMPLATE_FAIL" = 1 ]; then
      echo 'source database is being accessed by other users' >&2
      exit 1
    fi
    ;;
esac
printf '42\n' > "$DB_IDENTITY_DIR/$1"
exit 0
`)
	writeExecutable(t, filepath.Join(bin, "psql"), "#!/bin/sh\nprintf 'psql %s\\n' \"$*\" >> \"$DB_MODE_CALLS\"\n"+fakeDatabaseIdentityPSQL+`
if [ "$2" = postgres ]; then
  printf '%s\n' "$DB_EXISTS"
else
  cat >/dev/null
fi
`)
	writeExecutable(t, filepath.Join(bin, "pg_dump"), `#!/bin/sh
printf 'pg_dump %s\n' "$*" >> "$DB_MODE_CALLS"
if [ -n "$DB_OWNERSHIP_SNAPSHOT" ]; then
  cp "$KIT_STATE_DIR/config.toml" "$DB_OWNERSHIP_SNAPSHOT"
fi
if [ "$DB_DUMP_FAIL" = 1 ]; then exit 1; fi
printf 'fake SQL\n'
`)
	writeExecutable(t, filepath.Join(bin, "dropdb"), `#!/bin/sh
printf 'dropdb %s\n' "$*" >> "$DB_MODE_CALLS"
if [ "$DB_DROP_FAIL" = 1 ]; then exit 1; fi
rm -f "$DB_IDENTITY_DIR/$2"
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return capture
}

func deferredDatabaseLayout(t *testing.T) Layout {
	t.Helper()
	setStateDir(t)
	root := t.TempDir()
	l := Layout{Root: root, Master: filepath.Join(root, "master")}
	path := l.WorktreePath("feature")
	if err := writeIsolatedDatabaseEnv(path, DBName("feature")); err != nil {
		t.Fatal(err)
	}
	if err := WithConfigLock(func(c *Config) error {
		c.Worktrees["feature"] = WorktreeMeta{Slot: 1, Path: path, DatabaseDeferred: true}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestRunDressExplicitDatabaseModes(t *testing.T) {
	for _, mode := range []string{DatabaseModeNone, DatabaseModeEmpty, DatabaseModeCopy} {
		for _, sourceEnv := range []bool{false, true} {
			t.Run(mode+"/source-env="+map[bool]string{true: "yes", false: "no"}[sourceEnv], func(t *testing.T) {
				l, path := dressLayout(t)
				setStateDir(t)
				t.Setenv("KIT_RUN_DIR", t.TempDir())
				capture := stubModeDBCommands(t)
				if sourceEnv {
					if err := writeIsolatedDatabaseEnv(l.Master, "liftoff"); err != nil {
						t.Fatal(err)
					}
				}
				if err := runDressBlocking(l, DressPlan{Name: "fdb", Worktree: path, DatabaseMode: mode}); err != nil {
					t.Fatal(err)
				}
				name, found, err := DBNameFromEnv(path)
				if err != nil || !found || name != "liftoff_fdb" {
					t.Fatalf("isolated env = %q, found=%v err=%v", name, found, err)
				}
				cfg, err := LoadConfig()
				if err != nil {
					t.Fatal(err)
				}
				meta := cfg.Worktrees["fdb"]
				calls := readCapture(t, capture)
				if mode == DatabaseModeNone {
					if !meta.DatabaseDeferred || meta.DatabaseName != "" || calls != "" {
						t.Fatalf("none created or claimed a database: %+v calls=%q", meta, calls)
					}
				} else {
					if meta.DatabaseDeferred || meta.DatabaseName != "liftoff_fdb" {
						t.Fatalf("database not recorded ready: %+v", meta)
					}
					if strings.Contains(calls, "pg_dump") {
						t.Fatalf("unexpected dump: %s", calls)
					}
					if (mode == DatabaseModeCopy) != strings.Contains(calls, "--template=liftoff --strategy=file_copy") {
						t.Fatalf("unexpected creation strategy: %s", calls)
					}
				}
			})
		}
	}
}

func TestRunDressCopyNeverFallsBackWithoutPermission(t *testing.T) {
	l, path := dressLayout(t)
	setStateDir(t)
	capture := stubModeDBCommands(t)
	t.Setenv("DB_TEMPLATE_FAIL", "1")
	err := runDressBlocking(l, DressPlan{Name: "fdb", Worktree: path, DatabaseMode: DatabaseModeCopy})
	if err == nil || !strings.Contains(err.Error(), "no full dump attempted") {
		t.Fatalf("copy failure = %v", err)
	}
	calls := readCapture(t, capture)
	if strings.Contains(calls, "pg_dump") || strings.Contains(calls, "createdb liftoff_fdb\n") || strings.Contains(calls, "dropdb") {
		t.Fatalf("failed template triggered a dump or cleanup of an unowned database: %s", calls)
	}
}

func TestProvisionDatabaseModes(t *testing.T) {
	for _, tc := range []struct {
		name          string
		mode          string
		templateFails bool
		allowDump     bool
		wantErr       bool
	}{
		{"empty", DatabaseModeEmpty, false, false, false},
		{"template", DatabaseModeCopy, false, false, false},
		{"busy-no-dump", DatabaseModeCopy, true, false, true},
		{"busy-allowed-dump", DatabaseModeCopy, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := deferredDatabaseLayout(t)
			capture := stubModeDBCommands(t)
			if tc.templateFails {
				t.Setenv("DB_TEMPLATE_FAIL", "1")
			}
			snapshot := filepath.Join(t.TempDir(), "ownership")
			t.Setenv("DB_OWNERSHIP_SNAPSHOT", snapshot)
			err := l.ProvisionDatabase("feature", tc.mode, tc.allowDump, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("provision error=%v wantErr=%v", err, tc.wantErr)
			}
			calls := readCapture(t, capture)
			wantDump := tc.templateFails && tc.allowDump
			if strings.Contains(calls, "pg_dump") != wantDump {
				t.Fatalf("dump permission not respected: %s", calls)
			}
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			meta := cfg.Worktrees["feature"]
			if meta.DatabaseDeferred != tc.wantErr || (meta.DatabaseName == "") != tc.wantErr {
				t.Fatalf("incorrect ownership/readiness: %+v", meta)
			}
			if wantDump {
				data, err := os.ReadFile(snapshot)
				if err != nil || !strings.Contains(string(data), `database_name = "liftoff_feature"`) {
					t.Fatalf("ownership not durable before dump: %s err=%v", data, err)
				}
			}
		})
	}
}

func TestProvisionDatabaseRefusesUnsafeTargets(t *testing.T) {
	for _, condition := range []string{"owned", "shared-env", "existing-db", "not-deferred", "cleanup-pending", "claimed-by-other", "unknown-db-state", "identifier-overflow"} {
		t.Run(condition, func(t *testing.T) {
			l := deferredDatabaseLayout(t)
			capture := stubModeDBCommands(t)
			name := "feature"
			switch condition {
			case "identifier-overflow":
				name = strings.Repeat("a", 56)
				if err := writeIsolatedDatabaseEnv(l.WorktreePath("feature"), DBName(name)); err != nil {
					t.Fatal(err)
				}
				if err := WithConfigLock(func(c *Config) error {
					c.Worktrees[name] = c.Worktrees["feature"]
					delete(c.Worktrees, "feature")
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			case "shared-env":
				if err := writeIsolatedDatabaseEnv(l.WorktreePath("feature"), "liftoff"); err != nil {
					t.Fatal(err)
				}
			case "existing-db":
				t.Setenv("DB_EXISTS", "liftoff_feature")
			case "unknown-db-state":
				bin := t.TempDir()
				writeExecutable(t, filepath.Join(bin, "psql"), "#!/bin/sh\nexit 1\n")
				t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			default:
				if err := WithConfigLock(func(c *Config) error {
					meta := c.Worktrees["feature"]
					switch condition {
					case "owned":
						meta.DatabaseName = "liftoff_feature"
					case "not-deferred":
						meta.DatabaseDeferred = false
					case "cleanup-pending":
						meta.CleanupPending = true
					case "claimed-by-other":
						c.Worktrees["other"] = WorktreeMeta{DatabaseName: "liftoff_feature"}
					}
					c.Worktrees["feature"] = meta
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			err := l.ProvisionDatabase(name, DatabaseModeEmpty, false, nil)
			if err == nil {
				t.Fatal("unsafe provision was accepted")
			}
			if condition == "identifier-overflow" && !strings.Contains(err.Error(), "too long") {
				t.Fatalf("identifier overflow was not rejected at validation: %v", err)
			}
			calls := readCapture(t, capture)
			if strings.Contains(calls, "createdb") || strings.Contains(calls, "dropdb") || strings.Contains(calls, "pg_dump") {
				t.Fatalf("unsafe target mutated: %s", calls)
			}
		})
	}
}

func TestProvisionDatabaseRetainsRecoveryOnFailedCleanup(t *testing.T) {
	for _, dropFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleaned", true: "retained"}[dropFails], func(t *testing.T) {
			l := deferredDatabaseLayout(t)
			capture := stubModeDBCommands(t)
			t.Setenv("DB_TEMPLATE_FAIL", "1")
			t.Setenv("DB_DUMP_FAIL", "1")
			if dropFails {
				t.Setenv("DB_DROP_FAIL", "1")
			}
			if err := l.ProvisionDatabase("feature", DatabaseModeCopy, true, nil); err == nil {
				t.Fatal("failed copy succeeded")
			}
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			meta := cfg.Worktrees["feature"]
			if !meta.DatabaseDeferred || meta.CleanupPending != dropFails || (meta.DatabaseName != "") != dropFails {
				t.Fatalf("incorrect recovery metadata: %+v", meta)
			}
			if !strings.Contains(readCapture(t, capture), "dropdb --if-exists liftoff_feature") {
				t.Fatal("failed copy was not cleaned up")
			}
			if _, err := os.Stat(l.WorktreePath("feature")); err != nil {
				t.Fatalf("provision failure removed checkout: %v", err)
			}
		})
	}
}

func TestProvisionDatabaseSerializesConcurrentRequests(t *testing.T) {
	l := deferredDatabaseLayout(t)
	capture := stubModeDBCommands(t)
	var wg sync.WaitGroup
	var successes atomic.Int32
	for range 2 {
		wg.Go(func() {
			if l.ProvisionDatabase("feature", DatabaseModeEmpty, false, nil) == nil {
				successes.Add(1)
			}
		})
	}
	wg.Wait()
	if successes.Load() != 1 || strings.Count(readCapture(t, capture), "createdb ") != 1 {
		t.Fatalf("concurrent requests were not serialized: successes=%d calls=%s", successes.Load(), readCapture(t, capture))
	}
}

func TestRunPlayDeferredDatabaseFailsBeforeAnySideEffects(t *testing.T) {
	for _, svc := range []Service{SvcAPI, SvcAdminBE, SvcMCP, SvcCelery, SvcBeat} {
		t.Run(string(svc), func(t *testing.T) {
			l := deferredDatabaseLayout(t)
			setRunDir(t)
			var brokerCalls atomic.Int32
			old := rabbitmqctl
			rabbitmqctl = func(args ...string) ([]byte, error) {
				brokerCalls.Add(1)
				return nil, nil
			}
			t.Cleanup(func() { rabbitmqctl = old })
			var updates []PlayUpdate
			for u := range l.RunPlay(PlayPlan{Worktree: "feature", WorktreePath: l.WorktreePath("feature"), Services: []Service{SvcApp, svc}, ReplaceCelery: true, ReplaceVictim: "other"}) {
				updates = append(updates, u)
			}
			if len(updates) != 1 || updates[0].Status != StepFailed || updates[0].Err == nil || !strings.Contains(updates[0].Err.Error(), "kit db feature") {
				t.Fatalf("backend was not blocked before replacement/start: %+v", updates)
			}
			if brokerCalls.Load() != 0 {
				t.Fatal("deferred startup touched broker")
			}
		})
	}
}

func TestRunPlayDeferredDatabaseAllowsFrontendOnly(t *testing.T) {
	l := deferredDatabaseLayout(t)
	setRunDir(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	var updates []PlayUpdate
	for u := range l.RunPlay(PlayPlan{Worktree: "feature", Services: []Service{SvcApp}, Ports: Ports{App: port}}) {
		updates = append(updates, u)
	}
	if len(updates) != 1 || updates[0].Status != StepDone {
		t.Fatalf("frontend-only play was blocked: %+v", updates)
	}
}

func TestNewWorkspaceDatabaseIsUnreadyUntilBound(t *testing.T) {
	for _, mode := range []string{DatabaseModeShared, DatabaseModeNone, DatabaseModeEmpty, DatabaseModeCopy} {
		t.Run(mode, func(t *testing.T) {
			setStateDir(t)
			plan := DressPlan{Name: "feature", Worktree: t.TempDir(), DatabaseMode: mode}
			if err := persistDressCheckout(plan); err != nil {
				t.Fatal(err)
			}
			if err := requireProvisionedDatabase("feature", []Service{SvcAPI}); (err != nil) != (mode != DatabaseModeShared) {
				t.Fatalf("startup guard before env binding: %v", err)
			}
		})
	}
}

func TestResolveCleanupDBDeferredNeverClaimsExistingTarget(t *testing.T) {
	l := deferredDatabaseLayout(t)
	capture := stubModeDBCommands(t)
	t.Setenv("DB_EXISTS", "liftoff_feature")
	db, skip, err := resolveCleanupDB("feature", l.WorktreePath("feature"))
	if err != nil || !skip || db != "" {
		t.Fatalf("deferred target claimed: db=%q skip=%v err=%v", db, skip, err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Worktrees["feature"].DatabaseName != "" || readCapture(t, capture) != "" {
		t.Fatalf("deferred cleanup queried or persisted ownership: %+v calls=%s", cfg.Worktrees["feature"], readCapture(t, capture))
	}
}

func TestResolveCleanupDBDeferredRetainedOwnershipStillCleans(t *testing.T) {
	l := deferredDatabaseLayout(t)
	if err := WithConfigLock(func(c *Config) error {
		meta := c.Worktrees["feature"]
		meta.DatabaseName = "liftoff_feature"
		meta.CleanupPending = true
		c.Worktrees["feature"] = meta
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	db, skip, err := resolveCleanupDB("feature", l.WorktreePath("feature"))
	if err != nil || skip || db != "liftoff_feature" {
		t.Fatalf("retained owned DB not eligible for cleanup: db=%q skip=%v err=%v", db, skip, err)
	}
}
