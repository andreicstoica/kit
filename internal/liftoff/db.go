package liftoff

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const disconnectedTestDBsQuery = `
SELECT d.datname,
       EXTRACT(EPOCH FROM (pg_stat_file('base/' || d.oid)).modification)::bigint
FROM pg_database d
WHERE d.datname ~ '^liftoff_(test_db($|_)|e2e($|_)|.+_test_db$)'
  AND NOT EXISTS (
      SELECT 1 FROM pg_stat_activity a WHERE a.datname = d.datname
  )
ORDER BY d.datname`

// CloneDB pipes `pg_dump <src>` into `psql <dst>` for a fast local clone.
// Connection (host/port/user/password) is left to libpq's standard PG* env
// vars — consistent with createdb/dropdb/HasDB — so non-default postgres setups
// work without kit-specific knobs. Caller must `createdb dst` first.
func CloneDB(srcDB, dstDB string, onLine LineFn) error {
	dump := exec.Command("pg_dump", srcDB)
	psql := exec.Command("psql", "-d", dstDB)

	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	dump.Stdout = w
	psql.Stdin = r

	dumpErr, _ := dump.StderrPipe()
	psqlErr, _ := psql.StderrPipe()

	if err := dump.Start(); err != nil {
		w.Close()
		r.Close()
		return err
	}
	if err := psql.Start(); err != nil {
		w.Close()
		r.Close()
		return err
	}
	go drain(dumpErr, "pg_dump", onLine)
	go drain(psqlErr, "psql", onLine)

	dumpDone := make(chan error, 1)
	go func() {
		err := dump.Wait()
		w.Close()
		dumpDone <- err
	}()
	psqlErr2 := psql.Wait()
	r.Close()
	if psqlErr2 != nil {
		return fmt.Errorf("psql restore: %w", psqlErr2)
	}
	if err := <-dumpDone; err != nil {
		return fmt.Errorf("pg_dump: %w", err)
	}
	return nil
}

// CreateDB runs `createdb <name>`.
func CreateDB(name string, onLine LineFn) error {
	return RunStream("", "createdb", []string{name}, onLine)
}

// DropDB runs `dropdb <name>`. Returns nil if DB does not exist.
func DropDB(name string, onLine LineFn) error {
	err := RunStream("", "dropdb", []string{name}, onLine)
	if err != nil && strings.Contains(err.Error(), "does not exist") {
		return nil
	}
	return err
}

// SweepOldTestDBs drops disconnected test databases whose data has not changed
// within maxAge. dropdb performs a final connection check, so a database that
// becomes active during the sweep is preserved.
func SweepOldTestDBs(maxAge time.Duration) (int, []error) {
	config, err := LoadConfig()
	if err != nil {
		return 0, []error{err}
	}
	managed := make(map[string]bool, len(config.Worktrees))
	for name := range config.Worktrees {
		managed[DBName(name)] = true
	}

	out, err := Run("", "psql", "-d", "postgres", "-At", "-F", "\t", "-c", disconnectedTestDBsQuery)
	if err != nil {
		return 0, []error{err}
	}

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	var errs []error
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 || !isGeneratedTestDB(fields[0]) {
			continue
		}
		if managed[fields[0]] {
			continue
		}
		modified, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			errs = append(errs, fmt.Errorf("parse database mtime for %s: %w", fields[0], err))
			continue
		}
		if !time.Unix(modified, 0).Before(cutoff) {
			continue
		}
		if err := DropDB(fields[0], nil); err != nil {
			errs = append(errs, fmt.Errorf("drop database %s: %w", fields[0], err))
			continue
		}
		removed++
	}
	return removed, errs
}

func isGeneratedTestDB(name string) bool {
	if !strings.HasPrefix(name, "liftoff_") {
		return false
	}
	suffix := strings.TrimPrefix(name, "liftoff_")
	return suffix == "test_db" ||
		strings.HasPrefix(suffix, "test_db_") ||
		suffix == "e2e" ||
		strings.HasPrefix(suffix, "e2e_") ||
		strings.HasSuffix(suffix, "_test_db")
}

// HasPostgres returns true if pg_dump is on PATH.
func HasPostgres() bool {
	_, err := exec.LookPath("pg_dump")
	return err == nil
}

// drain reads r line-by-line, forwarding each line to onLine prefixed with tag.
func drain(r io.Reader, tag string, onLine LineFn) {
	if onLine == nil {
		_, _ = io.Copy(io.Discard, r)
		return
	}
	s := bufio.NewScanner(r)
	for s.Scan() {
		onLine(tag + ": " + s.Text())
	}
}
