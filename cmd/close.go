package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var closeBackend string

var closeCmd = &cobra.Command{
	Use:               "close [name]",
	Short:             "Explicitly delete a worktree's terminal workspace",
	Long:              "close removes all Kit-mapped Rex and Herdr terminal workspaces, including panes and agents, but leaves the Git worktree and Kit services intact. Use --backend to close only one runtime. The next open recreates the workspace.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit close — pick a worktree")
		if err != nil || name == "" {
			return err
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}
		if closeBackend == "" {
			if err := liftoff.CloseManagedWorkspaces(name, path); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "closed managed terminal workspaces for %s\n", name)
			return nil
		}
		backend, err := liftoff.ParseTerminalBackend(closeBackend)
		if err != nil {
			return err
		}
		if backend == liftoff.BackendRex {
			err = liftoff.CloseRex(name, path)
		} else {
			err = liftoff.CloseHerdr(name, path)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "closed %s workspace for %s\n", backend, name)
		return nil
	},
}

func init() {
	closeCmd.Flags().StringVar(&closeBackend, "backend", "", "close only rex or herdr (default: all mapped runtimes)")
	rootCmd.AddCommand(closeCmd)
}
