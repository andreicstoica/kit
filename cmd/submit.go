package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var (
	submitStack   bool
	submitDraft   bool
	submitNoEdit  bool
)

var submitCmd = &cobra.Command{
	Use:     "submit [name]",
	Short:   "Submit the worktree's branch to GitHub via `gt submit`",
	Long: "**submit** thin-wraps `gt submit` so you don't have to `cd` into the worktree to push.\n\n" +
		"With no arg, uses the worktree from cwd, then opens a picker. Pass\n" +
		"`--stack` to submit the whole stack, `--draft` for draft PRs,\n" +
		"`--no-edit` to skip the editor.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !liftoff.HasGraphite() {
			return fmt.Errorf("gt not installed — run `kit setup` or `brew install withgraphite/tap/graphite`")
		}
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit submit — pick a kit")
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}
		if name == "master" {
			return fmt.Errorf("nothing to submit — master is the trunk")
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}
		gtArgs := []string{"submit"}
		if submitStack {
			gtArgs = append(gtArgs, "--stack")
		}
		if submitDraft {
			gtArgs = append(gtArgs, "--draft")
		}
		if submitNoEdit {
			gtArgs = append(gtArgs, "--no-edit")
		}
		c := exec.Command("gt", gtArgs...)
		c.Dir = path
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	submitCmd.Flags().BoolVar(&submitStack, "stack", false, "submit the entire stack (gt submit --stack)")
	submitCmd.Flags().BoolVar(&submitDraft, "draft", false, "open the PR as a draft")
	submitCmd.Flags().BoolVar(&submitNoEdit, "no-edit", false, "skip the PR description editor")
	rootCmd.AddCommand(submitCmd)
}
