package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var (
	logDeleteAll bool
	logWait      bool
)

var logCmd = &cobra.Command{
	Use:     "log [name]",
	Aliases: []string{"logs"},
	Short:   "Tail all service logs for a kit",
	Long: "Tails every `.log` under `~/.config/kit/run/<name>/` in a scrollable viewport.\n\n" +
		"Each line is prefixed with its service tag. Service tags are color-coded.\n\n" +
		"Keys:\n\n" +
		"- `f` toggle follow (auto-scroll to bottom)\n" +
		"- `/` open search (case-insensitive substring)\n" +
		"- `t` open services panel — toggle which streams show\n" +
		"- `↑/↓ k/j` scroll line\n" +
		"- `pgup/pgdn` scroll page\n" +
		"- `g/G` top/bottom\n" +
		"- `?` toggle help\n" +
		"- `q` / `ctrl+c` exit\n\n" +
		"Pass `--delete-all` to clear every `.log` for the worktree instead " +
		"of opening the viewer.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeWorktreeNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit log — pick a kit")
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}
		if logDeleteAll {
			return clearLogsFor(name)
		}
		if !logWait {
			if err := ensurePlaying(name); err != nil {
				return err
			}
		} else if err := ensureRunDir(name); err != nil {
			return err
		}
		if len(args) == 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "tailing %s\n", name)
		}
		return tui.RunLogTUI(name)
	},
}

func init() {
	logCmd.Flags().BoolVar(&logDeleteAll, "delete-all", false, "delete every .log for the worktree (confirms first)")
	logCmd.Flags().BoolVar(&logWait, "wait", false, "open the viewer even when nothing is playing (waits for logs to appear)")
	rootCmd.AddCommand(logCmd)
}

// ensureRunDir creates the run dir + pre-touches the default service log
// files so the log viewer can open and start tailing before `kit play`
// has actually run. Used in --wait mode (e.g. gtab logs tab).
func ensureRunDir(name string) error {
	if _, err := liftoff.RunDir(name); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	for _, svc := range []liftoff.Service{
		liftoff.SvcApp, liftoff.SvcAdmin, liftoff.SvcAPI,
		liftoff.SvcAdminBE, liftoff.SvcCelery,
	} {
		path, _ := liftoff.LogFile(name, string(svc))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		_ = f.Close()
	}
	return nil
}

// ensurePlaying blocks `kit log` when no services are alive — a stopped
// worktree just shows stale lines with no follow activity.
func ensurePlaying(name string) error {
	if name == "master" {
		return nil
	}
	cfg, err := liftoff.LoadConfig()
	if err != nil || cfg == nil {
		return fmt.Errorf("no kit config yet — run `kit play %s` first", name)
	}
	meta, ok := cfg.Worktrees[name]
	if !ok || meta.Slot == 0 {
		return fmt.Errorf("%s has no slot — run `kit play %s` first", name, name)
	}
	alive, _ := liftoff.RunningCount(name, liftoff.PortsForSlot(meta.Slot))
	if alive == 0 {
		return fmt.Errorf("nothing playing for %s — run `kit play %s` first", name, name)
	}
	return nil
}

// clearLogsFor truncates every .log under the worktree's run dir.
// Files stay in place (running tails keep their FD); contents get
// emptied so the next read starts fresh.
func clearLogsFor(name string) error {
	dir := liftoff.RunDirPath(name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("no log dir for %s — run `kit play` first", name)
	}
	var paths []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".log" {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	if len(paths) == 0 {
		fmt.Println(tui.StyleOK.Render("no .log files to clear in " + dir))
		return nil
	}
	fmt.Println(tui.StyleTitle.Render(fmt.Sprintf("clear %d log file(s) in %s?", len(paths), dir)))
	for _, p := range paths {
		fmt.Println("  " + tui.StyleDim.Render(filepath.Base(p)))
	}
	fmt.Println()
	accept := true
	if err := huh.NewConfirm().
		Title("Delete contents?").
		Description("Truncates each .log to 0 bytes; files stay so running tails keep their FD.").
		Affirmative("Yes, clear").
		Negative("Cancel").
		Value(&accept).Run(); err != nil {
		return err
	}
	if !accept {
		return nil
	}
	var failed []string
	for _, p := range paths {
		if err := os.Truncate(p, 0); err != nil {
			failed = append(failed, p+": "+err.Error())
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("some files failed:\n  %s", strings.Join(failed, "\n  "))
	}
	fmt.Println(tui.StyleOK.Render(fmt.Sprintf("✓ cleared %d log file(s)", len(paths))))
	return nil
}
