package cmd

import (
	"fmt"
	"os"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log [name]",
	Short: "Tail all service logs for a kit",
	Long: "Tails every `.log` under `~/.config/kit/run/<name>/` in a scrollable viewport.\n\n" +
		"Each line is prefixed with its service tag. Service tags are color-coded.\n\n" +
		"Keys:\n\n" +
		"- `f` toggle follow (auto-scroll to bottom)\n" +
		"- `↑/↓ k/j` scroll line\n" +
		"- `pgup/pgdn` scroll page\n" +
		"- `g/G` top/bottom\n" +
		"- `?` toggle help\n" +
		"- `q` / `ctrl+c` exit",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var name string
		if len(args) == 1 {
			n, err := liftoff.NormalizeAndValidate(args[0])
			if err != nil {
				return err
			}
			name = n
		} else {
			st, _ := liftoff.LoadState()
			if st == nil || len(st.Worktrees) == 0 {
				return fmt.Errorf("no worktrees in state — run `kit play` first or pass a name")
			}
			// Pick the most recently used worktree.
			names := st.SortedNames()
			if len(names) == 0 {
				return fmt.Errorf("no worktrees in state")
			}
			name = names[0]
			fmt.Fprintf(cmd.ErrOrStderr(), "tailing %s (most recent)\n", name)
		}
		// Pre-flight: bail before entering TUI if no run dir exists for this kit.
		dir := liftoff.RunDirPath(name)
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("no run dir for %s — run `kit play %s` first", name, name)
		}
		return tui.RunLogTUI(name)
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
}
