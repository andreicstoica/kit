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
const zshrcFenceComment = "# kit-setup: brew shellenv"

var setupDryRun bool

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install missing tools and bootstrap your kit environment",
	Long: "**setup** runs the same checks as `kit doctor`, then offers to fix each " +
		"failure: installs missing tools via Homebrew, runs `gh auth login`, clones " +
		"the Liftoff master repo, and runs `yarn install` so worktree node_modules " +
		"symlinks work.\n\n" +
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

	for _, r := range results {
		if r.Status == liftoff.CheckOK || r.Status == liftoff.CheckSkip {
			continue
		}
		if err := applyFix(layout, r); err != nil {
			fmt.Println(tui.StyleErr.Render("error: " + err.Error()))
		}
	}

	fmt.Println()
	fmt.Println(tui.StyleTitle.Render("re-checking…"))
	results = liftoff.RunChecks(liftoff.DefaultChecks(layout))
	fmt.Print(tui.RenderDoctor(results))

	if liftoff.AnyFailed(results) {
		fmt.Println(tui.StyleWarn.Render("still has failures — see hints above or re-run `kit setup`."))
		os.Exit(1)
	}

	// Persist what setup learned to config.toml.
	if err := persistSetupSettings(layout); err != nil {
		fmt.Println(tui.StyleDim.Render("(could not save settings to config.toml: " + err.Error() + ")"))
	}

	// Offer bulk-adopt for unmanaged worktrees.
	if err := offerBulkAdopt(layout); err != nil {
		fmt.Println(tui.StyleDim.Render("(adoption skipped: " + err.Error() + ")"))
	}

	fmt.Println(tui.StyleOK.Render("✓ ready to go — try `kit design my-first-kit`"))
	return nil
}

// persistSetupSettings writes Root + MasterDir + first installed editor
// into config.Settings. Merges with existing settings — user-edited
// fields aren't clobbered (only empty fields are filled in).
func persistSetupSettings(layout liftoff.Layout) error {
	c, err := liftoff.LoadConfig()
	if err != nil {
		return err
	}
	changed := false
	if c.Settings.Root == "" {
		c.Settings.Root = layout.Root
		changed = true
	}
	if c.Settings.MasterDir == "" {
		// Derive MasterDir from layout.Master minus layout.Root prefix.
		if rel := relativeDir(layout.Root, layout.Master); rel != "" {
			c.Settings.MasterDir = rel
			changed = true
		}
	}
	if c.Settings.Editor == "" {
		if eds := installedEditors(); len(eds) > 0 {
			c.Settings.Editor = eds[0].Binary
			changed = true
		}
	}
	if c.Settings.LiftoffRepo == "" {
		c.Settings.LiftoffRepo = liftoffMasterRepoURL
		changed = true
	}
	if !changed {
		return nil
	}
	return c.Save()
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

func offerBulkAdopt(layout liftoff.Layout) error {
	c, err := liftoff.LoadConfig()
	if err != nil {
		return err
	}
	cands, err := layout.FindAdoptCandidates(c)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		return nil
	}
	fmt.Println()
	fmt.Println(tui.StyleTitle.Render(fmt.Sprintf("found %d unmanaged worktree(s)", len(cands))))
	for _, c := range cands {
		fmt.Printf("  %s  %s\n", c.Name, tui.StyleDim.Render("("+c.Branch+")"))
	}
	fmt.Println()
	fmt.Println(tui.StyleDim.Render("kit will allocate a port slot and write metadata for each. stop running dev servers first for accurate slot picks."))

	accept := true
	if err := huh.NewConfirm().
		Title("Adopt all? (allocates port slots + writes metadata)").
		Affirmative("Yes").
		Negative("Skip").
		Value(&accept).Run(); err != nil {
		return err
	}
	if !accept {
		return nil
	}
	opts := liftoff.AdoptOptions{
		SymlinkNodeModules: false, // bulk adoption: don't mass-rewrite frontend trees
		WriteGtab:          true,
		GraphiteTrack:      false,
	}
	for _, cand := range cands {
		res, err := layout.Adopt(cand.Name, cand.Branch, cand.Path, opts, nil)
		if err != nil {
			fmt.Println(tui.StyleErr.Render("  ✗ " + cand.Name + ": " + err.Error()))
			continue
		}
		fmt.Println(tui.StyleOK.Render(fmt.Sprintf("  ✓ %s — slot %d", res.Name, res.Slot)))
	}
	return nil
}

func printDryRunPlan(layout liftoff.Layout, results []liftoff.CheckResult) {
	fmt.Println(tui.StyleTitle.Render("planned actions"))
	any := false
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
			fmt.Printf("      prompt to append %q to ~/.zshrc\n", liftoff.BrewShellenvLine(st.BinaryAt))
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

func applyFix(layout liftoff.Layout, r liftoff.CheckResult) error {
	switch r.ID {
	case "brew":
		return fixBrewMissing()
	case "brew-path":
		return fixBrewPath()
	case "gh":
		return fixGh(r)
	case "liftoff-master":
		return fixLiftoffMaster(layout, r)
	default:
		return fixBrewInstall(r)
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

func fixBrewPath() error {
	st := liftoff.DetectBrew()
	if st.BinaryAt == "" {
		return nil
	}
	line := liftoff.BrewShellenvLine(st.BinaryAt)
	fmt.Println("Homebrew is installed at " + st.BinaryAt + " but `brew` isn't on your PATH.")
	fmt.Println("Adding this line to ~/.zshrc fixes it:")
	fmt.Println()
	fmt.Println("  " + line)
	fmt.Println()

	accept := true
	form := huh.NewConfirm().
		Title("Append shellenv line to ~/.zshrc?").
		Affirmative("Yes").
		Negative("Skip").
		Value(&accept)
	if err := form.Run(); err != nil {
		return err
	}
	if !accept {
		return nil
	}
	if err := appendToZshrc(line); err != nil {
		return err
	}
	fmt.Println(tui.StyleOK.Render("✓ appended. Restart your terminal or run `source ~/.zshrc`."))
	return nil
}

func appendToZshrc(shellenv string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".zshrc")
	existing, _ := os.ReadFile(path)
	if strings.Contains(string(existing), zshrcFenceComment) {
		return nil
	}
	block := "\n" + zshrcFenceComment + "\n" + shellenv + "\n"
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
	accept := true
	form := huh.NewConfirm().
		Title("Run `gh auth login` now?").
		Description("You'll be guided through a browser-based login flow.").
		Affirmative("Yes").
		Negative("Skip").
		Value(&accept)
	if err := form.Run(); err != nil {
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

	accept := true
	confirm := huh.NewConfirm().
		Title(fmt.Sprintf("Run `git clone %s %s` and then `yarn install`?", url, path)).
		Affirmative("Yes").
		Negative("Skip").
		Value(&accept)
	if err := confirm.Run(); err != nil {
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
	accept := true
	form := huh.NewConfirm().
		Title(fmt.Sprintf("Run `brew install %s%s`?", caskFlag, pkgList)).
		Affirmative("Yes").
		Negative("Skip").
		Value(&accept)
	if err := form.Run(); err != nil {
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
