package liftoff

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type DatabaseIdentity struct {
	ClusterID string `toml:"cluster_id"`
	OID       uint32 `toml:"oid"`
}

type DatabaseCreateIntent struct {
	Name      string `toml:"name"`
	ClusterID string `toml:"cluster_id"`
}

func sqlLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func databaseIdentity(target string) (DatabaseIdentity, error) {
	return lookupDatabaseIdentity("datname = " + sqlLiteral(target))
}

func lookupDatabaseIdentity(predicate string) (DatabaseIdentity, error) {
	query := "SELECT (pg_catalog.pg_control_system()).system_identifier::text, COALESCE((SELECT oid::text FROM pg_catalog.pg_database WHERE " + predicate + "), '')"
	ctx, cancel := context.WithTimeout(context.Background(), psqlTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "psql", "-d", "postgres", "-X", "-A", "-t", "-F", "\t", "-v", "ON_ERROR_STOP=1", "-c", query)
	out, err := cmd.Output()
	if err != nil {
		return DatabaseIdentity{}, fmt.Errorf("verify PostgreSQL identity (requires EXECUTE on pg_catalog.pg_control_system()): %w", err)
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\n"), "\t")
	if len(fields) != 2 {
		return DatabaseIdentity{}, errors.New("verify PostgreSQL identity: unexpected response")
	}
	cluster := strings.TrimSpace(fields[0])
	if _, err := strconv.ParseUint(cluster, 10, 64); err != nil || cluster == "0" {
		return DatabaseIdentity{}, errors.New("verify PostgreSQL identity: invalid cluster identifier")
	}
	identity := DatabaseIdentity{ClusterID: cluster}
	if oid := strings.TrimSpace(fields[1]); oid != "" {
		value, err := strconv.ParseUint(oid, 10, 32)
		if err != nil || value == 0 {
			return DatabaseIdentity{}, errors.New("verify PostgreSQL identity: invalid database identifier")
		}
		identity.OID = uint32(value)
	}
	return identity, nil
}

func uncertainDatabaseCreation(name string) error {
	return fmt.Errorf("database creation for %s is unverified; Kit will not claim or drop it. Inspect the recorded target on the original PostgreSQL cluster, resolve it manually, then remove database_create_intent from %s before retrying", name, ConfigPath())
}

func beginDatabaseCreation(name string) error {
	target := DBName(name)
	identity, err := databaseIdentity(target)
	if err != nil {
		return err
	}
	if identity.OID != 0 {
		return fmt.Errorf("database for %s already exists without ownership; refusing to overwrite it", name)
	}
	return WithConfigLock(func(c *Config) error {
		meta := c.Worktrees[name]
		if meta.DatabaseCreateIntent != nil {
			return uncertainDatabaseCreation(name)
		}
		if meta.DatabaseName != "" || meta.DatabaseIdentity != nil {
			return fmt.Errorf("%s already owns a database; refusing to overwrite it", name)
		}
		meta.DatabaseDeferred = true
		meta.DatabaseCreateIntent = &DatabaseCreateIntent{Name: target, ClusterID: identity.ClusterID}
		c.Worktrees[name] = meta
		return nil
	})
}

func createOwnedDatabase(name, mode string, allowPGDump bool, onLine LineFn) (bool, error) {
	if err := beginDatabaseCreation(name); err != nil {
		return false, err
	}
	copied, err := createDatabase(DBName(name), mode, allowPGDump, onLine)
	if err != nil {
		return false, errors.Join(err, clearAbsentDatabaseIntent(name))
	}
	if err := finishDatabaseCreation(name); err != nil {
		return false, errors.Join(err, uncertainDatabaseCreation(name))
	}
	return copied, nil
}

func clearAbsentDatabaseIntent(name string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return errors.Join(err, uncertainDatabaseCreation(name))
	}
	intent := cfg.Worktrees[name].DatabaseCreateIntent
	if intent == nil {
		return uncertainDatabaseCreation(name)
	}
	identity, err := databaseIdentity(intent.Name)
	if err != nil || identity.ClusterID != intent.ClusterID || identity.OID != 0 {
		return errors.Join(err, uncertainDatabaseCreation(name))
	}
	return WithConfigLock(func(c *Config) error {
		meta := c.Worktrees[name]
		if meta.DatabaseCreateIntent == nil || *meta.DatabaseCreateIntent != *intent {
			return uncertainDatabaseCreation(name)
		}
		meta.DatabaseCreateIntent = nil
		c.Worktrees[name] = meta
		return nil
	})
}

func finishDatabaseCreation(name string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	intent := cfg.Worktrees[name].DatabaseCreateIntent
	if intent == nil {
		return uncertainDatabaseCreation(name)
	}
	identity, err := databaseIdentity(intent.Name)
	if err != nil {
		return err
	}
	if identity.ClusterID != intent.ClusterID || identity.OID == 0 {
		return uncertainDatabaseCreation(name)
	}
	return WithConfigLock(func(c *Config) error {
		meta := c.Worktrees[name]
		if meta.DatabaseCreateIntent == nil || *meta.DatabaseCreateIntent != *intent {
			return uncertainDatabaseCreation(name)
		}
		meta.DatabaseName = intent.Name
		meta.DatabaseIdentity = &identity
		meta.DatabaseCreateIntent = nil
		c.Worktrees[name] = meta
		return nil
	})
}

func verifyOwnedDatabase(name, target string, meta WorktreeMeta) (absent bool, err error) {
	if meta.DatabaseCreateIntent != nil {
		return false, uncertainDatabaseCreation(name)
	}
	if meta.DatabaseIdentity == nil {
		return false, nil
	}
	if meta.DatabaseName != target || meta.DatabaseIdentity.ClusterID == "" || meta.DatabaseIdentity.OID == 0 {
		return false, fmt.Errorf("incomplete PostgreSQL ownership for %s; refusing cleanup", name)
	}
	current, err := databaseIdentity(target)
	if err != nil {
		return false, err
	}
	if current.ClusterID != meta.DatabaseIdentity.ClusterID {
		return false, fmt.Errorf("PostgreSQL cluster changed for %s; restore the original connection before cleanup", name)
	}
	if current.OID != 0 && current.OID != meta.DatabaseIdentity.OID {
		return false, fmt.Errorf("PostgreSQL database identity changed for %s; refusing to drop a replacement database", name)
	}
	if current.OID == 0 {
		renamed, err := lookupDatabaseIdentity(fmt.Sprintf("oid = %d AND datname <> %s", meta.DatabaseIdentity.OID, sqlLiteral(target)))
		if err != nil {
			return false, err
		}
		if renamed.ClusterID != meta.DatabaseIdentity.ClusterID || renamed.OID != 0 {
			return false, fmt.Errorf("PostgreSQL database was renamed or its cluster changed for %s; inspect the recorded ownership before cleanup", name)
		}
	}
	return current.OID == 0, nil
}

func dropOwnedDatabase(name, target string, onLine LineFn) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	meta := cfg.Worktrees[name]
	if meta.DatabaseCreateIntent != nil {
		return uncertainDatabaseCreation(name)
	}
	if meta.DatabaseName != target || target == "liftoff" || target == "" {
		return fmt.Errorf("cannot verify database ownership for %s; refusing cleanup", name)
	}
	if meta.DatabaseIdentity == nil {
		// Existing configs predate cluster identities; keep their proven-name cleanup path.
		if onLine != nil {
			onLine("legacy database ownership has no PostgreSQL identity; using recorded-name compatibility cleanup")
		}
		return DropDB(target, onLine)
	}
	identity := meta.DatabaseIdentity
	if identity.ClusterID == "" || identity.OID == 0 {
		return fmt.Errorf("incomplete PostgreSQL ownership for %s; refusing cleanup", name)
	}
	// Check and drop in one libpq session so ambient connection changes cannot retarget dropdb.
	check := fmt.Sprintf("DO $kit$ BEGIN IF (SELECT system_identifier::text FROM pg_catalog.pg_control_system()) <> %s OR EXISTS (SELECT 1 FROM pg_catalog.pg_database WHERE datname = %s AND oid <> %d) OR EXISTS (SELECT 1 FROM pg_catalog.pg_database WHERE oid = %d AND datname <> %s) THEN RAISE EXCEPTION 'Kit PostgreSQL ownership changed; refusing database cleanup'; END IF; END $kit$", sqlLiteral(identity.ClusterID), sqlLiteral(target), identity.OID, identity.OID, sqlLiteral(target))
	drop := "DROP DATABASE IF EXISTS \"" + strings.ReplaceAll(target, "\"", "\"\"") + "\""
	return RunStream("", "psql", []string{"-d", "postgres", "-X", "-v", "ON_ERROR_STOP=1", "-c", check, "-c", drop}, onLine)
}
