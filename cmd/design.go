package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var designCmd = newDesignCmd()

func newDesignCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "design [name]",
		Aliases: []string{"new"},
		Short:   "Create a fresh workspace",
		Long: `design creates a Liftoff worktree with its own branch and port slot.

In a terminal, the default is the setup wizard. Agents and scripts can use:

  kit design login-fix --non-interactive --json

Headless setup defaults to no database, shared frontend packages, Graphite
when installed, and no backend dependency install. It never opens a terminal
or starts services. --db none reserves an isolated database target without
creating it; use kit db later if the task needs one. --db empty creates an
empty private database. --db copy tries a file-level template copy and refuses
a full pg_dump fallback unless --allow-pg-dump is explicitly set.

--dry-run prints the proposed setup without fetching or creating resources.
--json and explicit setup flags also skip the wizard.

--checkout-backend twig opts into released Twig >=0.4.0 copy-on-write
source checkouts. Git remains the default. A leading liftoff- is stripped
from the feature name.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			backend, _ := cmd.Flags().GetString("checkout-backend")
			if designHeadless(cmd) {
				return runDesignHeadless(liftoff.DefaultLayout(), cmd, args)
			}
			prefill := ""
			if len(args) == 1 {
				prefill = args[0]
			}
			return tui.RunDesignTUI(liftoff.DefaultLayout(), prefill, backend)
		},
	}
	cmd.Flags().String("checkout-backend", "git", "Source checkout backend: git or twig (opt-in copy-on-write)")
	cmd.Flags().Bool("non-interactive", false, "Create without prompts, opening windows, or starting services")
	cmd.Flags().Bool("json", false, "Print a single JSON result on stdout (progress goes to stderr)")
	cmd.Flags().Bool("dry-run", false, "Print the setup plan without changing anything")
	cmd.Flags().String("db", "none", "Headless database setup: none, empty, or copy")
	cmd.Flags().Bool("allow-pg-dump", false, "Allow --db copy to fall back to a full dump and restore")
	cmd.Flags().Bool("backend-deps", false, "Install backend dependencies into the configured Python environment")
	cmd.Flags().Bool("symlink-frontend", true, "Reuse master's frontend node_modules")
	cmd.Flags().Bool("graphite", liftoff.HasGraphite(), "Track the new branch in Graphite")
	return cmd
}

func designHeadless(cmd *cobra.Command) bool {
	noninteractive, _ := cmd.Flags().GetBool("non-interactive")
	if noninteractive || !interactive() {
		return true
	}
	for _, flag := range []string{"json", "dry-run", "db", "allow-pg-dump", "backend-deps", "symlink-frontend", "graphite"} {
		if cmd.Flags().Changed(flag) {
			return true
		}
	}
	return false
}

type designResult struct {
	Name            string         `json:"name"`
	Path            string         `json:"path"`
	Branch          string         `json:"branch"`
	DatabaseMode    string         `json:"database_mode"`
	BackendDeps     bool           `json:"backend_deps"`
	SymlinkFrontend bool           `json:"symlink_frontend"`
	Graphite        bool           `json:"graphite"`
	CheckoutBackend string         `json:"checkout_backend"`
	AllowPGDump     bool           `json:"allow_pg_dump"`
	DryRun          bool           `json:"dry_run"`
	Slot            int            `json:"slot,omitempty"`
	Ports           *liftoff.Ports `json:"ports,omitempty"`
}

func runDesignHeadless(layout liftoff.Layout, cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("headless design requires a feature name: kit design <name> --non-interactive")
	}
	name, err := liftoff.NormalizeAndValidate(args[0])
	if err != nil {
		return err
	}
	if len(liftoff.DBName(name)) > 63 {
		return errors.New("feature name is too long for an isolated PostgreSQL database (max 55 characters)")
	}
	mode, _ := cmd.Flags().GetString("db")
	if mode == "" {
		return errors.New("--db must be none, empty, or copy")
	}
	if err := liftoff.ValidateDatabaseMode(mode); err != nil {
		return err
	}
	allowDump, _ := cmd.Flags().GetBool("allow-pg-dump")
	if allowDump && mode != "copy" {
		return errors.New("--allow-pg-dump requires --db copy")
	}
	backend, _ := cmd.Flags().GetString("checkout-backend")
	if backend != "git" && backend != "twig" {
		return fmt.Errorf("unknown checkout backend %q; use git or twig", backend)
	}
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	return executeHeadlessDesign(layout, cmd, name, mode, backend, allowDump, dryRun)
}

func executeHeadlessDesign(layout liftoff.Layout, cmd *cobra.Command, name, mode, backend string, allowDump, dryRun bool) error {
	if !layout.MasterIsRepo() {
		return fmt.Errorf("master repo not found at %s (set KIT_ROOT/KIT_MASTER_DIR)", layout.Master)
	}
	cfg, err := liftoff.LoadConfig()
	if err != nil {
		return err
	}
	if _, exists := cfg.Worktrees[name]; exists {
		return fmt.Errorf("kit record already exists for %s; inspect kit lineup or kit reconcile before retrying", name)
	}
	for _, path := range []string{layout.WorktreePath(name), layout.LegacyWorktreePath(name)} {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("worktree path already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	backendDeps, _ := cmd.Flags().GetBool("backend-deps")
	symlink, _ := cmd.Flags().GetBool("symlink-frontend")
	graphite, _ := cmd.Flags().GetBool("graphite")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	path, err := filepath.Abs(layout.WorktreePath(name))
	if err != nil {
		return err
	}
	result := designResult{
		Name: name, Path: path, Branch: name, DatabaseMode: mode,
		BackendDeps: backendDeps, SymlinkFrontend: symlink, Graphite: graphite,
		CheckoutBackend: backend, AllowPGDump: allowDump, DryRun: dryRun,
	}
	if !dryRun {
		plan := liftoff.DressPlan{
			Name: name, Worktree: path, DatabaseMode: mode, AllowPGDump: allowDump,
			BackendDeps: backendDeps, SymlinkFrontend: symlink, GraphiteTrack: graphite,
			CheckoutBackend: backend,
		}
		var failures []error
		for update := range layout.RunDress(plan) {
			if update.Line != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), update.Line)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", update.Title, update.Status)
			}
			if update.Status == liftoff.StepFailed && update.Err != nil {
				failures = append(failures, update.Err)
			}
			if update.AllocatedSlot > 0 {
				result.Slot = update.AllocatedSlot
			}
		}
		if err := errors.Join(failures...); err != nil {
			return fmt.Errorf("kit design failed: %w", err)
		}
		if result.Slot == 0 {
			return errors.New("kit design finished without a port slot; inspect kit lineup before retrying")
		}
		ports := liftoff.PortsForSlot(result.Slot)
		result.Ports = &ports
	}
	if jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
	}
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "plan: %s at %s (db=%s, backend-deps=%t, frontend-reuse=%t, graphite=%t, checkout=%s)\n",
			name, path, mode, backendDeps, symlink, graphite, backend)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "ready: %s at %s (slot %d, db=%s)\n", name, path, result.Slot, mode)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(designCmd)
}
