package cmd

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var designCmd = &cobra.Command{
	Use:     "design [name]",
	Aliases: []string{"new"},
	Short:   "Create a fresh workspace",
	Long: "**design** walks you through creating a new Liftoff workspace:\n\n" +
		"- asks for a short feature name\n" +
		"- creates a separate folder and code branch\n" +
		"- copies app settings from master\n" +
		"- optionally copies your local database for safe experiments\n" +
		"- installs backend dependencies\n" +
		"- optionally reuses master frontend packages to save disk/time\n" +
		"- optionally adds the branch to Graphite\n" +
		"- creates a Ghostty workspace and reserves local ports\n\n" +
		"Passing `name` pre-fills the wizard's first field. A leading `liftoff-`\n" +
		"is stripped from your input.\n\n" +
		"Alias: `new`.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prefill := ""
		if len(args) == 1 {
			prefill = args[0]
		}
		return tui.RunDesignTUI(liftoff.DefaultLayout(), prefill)
	},
}

func init() {
	rootCmd.AddCommand(designCmd)
}
