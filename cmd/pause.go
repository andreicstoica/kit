package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var (
	pauseOnly []string
	pauseAll  bool
)

var pauseCmd = &cobra.Command{
	Use:     "pause [name]",
	Aliases: []string{"stop"},
	Short:   "Halt the kit's services",
	Long: `pause stops services for a worktree.

  kit pause            picker → confirm → kill
  kit pause <name>     skip picker
  kit pause <name> --only celery   stop only specific services
  kit pause --all      stop every running service across every worktree`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		if pauseAll {
			return tui.PauseAll(layout)
		}
		name, err := resolveArgOrCwd(layout, args, true)
		if err != nil {
			return err
		}
		only, err := parseServiceList(pauseOnly)
		if err != nil {
			return err
		}
		return tui.RunPauseTUI(layout, tui.PauseConfig{Name: name, Only: only})
	},
}

func init() {
	pauseCmd.Flags().StringSliceVar(&pauseOnly, "only", nil,
		"comma-separated services to stop (app,admin,api,admin_be,mcp,celery,beat)")
	pauseCmd.Flags().BoolVar(&pauseAll, "all", false,
		"stop every running service across every worktree (no picker)")
	rootCmd.AddCommand(pauseCmd)
}
