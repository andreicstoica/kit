package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var restartOnly []string

var restartCmd = &cobra.Command{
	Use:     "restart [name]",
	Aliases: []string{"bounce"},
	Short:   "Stop then start a kit's services — handy when one hangs",
	Long: "**restart** stops and re-spawns a worktree's services. With no " +
		"`--only`, it restarts exactly the services currently running, so a " +
		"hung frontend can be bounced without touching the rest.\n\n" +
		"Headless and scriptable — prints each service's status and the log " +
		"dir on exit.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit restart — pick a kit")
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}

		cfg, err := liftoff.LoadConfig()
		if err != nil {
			return err
		}
		slot, err := resolveSlot(cfg, name)
		if err != nil {
			return err
		}
		path, err := layout.ResolveWorktreePath(name)
		if err != nil {
			return err
		}
		ports := liftoff.PortsForSlot(slot)

		only, err := parseServiceList(restartOnly)
		if err != nil {
			return err
		}
		svcs := restartTargets(name, ports, only)
		if len(svcs) == 0 {
			return fmt.Errorf("nothing running for %s — use `kit play %s`", name, name)
		}

		for _, svc := range svcs {
			fmt.Printf("  stopping %s…\n", svc.Label())
			if err := liftoff.StopService(name, svc); err != nil {
				fmt.Println(tui.StyleErr.Render("    " + err.Error()))
			}
		}

		plan := liftoff.PlayPlan{
			Worktree:     name,
			WorktreePath: path,
			Slot:         slot,
			Ports:        ports,
			Services:     svcs,
		}
		for upd := range layout.RunPlay(plan) {
			switch upd.Status {
			case liftoff.StepDone:
				line := "  ✓ " + upd.Title
				if upd.URL != "" {
					line += "  " + upd.URL
				}
				fmt.Println(tui.StyleOK.Render(line))
			case liftoff.StepFailed:
				msg := upd.Title
				if upd.Err != nil {
					msg += ": " + upd.Err.Error()
				}
				fmt.Println(tui.StyleErr.Render("  ✗ " + msg))
			}
		}
		fmt.Println(tui.StyleDim.Render("logs: " + liftoff.RunDirPath(name)))
		return nil
	},
}

func init() {
	restartCmd.Flags().StringSliceVar(&restartOnly, "only", nil,
		"comma-separated services to restart (default: whatever's running)")
	rootCmd.AddCommand(restartCmd)
}

// restartTargets resolves which services to bounce: the explicit --only list,
// else everything currently alive. Celery pulls in beat (they're always paired).
func restartTargets(name string, ports liftoff.Ports, only []liftoff.Service) []liftoff.Service {
	want := map[liftoff.Service]bool{}
	if len(only) > 0 {
		for _, s := range only {
			want[s] = true
		}
	} else {
		for _, s := range liftoff.DisplayServices {
			if liftoff.IsServiceAlive(name, s, ports) {
				want[s] = true
			}
		}
	}
	if want[liftoff.SvcCelery] {
		want[liftoff.SvcBeat] = true
	}
	var out []liftoff.Service
	for _, s := range liftoff.AllServices {
		if want[s] {
			out = append(out, s)
		}
	}
	return out
}
