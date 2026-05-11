package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var diffPlain bool

var diffCmd = &cobra.Command{
	Use:   "diff [name]",
	Short: "Show the worktree's diff vs master",
	Long: "**diff** runs an interactive diff of the worktree against master.\n\n" +
		"Uses [lumen](https://github.com/jnsahaj/lumen) when installed " +
		"(side-by-side viewer + syntax highlight); falls back to plain " +
		"`git diff` otherwise. Pass `--plain` to force git's default.\n\n" +
		"With no arg, resolves the worktree from cwd or opens a picker.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit diff — pick a kit")
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}
		if name == "master" {
			return fmt.Errorf("nothing to diff — master is the baseline")
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}

		mainBranch := layout.MainBranch
		if mainBranch == "" {
			mainBranch = "master"
		}
		ref := mainBranch + "..HEAD"

		var c *exec.Cmd
		if !diffPlain {
			if _, err := exec.LookPath("lumen"); err == nil {
				c = exec.Command("lumen", "diff", ref)
			}
		}
		if c == nil {
			c = exec.Command("git", "diff", mainBranch+"...HEAD")
			if !diffPlain {
				fmt.Println(tui.StyleDim.Render("(install `lumen` for an interactive side-by-side viewer)"))
			}
		}
		c.Dir = path
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	diffCmd.Flags().BoolVar(&diffPlain, "plain", false, "use plain git diff even if lumen is installed")
	rootCmd.AddCommand(diffCmd)
}
