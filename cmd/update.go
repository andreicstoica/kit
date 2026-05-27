package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

const (
	kitModulePath = "github.com/andreicstoica/kit"
	kitRepoURL    = "https://github.com/andreicstoica/kit"
)

var updateCheckOnly bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update kit to the latest released version",
	Long: "**update** fetches the newest tagged release and rebuilds kit in " +
		"place, overwriting the current binary (any install location). " +
		"Requires Go + network.\n\n" +
		"`--check` only reports whether an update is available.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cur := Version()
		latest, err := latestRemoteTag()
		if err != nil {
			return fmt.Errorf("check latest version: %w", err)
		}

		fmt.Printf("current: %s\nlatest:  %s\n", cur, latest)
		// Treat a -dirty / git-describe build (e.g. v0.1.4-3-gabc) as its base
		// tag so we don't "update" to the version we're already on.
		if baseVersion(cur) == latest {
			fmt.Println(tui.StyleOK.Render("✓ already up to date"))
			return nil
		}
		if updateCheckOnly {
			fmt.Println(tui.StyleWarn.Render("update available — run `kit update`"))
			return nil
		}

		self, err := os.Executable()
		if err != nil {
			return err
		}
		if resolved, err := filepath.EvalSymlinks(self); err == nil {
			self = resolved
		}

		// Cap the whole operation so a network/VCS stall can't hang forever.
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		// Clone the tag and build it directly, rather than `go install
		// module@tag` — the module proxy cold-fetches a fresh tag the first
		// time anyone requests it, which can stall for a minute. git clone is
		// direct and already fast (same path as the version check).
		srcDir, err := os.MkdirTemp("", "kit-src-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(srcDir)

		fmt.Printf("fetching %s…\n", latest)
		clone := exec.CommandContext(ctx, "git", "clone", "--quiet", "--depth", "1",
			"--branch", latest, kitRepoURL, srcDir)
		clone.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0") // never block on auth
		clone.Stdout = os.Stderr
		clone.Stderr = os.Stderr
		if err := clone.Run(); err != nil {
			return fmt.Errorf("clone %s failed: %w", latest, err)
		}

		// Build onto the same filesystem as the current binary so the final
		// swap is an atomic rename.
		built := filepath.Join(filepath.Dir(self), ".kit-update-"+latest)
		defer os.Remove(built)
		fmt.Println("building…")
		build := exec.CommandContext(ctx, "go", "build",
			"-ldflags", "-s -w -X "+kitModulePath+"/cmd.version="+latest,
			"-o", built, ".")
		build.Dir = srcDir
		build.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		build.Stdout = os.Stderr
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			return fmt.Errorf("build failed: %w", err)
		}

		// macOS Gatekeeper SIGKILLs unsigned freshly-built binaries; ad-hoc
		// re-sign clears the provenance flag (mirrors the Makefile).
		if runtime.GOOS == "darwin" {
			_ = exec.Command("codesign", "--force", "--sign", "-", built).Run()
		}

		if err := os.Rename(built, self); err != nil {
			return fmt.Errorf("replace %s (try a writable install dir or re-run with permission): %w", self, err)
		}
		fmt.Println(tui.StyleOK.Render(fmt.Sprintf("✓ updated %s → %s", cur, latest)))
		return nil
	},
}

// baseVersion strips a git-describe / -dirty suffix, returning just the tag
// portion (v0.1.4-3-gabc → v0.1.4, v0.1.4-dirty → v0.1.4).
func baseVersion(v string) string {
	if i := strings.Index(v, "-"); i >= 0 {
		return v[:i]
	}
	return v
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "report whether an update is available without installing")
	rootCmd.AddCommand(updateCmd)
}

// latestRemoteTag returns the highest semver tag on the remote (e.g. "v0.1.5"),
// without needing a local clone.
func latestRemoteTag() (string, error) {
	return latestRemoteTagCtx(context.Background())
}

// latestRemoteTagCtx is latestRemoteTag with a cancellation context, so the
// background update nudge can cap how long it waits on the network.
func latestRemoteTagCtx(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--refs",
		"--sort=-v:refname", kitRepoURL, "v*").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		_, ref, ok := strings.Cut(line, "refs/tags/")
		if ok && ref != "" {
			return strings.TrimSpace(ref), nil
		}
	}
	return "", fmt.Errorf("no tags found at %s", kitRepoURL)
}
