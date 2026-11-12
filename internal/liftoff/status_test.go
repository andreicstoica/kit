package liftoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHasDBs_EmptyBatchUsesOnePSQLInvocation(t *testing.T) {
	bin := t.TempDir()
	count := filepath.Join(bin, "count")
	writeExecutable(t, filepath.Join(bin, "psql"), "#!/bin/sh\necho call >> \"$COUNT_FILE\"\n# Successful empty batch result: no matching databases.\n")
	t.Setenv("COUNT_FILE", count)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := HasDBs([]string{"alpha", "beta", "gamma"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["alpha"] || got["beta"] || got["gamma"] {
		t.Fatalf("HasDBs() = %#v, want all three false", got)
	}
	calls, err := os.ReadFile(count)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(strings.Fields(string(calls))); n != 1 {
		t.Fatalf("psql invocation count = %d, want 1; calls: %q", n, calls)
	}
}

func TestHasDBs_UnavailablePSQLIsBounded(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "psql"), "#!/bin/sh\nexec sleep 10\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	started := time.Now()
	got, err := HasDBs([]string{"alpha", "beta"})
	if elapsed := time.Since(started); elapsed > psqlTimeout+time.Second {
		t.Fatalf("HasDBs took %s with unavailable psql; want bounded by %s", elapsed, psqlTimeout)
	}
	if err == nil || got != nil {
		t.Fatalf("HasDBs() = %#v, %v; unavailable database state must be reported as unknown", got, err)
	}
}
