package liftoff

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// CloneDB pipes `pg_dump <src>` into `psql <dst>` for a fast local clone.
// Both DBs assumed local on default port. Caller must `createdb dst` first.
func CloneDB(srcDB, dstDB string, onLine LineFn) error {
	src := "postgresql://localhost:5432/" + srcDB
	dst := "postgresql://localhost:5432/" + dstDB

	dump := exec.Command("pg_dump", src)
	psql := exec.Command("psql", dst)

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
