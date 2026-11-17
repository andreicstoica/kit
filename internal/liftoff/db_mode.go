package liftoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	databaseLockNamespace = "database:"
	DatabaseModeShared    = ""
	DatabaseModeNone      = "none"
	DatabaseModeEmpty     = "empty"
	DatabaseModeCopy      = "copy"
)

func validateDatabaseOptions(mode string, allowPGDump bool) error {
	switch mode {
	case DatabaseModeShared, DatabaseModeNone, DatabaseModeEmpty, DatabaseModeCopy:
	default:
		return fmt.Errorf("unknown database mode %q (want none, empty, or copy)", mode)
	}
	if allowPGDump && mode != DatabaseModeCopy {
		return errors.New("--allow-pg-dump requires database mode copy")
	}
	return nil
}

// createDatabase reports whether the new database already contains the source data.
// The caller must persist ownership before running a dump into an empty target.
func createDatabase(name, mode string, allowPGDump bool, onLine LineFn) (bool, error) {
	if mode == DatabaseModeCopy {
		if err := createDBByTemplate(name, "liftoff", onLine); err == nil {
			return true, nil
		} else if !allowPGDump {
			return false, fmt.Errorf("%w; no full dump attempted (use --allow-pg-dump to opt in)", err)
		} else if onLine != nil {
			onLine("template copy unavailable, using explicitly allowed pg_dump: " + err.Error())
		}
	}
	if err := CreateDB(name, onLine); err != nil {
		return false, err
	}
	return false, nil
}

func writeIsolatedDatabaseEnv(path, target string) error {
	backend := filepath.Join(path, "backend")
	envPath := filepath.Join(backend, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(backend, 0o755); err != nil {
		return err
	}
	return os.WriteFile(envPath, []byte(replaceEnvKey(string(data), "SQLALCHEMY_DATABASE_NAME", target)), 0o600)
}

func validateDatabaseName(name string) error {
	if err := Validate(name); err != nil {
		return err
	}
	if len(DBName(name)) > 63 {
		return errors.New("workspace name too long for an isolated database (max 55 characters)")
	}
	return nil
}

func (c *Config) validateDatabaseOwner(name string) error {
	if c.Worktrees[name].DatabaseCreateIntent != nil {
		return uncertainDatabaseCreation(name)
	}
	if c.Worktrees[name].DatabaseName != "" || c.Worktrees[name].DatabaseIdentity != nil {
		return fmt.Errorf("%s already owns a database; refusing to overwrite it", name)
	}
	target := DBName(name)
	for other, meta := range c.Worktrees {
		if other == name {
			continue
		}
		claimed := meta.DatabaseName
		if claimed == "" {
			claimed = DBName(other)
		}
		if claimed == target {
			return fmt.Errorf("database for %s is reserved by workspace %s", name, other)
		}
	}
	return nil
}

func (l Layout) ProvisionDatabase(name, mode string, allowPGDump bool, onLine LineFn) error {
	if err := validateDatabaseName(name); err != nil {
		return err
	}
	if err := validateDatabaseOptions(mode, allowPGDump); err != nil {
		return err
	}
	if mode != DatabaseModeEmpty && mode != DatabaseModeCopy {
		return errors.New("provisioning requires --mode empty or --mode copy")
	}
	return WithWorkspaceLock(databaseLockNamespace+name, func() error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		if err := cfg.validateDatabaseOwner(name); err != nil {
			return err
		}
		meta, ok := cfg.Worktrees[name]
		if !ok || !meta.DatabaseDeferred || meta.CleanupPending {
			return fmt.Errorf("%s has no deferred database ready for provisioning", name)
		}
		path, err := l.ResolveWorktreePath(name)
		if err != nil {
			return err
		}
		target := DBName(name)
		envDB, found, err := DBNameFromEnv(path)
		if err != nil {
			return err
		}
		if !found || envDB != target {
			return fmt.Errorf("%s backend/.env does not point at its isolated database; refusing to provision", name)
		}
		copied, err := createOwnedDatabase(name, mode, allowPGDump, onLine)
		if err != nil {
			return err
		}
		if mode == DatabaseModeCopy && !copied {
			if err := CloneDB("liftoff", target, onLine); err != nil {
				return rollbackProvisionDatabase(name, target, err)
			}
		}
		if err := l.UpdateBackendDBName(path, target); err != nil {
			return rollbackProvisionDatabase(name, target, err)
		}
		if err := WithConfigLock(func(c *Config) error {
			meta := c.Worktrees[name]
			meta.DatabaseDeferred = false
			c.Worktrees[name] = meta
			return nil
		}); err != nil {
			return rollbackProvisionDatabase(name, target, err)
		}
		return nil
	})
}

func rollbackProvisionDatabase(name, target string, cause error) error {
	dropErr := dropOwnedDatabase(name, target, nil)
	persistErr := WithConfigLock(func(c *Config) error {
		meta := c.Worktrees[name]
		meta.DatabaseDeferred = true
		if dropErr != nil {
			meta.CleanupPending = true
		} else {
			meta.DatabaseName = ""
			meta.DatabaseIdentity = nil
		}
		c.Worktrees[name] = meta
		return nil
	})
	if dropErr != nil {
		dropErr = fmt.Errorf("database cleanup failed; retaining recovery ownership: %w", dropErr)
	}
	return errors.Join(cause, dropErr, persistErr)
}

func requireProvisionedDatabase(name string, services []Service) error {
	if !needsBroker(services) {
		return nil
	}
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Worktrees[name].DatabaseCreateIntent != nil {
		return uncertainDatabaseCreation(name)
	}
	if cfg.Worktrees[name].DatabaseDeferred {
		return fmt.Errorf("%s has an unprovisioned database; run `kit db %s --mode empty` or `kit db %s --mode copy` before starting backend services", name, name, name)
	}
	return nil
}
