package liftoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubTwig(t *testing.T, body string) {
	t.Helper()
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "git-twig"), body)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCheckoutBackendValidation(t *testing.T) {
	for _, tc := range []struct {
		version string
		valid   bool
	}{
		{"git-twig 0.3.0", false},
		{"git-twig 0.4.0", true},
		{"git-twig 0.4.1", true},
		{"git-twig 1.0.0", true},
		{"git-twig 0.4.0-dev", false},
		{"unexpected output", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			stubTwig(t, "#!/bin/sh\necho '"+tc.version+"'\n")
			err := ValidateCheckoutBackend("twig")
			if (err == nil) != tc.valid {
				t.Fatalf("version %q: err=%v, valid=%v", tc.version, err, tc.valid)
			}
		})
	}
	if err := ValidateCheckoutBackend("typo"); err == nil {
		t.Fatal("unknown backend accepted")
	}
	for _, backend := range []string{"", "git"} {
		if err := ValidateCheckoutBackend(backend); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRunDressTwigPreflightHasNoSideEffects(t *testing.T) {
	l, path := dressLayout(t)
	setStateDir(t)
	stubTwig(t, "#!/bin/sh\necho git-twig 0.3.0\n")
	err := runDressBlocking(l, DressPlan{Name: "fdb", Worktree: path, CheckoutBackend: "twig", DatabaseMode: DatabaseModeCopy, AllowPGDump: true})
	if err == nil || !strings.Contains(err.Error(), "0.4.0") {
		t.Fatalf("expected actionable version error, got %v", err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("preflight created a checkout")
	}
	cfg, err := LoadConfig()
	if err != nil || len(cfg.Worktrees) != 0 {
		t.Fatalf("preflight changed config: %+v, %v", cfg, err)
	}
}

func TestRunDressTwigPartialFailurePreservesRecovery(t *testing.T) {
	l, path := dressLayout(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	dropped := filepath.Join(t.TempDir(), "dropped")
	stubDBBins(t, "", "", "", dropped)
	stubTwig(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo git-twig 0.4.0
  exit 0
fi
shift
git worktree add --no-checkout "$@" || exit 1
mkdir -p .git/twigs/staging/failed-attempt
echo diagnostic > .git/twigs/staging/failed-attempt/marker
echo 'simulated failure after registration' >&2
exit 1
`)
	err := runDressBlocking(l, DressPlan{Name: "fdb", Worktree: path, CheckoutBackend: "twig", DatabaseMode: DatabaseModeCopy, AllowPGDump: true})
	if err == nil || !strings.Contains(err.Error(), "do not retry") {
		t.Fatalf("expected recovery instructions, got %v", err)
	}
	wts, err := l.ListWorktrees()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := NewWorktreeSet(wts).At(path); !ok {
		t.Fatal("partial registration was removed")
	}
	if _, err := os.Stat(filepath.Join(l.Master, ".git/twigs/staging/failed-attempt/marker")); err != nil {
		t.Fatalf("diagnostic staging was removed: %v", err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	meta := cfg.Worktrees["fdb"]
	if !meta.CleanupPending || meta.Path != path || meta.Branch != "fdb" {
		t.Fatalf("partial checkout lost recovery metadata: %+v", meta)
	}
	if !strings.Contains(readCapture(t, dropped), "liftoff_fdb") {
		t.Fatal("concurrent database creation was not rolled back")
	}
}

func TestRunDressTwigRefusalDoesNotClaimExistingBranch(t *testing.T) {
	l, path := dressLayout(t)
	runGit(t, l.Master, "branch", "fdb")
	setStateDir(t)
	stubTwig(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo git-twig 0.4.0
  exit 0
fi
echo 'must not invoke Twig creation' >&2
exit 1
`)
	if err := runDressBlocking(l, DressPlan{Name: "fdb", Worktree: path, CheckoutBackend: "twig"}); err == nil {
		t.Fatal("existing branch accepted")
	}
	cfg, err := LoadConfig()
	if err != nil || len(cfg.Worktrees) != 0 {
		t.Fatalf("existing branch incorrectly claimed: %+v, %v", cfg, err)
	}
	if runGit(t, l.Master, "branch", "--list", "fdb") == "" {
		t.Fatal("existing branch was removed")
	}
}

func TestRunDressTwigFailureDoesNotRetryWithGit(t *testing.T) {
	l, path := dressLayout(t)
	setStateDir(t)
	stubTwig(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo git-twig 0.4.0
  exit 0
fi
echo 'simulated refusal before registration' >&2
exit 1
`)
	if err := runDressBlocking(l, DressPlan{Name: "fdb", Worktree: path, CheckoutBackend: "twig"}); err == nil {
		t.Fatal("Twig failure was swallowed")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("failed Twig creation retried with Git")
	}
	cfg, err := LoadConfig()
	if err != nil || len(cfg.Worktrees) != 0 {
		t.Fatalf("resource-free failure left recovery state: %+v, %v", cfg, err)
	}
}

func TestRunDressTwigBranchHookFailurePreservesCheckout(t *testing.T) {
	l, path := dressLayout(t)
	setStateDir(t)
	t.Setenv("KIT_RUN_DIR", t.TempDir())
	// The hook is installed after the detached checkout, so only branch setup fails.
	stubTwig(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo git-twig 0.4.0
  exit 0
fi
shift
git worktree add "$@" || exit 1
mkdir -p .git/hooks
printf '#!/bin/sh\nexit 1\n' > .git/hooks/post-checkout
chmod +x .git/hooks/post-checkout
`)
	if err := runDressBlocking(l, DressPlan{Name: "fdb", Worktree: path, CheckoutBackend: "twig"}); err == nil {
		t.Fatal("branch hook failure was swallowed")
	}
	if got := runGit(t, path, "branch", "--show-current"); got != "fdb" {
		t.Fatalf("partial branch not retained: %s", got)
	}
	cfg, err := LoadConfig()
	if err != nil || !cfg.Worktrees["fdb"].CleanupPending {
		t.Fatalf("branch setup failure lost recovery state: %+v, %v", cfg, err)
	}
}

func TestTwigLiveWorktree(t *testing.T) {
	if os.Getenv("KIT_TEST_TWIG") != "1" {
		t.Skip("set KIT_TEST_TWIG=1 to test installed Twig")
	}
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "matching", true: "stale"}[stale], func(t *testing.T) {
			l := newMasterRepo(t)
			target := runGit(t, l.Master, "rev-parse", "origin/master")
			if stale {
				writeFile(t, l.Master, "new.txt", "upstream")
				runGit(t, l.Master, "add", ".")
				runGit(t, l.Master, "commit", "-m", "advance upstream")
				runGit(t, l.Master, "push", "origin", l.MainBranch)
				target = runGit(t, l.Master, "rev-parse", "HEAD")
				runGit(t, l.Master, "checkout", "--detach", "HEAD~1")
			}
			path := filepath.Join(filepath.Dir(l.Master), "twig-live")
			setStateDir(t)
			t.Setenv("KIT_RUN_DIR", t.TempDir())
			var lines []string
			for u := range l.RunDress(DressPlan{Name: "twig-live", Worktree: path, CheckoutBackend: "twig"}) {
				lines = append(lines, u.Line)
				if u.Status == StepFailed {
					t.Errorf("design failed: %v", u.Err)
				}
			}
			if t.Failed() {
				t.FailNow()
			}
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			meta := cfg.Worktrees["twig-live"]
			if meta.Path != path || meta.Branch != "twig-live" || meta.Slot <= 0 || meta.CleanupPending {
				t.Fatalf("design lost workspace metadata: %+v", meta)
			}
			if got := runGit(t, path, "rev-parse", "HEAD"); got != target {
				t.Fatalf("created %s, want upstream %s", got, target)
			}
			if got := runGit(t, path, "status", "--porcelain"); got != "" {
				t.Fatalf("new worktree is dirty: %s", got)
			}
			if got := runGit(t, path, "rev-parse", "--abbrev-ref", "@{upstream}"); got != "origin/master" {
				t.Fatalf("tracking changed: %s", got)
			}
			writeFile(t, path, "README.md", "private edit")
			if got, err := os.ReadFile(filepath.Join(l.Master, "README.md")); err != nil || string(got) != "init" {
				t.Fatalf("write changed source: %q, %v", got, err)
			}
			if stale && !strings.Contains(strings.Join(lines, "\n"), "stock Git") {
				t.Fatalf("fallback not reported: %v", lines)
			}
			if err := l.RemoveWorktree(path, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}
