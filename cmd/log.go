package cmd

import (
	"fmt"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:     "log [name]",
	Aliases: []string{"logs"},
	Short:   "Tail all service logs for a kit",
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
		layout := liftoff.DefaultLayout()
		name, err := resolveTarget(layout, args, "kit log — pick a kit", true)
		if err != nil {
			return err
		}
		if name == "" {
			return nil
		}
		if len(args) == 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "tailing %s\n", name)
		}
		return tui.RunLogTUI(name)
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
}
