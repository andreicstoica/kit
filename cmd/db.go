package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

func newDBCommand() *cobra.Command {
	var mode string
	var allowPGDump bool
	command := &cobra.Command{
		Use:               "db <name>",
		Short:             "Provision a deferred workspace database",
		Long:              "Create an isolated empty database or copy liftoff by template. Copy never falls back to a full dump unless --allow-pg-dump is set. Empty databases require schema/migrations before use.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeWorktreeNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := liftoff.DefaultLayout().ProvisionDatabase(args[0], mode, allowPGDump, func(line string) {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "database provisioned for %s (%s)\n", args[0], mode)
			if mode == liftoff.DatabaseModeEmpty {
				fmt.Fprintln(cmd.OutOrStdout(), "schema and migrations are not initialized")
			}
			return nil
		},
	}
	command.Flags().StringVar(&mode, "mode", "", "empty or copy")
	command.Flags().BoolVar(&allowPGDump, "allow-pg-dump", false, "explicitly allow full dump/restore when template copy fails")
	_ = command.MarkFlagRequired("mode")
	return command
}

func init() {
	rootCmd.AddCommand(newDBCommand())
}
