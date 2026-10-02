package liftoff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWashRemoval_IsIdempotent(t *testing.T) {
	l := newMasterRepo(t)
	path := addWorktree(t, l, "gone")
	if err := l.RemoveWorktree(path, nil); err != nil {
		t.Fatal(err)
	}
	if err := l.DeleteBranch("gone", nil); err != nil {
		t.Fatal(err)
	}
	if err := l.RemoveWorktree(path, nil); err != nil {
		t.Errorf("retry removing absent worktree: %v", err)
	}
	if err := l.DeleteBranch("gone", nil); err != nil {
		t.Errorf("retry deleting absent branch: %v", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep.txt"), []byte("unregistered data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := l.RemoveWorktree(path, nil); err == nil {
		t.Error("unregistered existing folder must report an error")
	}
	if _, err := os.Stat(filepath.Join(path, "keep.txt")); err != nil {
		t.Fatalf("unregistered folder was removed: %v", err)
	}
}

func TestMarkCleanupPending_RecordsUnadoptedWorktree(t *testing.T) {
	setStateDir(t)
	p := WashPlan{Name: "unadopted", Branch: "feature/work", WorktreePath: "/tmp/work"}
	if err := markCleanupPending(p); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta, ok := cfg.Worktrees[p.Name]
	if !ok || !meta.CleanupPending || meta.Path != p.WorktreePath || meta.Branch != p.Branch {
		t.Fatalf("cleanup intent = %+v, exists=%v; want durable retry record", meta, ok)
	}
}

func TestRunWash_RejectsChangedCandidate(t *testing.T) {
	for _, change := range []string{"commit", "untracked"} {
		t.Run(change, func(t *testing.T) {
			l := newMasterRepo(t)
			setStateDir(t)
			t.Setenv("KIT_RUN_DIR", t.TempDir())
			path := addWorktree(t, l, "changed")
			head := runGit(t, path, "rev-parse", "HEAD")
			writeFile(t, path, "new.txt", "new work after selection")
			if change == "commit" {
				runGit(t, path, "add", ".")
				runGit(t, path, "commit", "-m", "new work")
			}
			if err := l.RunWashBlocking(WashPlan{Name: "changed", Branch: "changed", WorktreePath: path, ExpectedHead: head, RequireClean: true}); err == nil {
				t.Fatal("wash accepted a changed cleanup candidate")
			}
			if _, err := os.Stat(filepath.Join(path, "new.txt")); err != nil {
				t.Fatalf("new local work was removed: %v", err)
			}
			runGit(t, l.Master, "rev-parse", "changed")
		})
	}
}

func TestBranchForDelete(t *testing.T) {
	cases := []struct {
		name string
		plan WashPlan
		want string
	}{
		{
			name: "uses Branch when set",
			plan: WashPlan{Name: "voice-agent", Branch: "acs/voice-agent-cleanup"},
			want: "acs/voice-agent-cleanup",
		},
		{
			name: "falls back to Name when Branch empty",
			plan: WashPlan{Name: "voice-agent"},
			want: "voice-agent",
		},
		{
			name: "Branch is identical to Name",
			plan: WashPlan{Name: "feat", Branch: "feat"},
			want: "feat",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := branchForDelete(tc.plan); got != tc.want {
				t.Fatalf("branchForDelete(%+v) = %q, want %q", tc.plan, got, tc.want)
			}
		})
	}
}
