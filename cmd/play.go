package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

const (
	logRetention    = 30 * 24 * time.Hour
	testDBRetention = 24 * time.Hour
)

var (
	playOnly     []string
	playNoCelery bool
	playOpen     bool
)

var playCmd = &cobra.Command{
	Use:     "play [name]",
	Aliases: []string{"start"},
	Short:   "Start a workspace",
	Long: `play starts the app for a workspace:

  website, admin, API, admin API, background worker

Each workspace gets its own local ports so multiple features can run at once.

If no <name> is given, you'll get a Bubble Tea picker. Use --only to skip
the service-selection screen.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Passive cleanup must not prevent a workspace from starting.
		_, _ = liftoff.SweepOldRunDirs(logRetention)
		_, _ = liftoff.SweepOldTestDBs(testDBRetention)

		layout := liftoff.DefaultLayout()
		name, err := resolveArgOrCwdSkipMasterCwd(layout, args)
		if err != nil {
			return err
		}
		if name != "" && len(args) == 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "playing %s (from cwd)\n", name)
		}
		only, err := parseServiceList(playOnly)
		if err != nil {
			return err
		}
		err = tui.RunPlayTUI(layout, tui.PlayConfig{
			Name:     name,
			Only:     only,
			NoCelery: playNoCelery,
		})
		if err != nil || !playOpen || name == "" {
			return err
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}
		return tui.OpenHerdrWorktree(name, path, "", tui.HerdrConnectAttach)
	},
}

func init() {
	playCmd.Flags().StringSliceVar(&playOnly, "only", nil,
		"comma-separated services to start (app,admin,api,admin_be,mcp,celery,beat)")
	playCmd.Flags().BoolVar(&playNoCelery, "no-celery", false,
		"skip celery worker and beat")
	playCmd.Flags().BoolVar(&playOpen, "open", false,
		"attach to the worktree's Herdr space after services start")
	playCmd.Flags().BoolVar(&playOpen, "attach", false,
		"alias for --open")
	rootCmd.AddCommand(playCmd)
}

// parseServiceList resolves user input ("app,admin,api") to []Service.
func parseServiceList(raw []string) ([]liftoff.Service, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	known := map[string]liftoff.Service{
		"app":      liftoff.SvcApp,
		"admin":    liftoff.SvcAdmin,
		"api":      liftoff.SvcAPI,
		"admin_be": liftoff.SvcAdminBE,
		"adminbe":  liftoff.SvcAdminBE,
		"admin-be": liftoff.SvcAdminBE,
		"mcp":      liftoff.SvcMCP,
		"celery":   liftoff.SvcCelery,
		"beat":     liftoff.SvcBeat,
	}
	var out []liftoff.Service
	seen := map[liftoff.Service]bool{}
	for _, item := range raw {
		for _, part := range strings.Split(item, ",") {
			part = strings.TrimSpace(strings.ToLower(part))
			if part == "" {
				continue
			}
			svc, ok := known[part]
			if !ok {
				return nil, fmt.Errorf("unknown service %q (valid: app, admin, api, admin_be, mcp, celery, beat)", part)
			}
			if seen[svc] {
				continue
			}
			seen[svc] = true
			out = append(out, svc)
		}
	}
	return out, nil
}
