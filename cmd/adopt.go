package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var (
	adoptSkipSymlink bool
	adoptSkipGtab    bool
	adoptSkipGt      bool
	adoptYes         bool
)

var adoptCmd = &cobra.Command{
	Use:     "adopt [name]",
	Aliases: []string{"register"},
	Short:   "Register an existing worktree with kit (allocate a slot + write metadata)",
	Long: "**adopt** registers a worktree that exists on disk + in git but isn't " +
		"yet known to kit. Allocates the next available port slot and writes the " +
		"branch + path to `config.toml` so `kit play`, `kit links`, `kit wash` " +
		"recognize it.\n\n" +
		"Pass a name, or run from inside the worktree, or pick from a list of " +
		"adoptable candidates.",
	Args: cobra.MaximumNArgs(1),
	RunE: runAdopt,
}

func init() {
	adoptCmd.Flags().BoolVar(&adoptSkipSymlink, "no-symlink", false, "skip symlinking frontend node_modules from master")
	adoptCmd.Flags().BoolVar(&adoptSkipGtab, "no-gtab", false, "skip writing the Ghostty workspace AppleScript")
	adoptCmd.Flags().BoolVar(&adoptSkipGt, "no-graphite", false, "skip `gt track --parent master`")
	adoptCmd.Flags().BoolVarP(&adoptYes, "yes", "y", false, "accept all defaults without prompting")
	rootCmd.AddCommand(adoptCmd)
}

func runAdopt(cmd *cobra.Command, args []string) error {
	layout := liftoff.DefaultLayout()
	cfg, err := liftoff.LoadConfig()
	if err != nil {
		return err
	}

	name, err := resolveArgOrCwd(layout, args, true)
	if err != nil {
		return err
	}
	if name == "" {
		picked, err := pickAdoptCandidate(layout, cfg)
		if err != nil {
			return err
		}
		if picked == "" {
			return nil
		}
		name = picked
	}

	if _, ok := cfg.Worktrees[name]; ok {
		fmt.Println(tui.StyleWarn.Render(fmt.Sprintf("%s is already adopted (slot %d).", name, cfg.Worktrees[name].Slot)))
		return nil
	}

	wt, err := findWorktreeByName(layout, name)
	if err != nil {
		return err
	}

	fmt.Println(tui.StyleTitle.Render("kit adopt"))
	fmt.Printf("  name:   %s\n", name)
	fmt.Printf("  branch: %s\n", wt.Branch)
	fmt.Printf("  path:   %s\n", wt.Path)
	fmt.Println()

	if !adoptYes {
		accept := true
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("Adopt %s as a kit-managed worktree?", name)).
			Affirmative("Yes").
			Negative("Cancel").
			Value(&accept).Run(); err != nil {
			return err
		}
		if !accept {
			return nil
		}
	}

	opts := liftoff.AdoptOptions{
		SymlinkNodeModules: !adoptSkipSymlink,
		WriteGtab:          !adoptSkipGtab,
		GraphiteTrack:      !adoptSkipGt,
	}
	res, err := layout.Adopt(name, wt.Branch, wt.Path, opts, streamLine)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(tui.StyleOK.Render(fmt.Sprintf("✓ adopted %s — slot %d", res.Name, res.Slot)))
	ports := liftoff.PortsForSlot(res.Slot)
	fmt.Printf("  app:      http://localhost:%d\n", ports.App)
	fmt.Printf("  admin:    http://localhost:%d\n", ports.Admin)
	fmt.Printf("  api:      http://localhost:%d/api\n", ports.API)
	fmt.Printf("  admin_be: http://localhost:%d/api\n", ports.AdminBE)
	if res.Symlinked {
		fmt.Println(tui.StyleDim.Render("  symlinked frontend node_modules from master"))
	}
	if res.GtabPath != "" {
		fmt.Println(tui.StyleDim.Render("  wrote " + res.GtabPath))
	}
	if res.GraphiteTracked {
		fmt.Println(tui.StyleDim.Render("  tracked in graphite"))
	}
	fmt.Println()
	fmt.Println(tui.StyleDim.Render("next: `kit play " + name + "`"))
	return nil
}

// pickAdoptCandidate opens the picker scoped to worktrees not yet in
// config. Returns "" on user cancel.
func pickAdoptCandidate(layout liftoff.Layout, cfg *liftoff.Config) (string, error) {
	cands, err := layout.FindAdoptCandidates(cfg)
	if err != nil {
		return "", err
	}
	if len(cands) == 0 {
		fmt.Println(tui.StyleOK.Render("all worktrees are already adopted."))
		return "", nil
	}
	return tui.PickAdoptCandidate(cands)
}

// findWorktreeByName resolves a kit name to its git worktree record.
func findWorktreeByName(layout liftoff.Layout, name string) (*liftoff.Worktree, error) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return nil, err
	}
	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		if w.Name() == name {
			return &w, nil
		}
	}
	return nil, fmt.Errorf("no git worktree found for %q (check `git worktree list` in master)", name)
}
