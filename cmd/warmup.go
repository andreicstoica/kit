package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var warmupCmd = &cobra.Command{
	Use:     "warmup <name>",
	Aliases: []string{"gtab"},
	Short:   "Pre-match warmup: launch the gtab ghostty workspace for a kit",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := liftoff.NormalizeAndValidate(args[0])
		if err != nil {
			return err
		}
		if !layout.HasGtab(name) {
			return fmt.Errorf("no gtab workspace at %s — re-run `kit design` or write one manually", layout.GtabFile(name))
		}
		return layout.LaunchGtab(name)
	},
}

func init() {
	rootCmd.AddCommand(warmupCmd)
}
