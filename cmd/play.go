package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var (
	playOnly      []string
	playNoCelery  bool
)

var playCmd = &cobra.Command{
	Use:   "play [name]",
	Short: "Spin up the kit's services (frontend/app, frontend/admin, backend, celery)",
	Long: `play starts the dev servers for a worktree:

  app, admin (Vite), api, admin_be (uvicorn --reload), celery worker, beat

Each worktree gets a 5-port band based on its slot:
  app:3000+slot*10  admin:3001+slot*10  api:9000+slot*10  admin_be:9001+slot*10

If no <name> is given, you'll get a Bubble Tea picker. Use --only to skip
the service-selection screen.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name := ""
		if len(args) == 1 {
			n, err := liftoff.NormalizeAndValidate(args[0])
			if err != nil {
				return err
			}
			name = n
		} else {
			// No name given: try to detect from cwd.
			if n := worktreeNameFromCwd(layout); n != "" {
				name = n
				fmt.Fprintf(cmd.ErrOrStderr(), "playing %s (from cwd)\n", name)
			}
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

// worktreeNameFromCwd returns the worktree name if the current directory is
// inside one of the worktrees registered for the layout, else "".
func worktreeNameFromCwd(layout liftoff.Layout) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	cwd, _ = filepath.Abs(cwd)
	wts, err := layout.ListWorktrees()
	if err != nil {
		return ""
	}
	// Pick the longest matching worktree path (handles nested dirs correctly).
	best := ""
	bestLen := 0
	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		wp, _ := filepath.Abs(w.Path)
		if cwd == wp || strings.HasPrefix(cwd, wp+string(filepath.Separator)) {
			if len(wp) > bestLen {
				best = w.Name()
				bestLen = len(wp)
			}
		}
	}
	return best
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
