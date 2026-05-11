package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

// logRetention is how long stale run dirs are kept before passive cleanup.
const logRetention = 30 * 24 * time.Hour

var (
	playOnly      []string
	playNoCelery  bool
)

var playCmd = &cobra.Command{
	Use:     "play [name]",
	Aliases: []string{"start"},
	Short:   "Spin up the kit's services (frontend/app, frontend/admin, backend, celery)",
	Long: `play starts the dev servers for a worktree:

  app, admin (Vite), api, admin_be (uvicorn --reload), celery worker, beat

Each worktree gets a 5-port band based on its slot:
  app:3000+slot*10  admin:3001+slot*10  api:9000+slot*10  admin_be:9001+slot*10

If no <name> is given, you'll get a Bubble Tea picker. Use --only to skip
the service-selection screen.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Passive log cleanup: drop run dirs whose newest file is >30 days old
		// and which don't own a live PID. Cheap, fire-and-forget.
		_, _ = liftoff.SweepOldRunDirs(logRetention)

		layout := liftoff.DefaultLayout()
		name, err := resolveArgOrCwd(layout, args)
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
		return tui.RunPlayTUI(layout, tui.PlayConfig{
			Name:     name,
			Only:     only,
			NoCelery: playNoCelery,
		})
	},
}

func init() {
	playCmd.Flags().StringSliceVar(&playOnly, "only", nil,
		"comma-separated services to start (app,admin,api,admin_be,mcp,celery,beat)")
	playCmd.Flags().BoolVar(&playNoCelery, "no-celery", false,
		"skip celery worker and beat")
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
			out = append(out, svc)
		}
	}
	return out, nil
}
