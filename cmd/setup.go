package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

const liftoffMasterRepoURL = "https://github.com/liftoff-inc/liftoff-app.git"
const brewInstallScript = `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`
const brewFenceComment = "# kit-setup: brew shellenv"
const kitPathFenceComment = "# kit-setup: kit on PATH"

var setupDryRun bool

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install missing tools and bootstrap your kit environment",
	Long: "**setup** runs the same checks as `kit doctor`, then offers to fix each " +
		"failure: installs missing tools via Homebrew, runs `gh auth login`, clones " +
		"the Liftoff master repo, and installs frontend packages so new " +
		"workspaces start faster.\n\n" +
		"You'll be asked before anything is changed. Setup is idempotent — re-run " +
		"any time.\n\n" +
		"Pass `--dry-run` (or `-n`) to walk the flow and see what setup would do " +
		"without actually changing anything.",
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().BoolVarP(&setupDryRun, "dry-run", "n", false, "preview actions without running them")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	layout := liftoff.DefaultLayout()

	banner := "kit setup — check & install"
	if setupDryRun {
		banner += "  (dry run)"
	}
	fmt.Println(tui.StyleTitle.Render(banner))
	if setupDryRun {
		fmt.Println(tui.StyleDim.Render("nothing will be changed — preview only."))
	} else {
		fmt.Println(tui.StyleDim.Render("nothing is changed without your confirmation."))
	}
	fmt.Println()

	results := liftoff.RunChecks(liftoff.DefaultChecks(layout))
	fmt.Print(tui.RenderDoctor(results))
	fmt.Println()

	if setupDryRun {
		printDryRunPlan(layout, results)
		return nil
	}

	// kit migrated its diff viewer from lumen to hunk — remove the stale tool.
	if err := offerLumenRemoval(); err != nil {
		fmt.Println(tui.StyleErr.Render("error: " + err.Error()))
	}

	applied := 0
	pendingRestart := map[string]bool{} // check IDs whose fix needs a new shell
	for _, r := range results {
		if r.Status == liftoff.CheckOK || r.Status == liftoff.CheckSkip {
			continue
		}
		restart, err := applyFix(layout, r)
		if err != nil {
			fmt.Println(tui.StyleErr.Render("error: " + err.Error()))
		}
		if restart {
			pendingRestart[r.ID] = true
		}
		applied++
	}

	if applied > 0 {
		fmt.Println()
		fmt.Println(tui.StyleTitle.Render("re-checking…"))
		results = liftoff.RunChecks(liftoff.DefaultChecks(layout))
		fmt.Print(tui.RenderDoctor(results))
	}

	// PATH fixes only load in a new shell, so the in-process re-check still
	// reports them as failing — that's expected, not a real failure. Surface a
	// restart hint and exclude them from the exit-1 gate.
	if anyPendingStillFailing(results, pendingRestart) {
		fmt.Println(tui.StyleWarn.Render("PATH updated — restart your terminal or run `source " + liftoff.ShellProfilePath() + "` for it to take effect."))
	}
	if failedExcluding(results, pendingRestart) {
		fmt.Println(tui.StyleWarn.Render("still has failures — see hints above or re-run `kit setup`."))
		os.Exit(1)
	}

	// Persist what setup learned to config.toml.
	if err := persistSetupSettings(layout); err != nil {
		fmt.Println(tui.StyleDim.Render("(could not save settings to config.toml: " + err.Error() + ")"))
	}

	// Offer bulk-adopt for unmanaged worktrees.
	adopted, err := offerBulkAdopt(layout)
	if err != nil {
		fmt.Println(tui.StyleDim.Render("(adoption skipped: " + err.Error() + ")"))
	}

	fmt.Println()
	if adopted > 0 {
		fmt.Println(tui.StyleOK.Render("✓ existing workspaces ready to go."))
		fmt.Println(tui.StyleDim.Render("to make a new workspace, try ") + tui.Code("kit design my-first-kit"))
	} else {
		fmt.Println(tui.StyleOK.Render("✓ ready to go.") + tui.StyleDim.Render(" make a new workspace with ") + tui.Code("kit design my-first-kit"))
	}
	return nil
}

// anyPendingStillFailing reports whether any pending-restart check still shows
// as failing on the re-check (the expected state until the shell reloads).
func anyPendingStillFailing(results []liftoff.CheckResult, pending map[string]bool) bool {
	for _, r := range results {
		if pending[r.ID] && r.Status == liftoff.CheckFail {
			return true
		}
	}
	return false
}

// failedExcluding reports whether any check failed, ignoring pending-restart
// checks (PATH fixes the running process can't see yet).
func failedExcluding(results []liftoff.CheckResult, pending map[string]bool) bool {
	for _, r := range results {
		if r.Status == liftoff.CheckFail && !pending[r.ID] {
			return true
		}
	}
	return false
}

// persistSetupSettings writes Root + MasterDir + first installed editor
// into config.Settings. Merges with existing settings — user-edited
// fields aren't clobbered (only empty fields are filled in).
func persistSetupSettings(layout liftoff.Layout) error {
	return liftoff.WithConfigLock(func(c *liftoff.Config) error {
		if c.Settings.Root == "" {
			c.Settings.Root = layout.Root
		}
		if c.Settings.MasterDir == "" {
			// Derive MasterDir from layout.Master minus layout.Root prefix.
			if rel := relativeDir(layout.Root, layout.Master); rel != "" {
				c.Settings.MasterDir = rel
			}
		}
		if c.Settings.Editor == "" {
			if eds := liftoff.InstalledEditors(); len(eds) > 0 {
				c.Settings.Editor = eds[0].Binary
			}
		}
		if c.Settings.LiftoffRepo == "" {
			c.Settings.LiftoffRepo = liftoffMasterRepoURL
		}
		return nil
	})
}

// relativeDir returns the trailing path segment of full relative to base,
// or "" when full isn't inside base.
func relativeDir(base, full string) string {
	if base == "" || full == "" {
		return ""
	}
	if len(full) > len(base) && full[:len(base)] == base {
		return full[len(base)+1:]
	}
	return ""
}

// offerBulkAdopt returns the number of worktrees actually adopted (0 on
// skip or empty candidate list) so the caller can tailor the next-step
// message.
func offerBulkAdopt(layout liftoff.Layout) (int, error) {
	c, err := liftoff.LoadConfig()
	if err != nil {
		return 0, err
	}
	cands, err := layout.FindAdoptCandidates(c)
	if err != nil {
		return 0, err
	}
	if len(cands) == 0 {
		return 0, nil
	}
	fmt.Println()
	fmt.Println(tui.StyleTitle.Render(fmt.Sprintf("found %d existing workspace(s)", len(cands))))
	for _, c := range cands {
		fmt.Printf("  %s  %s\n", c.Name, tui.StyleDim.Render("("+c.Branch+")"))
	}
	fmt.Println()
	fmt.Println(tui.StyleDim.Render("Kit will remember each workspace and reserve local ports for it. Stop running apps first for the best port picks."))

	accept, err := tui.RunConfirm(tui.ConfirmConfig{
		Title:       "Set up all existing workspaces?",
		Affirmative: "Set up all",
		Negative:    "Skip",
		Default:     true,
	})
	if err != nil {
		return 0, err
	}
	if !accept {
		return 0, nil
	}
	opts := liftoff.AdoptOptions{
		SymlinkNodeModules: false, // bulk adoption: don't mass-rewrite frontend trees
		WriteGtab:          true,
		GraphiteTrack:      false,
	}
	count := 0
	for _, cand := range cands {
		res, err := layout.Adopt(cand.Name, cand.Branch, cand.Path, opts, nil)
		if err != nil {
			fmt.Println(tui.StyleErr.Render("  ✗ " + cand.Name + ": " + err.Error()))
			continue
		}
		fmt.Println(tui.StyleOK.Render(fmt.Sprintf("  ✓ %s — slot %d", res.Name, res.Slot)))
		count++
	}
	return count, nil
}

func printDryRunPlan(layout liftoff.Layout, results []liftoff.CheckResult) {
	fmt.Println(tui.StyleTitle.Render("planned actions"))
	any := false
	if _, err := exec.LookPath("lumen"); err == nil {
		any = true
		fmt.Printf("  %s\n", tui.StyleHi.Render("• lumen"))
		fmt.Printf("      brew uninstall lumen (kit now diffs via hunk)\n")
	}
	for _, r := range results {
		if r.Status == liftoff.CheckOK || r.Status == liftoff.CheckSkip {
			continue
		}
		any = true
		fmt.Printf("  %s\n", tui.StyleHi.Render("• "+r.Name))
		switch r.ID {
		case "brew":
			fmt.Printf("      print Homebrew install command, exit (no auto-run)\n")
		case "brew-path":
			st := liftoff.DetectBrew()
			fmt.Printf("      prompt to append %q to %s\n", liftoff.BrewShellenvLine(st.BinaryAt), liftoff.ShellProfilePath())
		case "kit-path":
			fmt.Printf("      prompt to append %q to %s\n", liftoff.PathExportLine(liftoff.KitBinDir()), liftoff.ShellProfilePath())
		case "gh":
			if len(r.FixCmd) > 0 {
				fmt.Printf("      brew install %s\n", strings.Join(r.FixCmd, " "))
			} else {
				fmt.Printf("      run `gh auth login` interactively\n")
			}
		case "liftoff-master":
			if _, err := os.Stat(layout.Master); err != nil {
				fmt.Printf("      git clone %s %s, then yarn install in frontend/app + frontend/admin\n", liftoffMasterRepoURL, layout.Master)
			} else {
				fmt.Printf("      yarn install in master's frontend/app + frontend/admin\n")
			}
		default:
			if len(r.FixCmd) > 0 {
				casFlag := ""
				if r.FixCask {
					casFlag = "--cask "
				}
				fmt.Printf("      brew install %s%s\n", casFlag, strings.Join(r.FixCmd, " "))
			} else {
				fmt.Printf("      manual: %s\n", r.FixHint)
			}
		}
	}
	if !any {
		fmt.Println(tui.StyleOK.Render("  nothing to do — all checks pass."))
	}
	fmt.Println()
	fmt.Println(tui.StyleDim.Render("rerun without --dry-run to apply."))
}

// applyFix runs the remediation for one failed check. It returns
// pendingRestart=true when the fix only takes effect in a new shell (PATH
// edits), so the caller can skip the in-process re-check for that check —
// the running process can't see a freshly-appended PATH line.
func applyFix(layout liftoff.Layout, r liftoff.CheckResult) (pendingRestart bool, err error) {
	switch r.ID {
	case "brew":
		return false, fixBrewMissing()
	case "brew-path":
		return fixBrewPath()
	case "kit-path":
		return fixKitPath()
	case "gh":
		return false, fixGh(r)
	case "liftoff-master":
		return false, fixLiftoffMaster(layout, r)
	default:
		return false, fixBrewInstall(r)
	}
}

func fixBrewMissing() error {
	fmt.Println(tui.StyleWarn.Render("Homebrew is missing — kit setup can't install other tools without it."))
	fmt.Println()
	fmt.Println("Run this in your terminal to install Homebrew, then re-run `kit setup`:")
	fmt.Println()
	fmt.Println("  " + brewInstallScript)
	fmt.Println()
	return nil
}

func fixBrewPath() (bool, error) {
	st := liftoff.DetectBrew()
	if st.BinaryAt == "" {
		return false, nil
	}
	return fixPathEntry("Homebrew", st.BinaryAt, liftoff.BrewShellenvLine(st.BinaryAt), brewFenceComment)
}

// fixKitPath adds kit's own bin directory to PATH in the login shell's rc
// file — the "command not found: kit" fix after go install / make install.
func fixKitPath() (bool, error) {
	dir := liftoff.KitBinDir()
	if liftoff.DirOnPath(dir) {
		return false, nil
	}
	return fixPathEntry("kit", dir, liftoff.PathExportLine(dir), kitPathFenceComment)
}

// fixPathEntry prompts to append `line` (guarded by `fence` for idempotence)
// to the login shell's rc file, explaining that `what` lives at `loc` but
// isn't on PATH. Shared by the brew-path and kit-path fixes. Returns true when
// the line was written (or already present) — the change only takes effect in
// a new shell, so the caller treats it as pending-restart, not resolved.
func fixPathEntry(what, loc, line, fence string) (bool, error) {
	rc := liftoff.ShellProfilePath()
	fmt.Println(what + " is installed at " + loc + " but isn't on your PATH.")
	fmt.Println("Adding this line to " + rc + " fixes it:")
	fmt.Println()
	fmt.Println("  " + line)
	fmt.Println()

	accept, err := tui.RunConfirm(tui.ConfirmConfig{
		Title:    "Append line to " + filepath.Base(rc) + "?",
		Negative: "Skip",
		Default:  true,
	})
	if err != nil {
		return false, err
	}
	if !accept {
		return false, nil
	}
	if err := appendToShellProfile(fence, line); err != nil {
		return false, err
	}
	fmt.Println(tui.StyleOK.Render("✓ appended. Restart your terminal or run `source " + rc + "`."))
	return true, nil
}

// appendToShellProfile appends a fenced block (fence comment + lines) to the
// login shell's rc file, detected via $SHELL so bash users get ~/.bash_profile
// and zsh users get ~/.zshrc. Idempotent: a present fence comment is a no-op.
func appendToShellProfile(fence string, lines ...string) error {
	path := liftoff.ShellProfilePath()
	existing, _ := os.ReadFile(path)
	if strings.Contains(string(existing), fence) {
		return nil
	}
	block := "\n" + fence + "\n" + strings.Join(lines, "\n") + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(block)
	return err
}

func fixGh(r liftoff.CheckResult) error {
	// Two sub-cases: gh missing (FixCmd set) or gh installed-but-not-authed.
	if len(r.FixCmd) > 0 {
		return fixBrewInstall(r)
	}
	accept, err := tui.RunConfirm(tui.ConfirmConfig{
		Title:       "Run `gh auth login` now?",
		Description: "You'll be guided through a browser-based login flow.",
		Negative:    "Skip",
		Default:     true,
	})
	if err != nil {
		return err
	}
	if !accept {
		return nil
	}
	c := exec.Command("gh", "auth", "login")
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func fixLiftoffMaster(layout liftoff.Layout, r liftoff.CheckResult) error {
	// Three paths:
	// (a) master not on disk → prompt URL + path → clone + yarn install
	// (b) master exists but no node_modules → just yarn install
	// (c) master path exists but isn't a git repo → printed FixHint, no auto-fix.
	if _, err := os.Stat(layout.Master); err != nil {
		return cloneLiftoffMaster(layout)
	}
	if _, err := os.Stat(filepath.Join(layout.Master, ".git")); err != nil {
		fmt.Println(tui.StyleWarn.Render(r.Detail))
		fmt.Println(tui.StyleDim.Render("manual fix needed — remove " + layout.Master + " and re-run `kit setup`."))
		return nil
	}
	return masterYarnInstall(layout)
}

func cloneLiftoffMaster(layout liftoff.Layout) error {
	url := liftoffMasterRepoURL
	path := layout.Master
	formURL := huh.NewInput().
		Title("Liftoff master repo URL").
		Value(&url)
	formPath := huh.NewInput().
		Title("Clone to").
		Value(&path)
	if err := formURL.Run(); err != nil {
		return err
	}
	if err := formPath.Run(); err != nil {
		return err
	}

	accept, err := tui.RunConfirm(tui.ConfirmConfig{
		Title:    fmt.Sprintf("Run `git clone %s %s` and then `yarn install`?", url, path),
		Negative: "Skip",
		Default:  true,
	})
	if err != nil {
		return err
	}
	if !accept {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	fmt.Println(tui.StyleDim.Render("cloning master…"))
	if err := liftoff.RunStream("", "git", []string{"clone", url, path}, streamLine); err != nil {
		return err
	}
	// Update layout in-place for the yarn step.
	layout.Master = path
	return masterYarnInstall(layout)
}

func masterYarnInstall(layout liftoff.Layout) error {
	frontends := []string{
		filepath.Join(layout.Master, "frontend", "app"),
		filepath.Join(layout.Master, "frontend", "admin"),
	}
	for _, dir := range frontends {
		if _, err := os.Stat(dir); err != nil {
			continue // not all repos have both
		}
		fmt.Println(tui.StyleDim.Render("yarn install in " + dir + " …"))
		if err := liftoff.RunStream(dir, "yarn", []string{"install"}, streamLine); err != nil {
			return err
		}
	}
	return nil
}

// offerLumenRemoval uninstalls the old `lumen` diff viewer if it's still
// present. kit's `diff` now drives `hunk`; leaving lumen around is harmless
// but confusing, so setup offers to clean it up. Skipped silently when lumen
// isn't installed or brew isn't reachable.
func offerLumenRemoval() error {
	if _, err := exec.LookPath("lumen"); err != nil {
		return nil // not installed — nothing to migrate
	}
	st := liftoff.DetectBrew()
	if !st.OnPath {
		return nil // can't uninstall without brew on PATH
	}
	accept, err := tui.RunConfirm(tui.ConfirmConfig{
		Title:    "kit now uses `hunk` for diffs — run `brew uninstall lumen`?",
		Negative: "Keep",
		Default:  true,
	})
	if err != nil {
		return err
	}
	if !accept {
		return nil
	}
	fmt.Println(tui.StyleDim.Render("brew uninstall lumen …"))
	return liftoff.BrewUninstall("lumen", streamLine)
}

func fixBrewInstall(r liftoff.CheckResult) error {
	if len(r.FixCmd) == 0 {
		// Not auto-fixable (eg python venv); leave for user.
		return nil
	}
	st := liftoff.DetectBrew()
	if !st.OnPath {
		fmt.Println(tui.StyleWarn.Render("brew not on PATH — skipping " + strings.Join(r.FixCmd, " ")))
		return nil
	}
	pkgList := strings.Join(r.FixCmd, " ")
	caskFlag := ""
	if r.FixCask {
		caskFlag = "--cask "
	}
	accept, err := tui.RunConfirm(tui.ConfirmConfig{
		Title:    fmt.Sprintf("Run `brew install %s%s`?", caskFlag, pkgList),
		Negative: "Skip",
		Default:  true,
	})
	if err != nil {
		return err
	}
	if !accept {
		return nil
	}
	for _, pkg := range r.FixCmd {
		fmt.Println(tui.StyleDim.Render("brew install " + caskFlag + pkg + " …"))
		if err := liftoff.BrewInstall(pkg, r.FixCask, streamLine); err != nil {
			return err
		}
	}
	return nil
}

func streamLine(s string) { fmt.Println(tui.StyleDim.Render("  " + s)) }
