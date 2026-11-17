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
		"- writes the legacy Ghostty workspace and reserves local ports\n" +
		"- can later materialize a persistent Rex or Herdr workspace with `kit open`\n\n" +
		"Passing `name` pre-fills the wizard's first field. A leading `liftoff-`\n" +
		"is stripped from your input.\n\n" +
		"`--checkout-backend twig` opts into copy-on-write source checkouts with\n" +
		"released Twig >=0.4.0. Git remains the default. Twig may delegate to Git;\n" +
		"the branch still starts from the latest fetched upstream commit.\n\n" +
		"Alias: `new`.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prefill := ""
		if len(args) == 1 {
			prefill = args[0]
		}
		backend, err := cmd.Flags().GetString("checkout-backend")
		if err != nil {
			return err
		}
		return tui.RunDesignTUI(liftoff.DefaultLayout(), prefill, backend)
	},
}

func init() {
	designCmd.Flags().String("checkout-backend", "git", "Source checkout backend: git or twig (opt-in copy-on-write)")
	rootCmd.AddCommand(designCmd)
}
