package liftoff

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func mockNoPRs(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "gh"), `#!/bin/sh
if [ "$1" = repo ]; then
  printf '{"nameWithOwner":"test/repo"}'
else
  printf '[[]]'
fi
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRemotePRStatuses_HistoryAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, entries, want string
		number              int
		wantErr             bool
	}{
		{"no PR", `[[]]`, "NONE", 0, false},
		{"latest closed", `[[{"number":2,"state":"closed","head":{"ref":"work","sha":"abc","repo":{"full_name":"test/repo"}}},{"number":1,"state":"closed","merged_at":"2026-01-01T00:00:00Z","head":{"ref":"work","repo":{"full_name":"test/repo"}}}]]`, "CLOSED", 2, false},
		{"open on later page", `[[{"number":2,"state":"closed","head":{"ref":"work","repo":{"full_name":"test/repo"}}}],[{"number":1,"state":"open","head":{"ref":"work","repo":{"full_name":"test/repo"}}}]]`, "OPEN", 1, false},
		{"fork ignored", `[[{"number":3,"state":"closed","head":{"ref":"work","repo":{"full_name":"other/repo"}}},{"number":2,"state":"open","head":{"ref":"work","repo":{"full_name":"test/repo"}}}]]`, "OPEN", 2, false},
		{"malformed", `invalid JSON`, "UNKNOWN", 0, true},
		{"invalid state", `[[{"number":3,"state":"unexpected","head":{"ref":"work","repo":{"full_name":"test/repo"}}}]]`, "UNKNOWN", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := Layout{Master: t.TempDir()}
			mockPRLookup(t, "work", "abc", tc.entries)
			got, err := l.RemotePRStatuses([]string{"work", "work", ""})
			if (err != nil) != tc.wantErr {
				t.Fatalf("lookup error = %v, want error=%v", err, tc.wantErr)
			}
			if len(got) != 1 || got["work"].State != tc.want || got["work"].Number != tc.number {
				t.Fatalf("statuses = %+v, want work #%d %s", got, tc.number, tc.want)
			}
		})
	}
}

func TestFindMergedWorktrees_RequiresPRHeadMatch(t *testing.T) {
	for _, state := range []string{"closed", "merged"} {
		t.Run(state, func(t *testing.T) {
			l := newMasterRepo(t)
			path := addWorktree(t, l, "work")
			writeFile(t, path, "work.txt", "submitted")
			runGit(t, path, "add", ".")
			runGit(t, path, "commit", "-m", "submitted")
			head := runGit(t, path, "rev-parse", "HEAD")
			mergedAt := ""
			if state == "merged" {
				mergedAt = "2026-01-01T00:00:00Z"
			}
			mockPRLookup(t, "work", head, fmt.Sprintf(`[[{"number":1,"state":"closed","merged_at":"%s","head":{"ref":"work","sha":"%s","repo":{"full_name":"test/repo"}}}]]`, mergedAt, head))
			got, err := l.FindMergedWorktrees()
			if err != nil || len(got) != 1 {
				t.Fatalf("matching PR head candidates = %+v, %v; want one", got, err)
			}
			writeFile(t, path, "new.txt", "new local work")
			runGit(t, path, "add", ".")
			runGit(t, path, "commit", "-m", "new work after PR")
			got, err = l.FindMergedWorktrees()
			if err != nil || len(got) != 0 {
				t.Fatalf("new local commit candidates = %+v, %v; want none", got, err)
			}
		})
	}
}
