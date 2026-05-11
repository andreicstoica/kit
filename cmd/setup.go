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

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install missing tools and bootstrap your kit environment",
	Long: "**setup** runs the same checks as `kit doctor`, then offers to fix each " +
		"failure: installs missing tools via Homebrew, runs `gh auth login`, clones " +
		"the Liftoff master repo, and runs `yarn install` so worktree node_modules " +
		"symlinks work.\n\n" +
		"You'll be asked before anything is changed. Setup is idempotent — re-run " +
		"any time.",
	RunE: runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	layout := liftoff.DefaultLayout()

	fmt.Println(tui.StyleTitle.Render("kit setup — check & install"))
	fmt.Println(tui.StyleDim.Render("nothing is changed without your confirmation."))
	fmt.Println()

	results := liftoff.RunChecks(liftoff.DefaultChecks(layout))
	fmt.Print(tui.RenderDoctor(results))
	fmt.Println()

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
	fmt.Println(tui.StyleOK.Render("✓ ready to go — try `kit design my-first-kit`"))
	return nil
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
