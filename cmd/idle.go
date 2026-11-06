package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

func newIdleCommand() *cobra.Command {
	var after time.Duration
	var apply, jsonOutput bool
	command := &cobra.Command{
		Use: "idle", Short: "Inspect or auto-pause inactive worktree services",
		Long: "Without flags, show a dry run. The monitor requires two continuously observed hours without code changes, Kit use, active agents, jobs, or client connections. Unknown activity blocks pausing. Shared infrastructure and master are protected.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := liftoff.DefaultLayout().IdleSweep(after, apply)
			if jsonOutput {
				return errors.Join(err, json.NewEncoder(cmd.OutOrStdout()).Encode(results))
			}
			for _, result := range results {
				if apply && result.Blocked != "paused" && !result.Eligible {
					continue
				}
				reason := result.Blocked
				if reason == "" {
					if result.Eligible {
						reason = "eligible for pause"
					} else if !result.Since.IsZero() {
						reason = fmt.Sprintf("observed idle for %s; threshold %s", time.Since(result.Since).Truncate(time.Second), after)
					} else {
						reason = "awaiting observation"
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", result.Name, reason)
			}
			return err
		},
	}
	command.PersistentFlags().DurationVar(&after, "after", 2*time.Hour, "continuous inactivity required before pause")
	command.Flags().BoolVar(&apply, "apply", false, "record observations and gracefully stop eligible services")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print observation details as JSON")
	command.AddCommand(&cobra.Command{
		Use: "enable", Short: "Enable the macOS idle monitor, checked every five minutes", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := liftoff.EnableIdleMonitor(after); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "idle auto-pause enabled after %s; checks every 5m; kit idle shows blockers\n", after)
			return nil
		},
	}, &cobra.Command{
		Use: "disable", Short: "Disable automatic pausing without stopping services", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := liftoff.DisableIdleMonitor(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "idle auto-pause disabled")
			return nil
		},
	})
	return command
}

func init() { rootCmd.AddCommand(newIdleCommand()) }
