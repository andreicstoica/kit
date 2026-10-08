package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var rexMigrateApply bool
var rexMigrateDryRun bool
var rexMigrateWorkspace string

var readHerdrForRexImport = liftoff.ReadHerdrState
var readRexForImport = liftoff.ReadRexState
var applyRexImport = liftoff.ApplyRexImport

var migrateRexCmd = &cobra.Command{
	Use: "rex", Short: "Plan or apply a Herdr-to-Rex workspace import",
	Long: "`kit migrate rex` inventories Herdr workspaces without changing either runtime. Use --apply to explicitly recreate workspace structure in Rex. Live processes and agents are not transferred.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if rexMigrateApply && rexMigrateDryRun {
			return fmt.Errorf("--apply and --dry-run cannot be used together")
		}
		apply := rexMigrateApply && !rexMigrateDryRun
		herdr, err := readHerdrForRexImport()
		if err != nil {
			return fmt.Errorf("read Herdr snapshot: %w", err)
		}
		var rex liftoff.RexState
		rexUnavailable := !liftoff.RexAvailable()
		if rexUnavailable && apply {
			return fmt.Errorf("cannot apply: Rex CLI is unavailable")
		}
		if !rexUnavailable {
			rex, err = readRexForImport()
			if err != nil {
				return fmt.Errorf("read Rex state: %w", err)
			}
		}
		mapping, err := liftoff.LoadRexImportMap()
		if err != nil {
			return err
		}
		existing := make(map[string]bool, len(rex.Sessions))
		for _, s := range rex.Sessions {
			existing[s.SessionID] = true
		}
		if rexMigrateWorkspace != "" {
			filtered := liftoff.HerdrState{}
			for _, ws := range herdr.Workspaces {
				if ws.WorkspaceID == rexMigrateWorkspace || ws.Label == rexMigrateWorkspace {
					filtered.Workspaces = append(filtered.Workspaces, ws)
				}
			}
			if len(filtered.Workspaces) == 0 {
				return fmt.Errorf("Herdr workspace %q not found", rexMigrateWorkspace)
			}
			wanted := map[string]bool{}
			for _, ws := range filtered.Workspaces {
				wanted[ws.WorkspaceID] = true
			}
			selectedTabs := map[string]bool{}
			for _, tab := range herdr.Tabs {
				if wanted[tab.WorkspaceID] {
					filtered.Tabs = append(filtered.Tabs, tab)
					selectedTabs[tab.TabID] = true
				}
			}
			for _, pane := range herdr.Panes {
				if wanted[pane.WorkspaceID] || selectedTabs[pane.TabID] {
					filtered.Panes = append(filtered.Panes, pane)
				}
			}
			herdr = filtered
		}
		plans := liftoff.PlanRexImport(herdr, mapping, existing)
		liftoff.AnnotateRexImportCollisions(plans, rex, mapping)
		printRexImportPlan(cmd.OutOrStdout(), plans, apply)
		if rexUnavailable {
			fmt.Fprintln(cmd.OutOrStdout(), "warning: Rex unavailable; destination collisions cannot be checked in this dry run")
		}
		if !apply {
			return nil
		}
		for _, item := range plans {
			if item.Blocked {
				return fmt.Errorf("cannot apply: %s (%s)", item.Skip, item.Label)
			}
			if item.CWD == "" || !isImportDirectory(item.CWD) {
				return fmt.Errorf("cannot apply workspace %q: checkout directory is unavailable", item.Label)
			}
		}
		if err := applyRexImport(herdr, &mapping); err != nil {
			return err
		}
		return nil
	},
}

var migrateCmd = &cobra.Command{Use: "migrate", Short: "Migrate workspace runtime state", Args: cobra.NoArgs}

func init() {
	migrateRexCmd.Flags().BoolVar(&rexMigrateApply, "apply", false, "explicitly create Rex workspaces and shells (default is dry-run)")
	migrateRexCmd.Flags().BoolVar(&rexMigrateDryRun, "dry-run", false, "show the plan without creating Rex sessions (default)")
	migrateRexCmd.Flags().StringVar(&rexMigrateWorkspace, "workspace", "", "only migrate one Herdr workspace label or ID")
	migrateCmd.AddCommand(migrateRexCmd)
	rootCmd.AddCommand(migrateCmd)
}

func printRexImportPlan(w io.Writer, items []liftoff.RexImportItem, apply bool) {
	mode := "DRY RUN — no changes"
	if apply {
		mode = "APPLY — shells only; no agents or services will be launched"
	}
	fmt.Fprintf(w, "Herdr → Rex migration (%s)\n", mode)
	fmt.Fprintln(w, "Live agents/services are not transferred or started; resume manually only after avoiding duplicates in Herdr.")
	fmt.Fprintln(w, "Herdr does not expose split geometry in this snapshot; multi-pane splits are reconstructed horizontally.")
	for _, item := range items {
		fmt.Fprintf(w, "- %s [%s] cwd=%s", item.Label, item.SourceID, item.CWD)
		if item.Skip != "" {
			fmt.Fprintf(w, " — %s", item.Skip)
		}
		fmt.Fprintln(w)
		for _, tab := range item.Tabs {
			fmt.Fprintf(w, "  tab %q (%d panes)\n", tab.Label, len(tab.Panes))
			for _, pane := range tab.Panes {
				note := "shell; manual resume if needed"
				if pane.Agent != "" {
					note = "agent not started; manual resume only"
				}
				fmt.Fprintf(w, "    pane %s cwd=%s — %s\n", pane.SourceID, pane.CWD, note)
			}
		}
		for _, warning := range item.Warnings {
			fmt.Fprintf(w, "  warning: %s\n", warning)
		}
	}
	if len(items) == 0 {
		fmt.Fprintln(w, "No Herdr workspaces found.")
	}
}

func isImportDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
