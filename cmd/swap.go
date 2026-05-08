package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var swapCmd = &cobra.Command{
	Use:     "swap <name> [editor]",
	Aliases: []string{"open"},
	Short:   "Sub into a kit — open its worktree in your IDE",
	Long: "**swap** opens the worktree for `<name>` in your editor.\n\n" +
		"## Examples\n\n" +
		"```\n" +
		"kit swap notebook            # uses default editor\n" +
		"kit swap notebook cursor     # forces cursor\n" +
		"kit swap notebook zed        # forces zed\n" +
		"```\n\n" +
		"## Editor selection (first match wins)\n\n" +
		"1. positional `[editor]` arg if provided\n" +
		"2. `$KIT_EDITOR` env var\n" +
		"3. `$VISUAL` / `$EDITOR`\n" +
		"4. `zed` if installed\n" +
		"5. `cursor` if installed\n" +
		"6. `code` if installed",
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		name, err := liftoff.NormalizeAndValidate(args[0])
		if err != nil {
			return err
		}
		path := layout.WorktreePath(name)
		if _, err := os.Stat(path); err != nil {
			legacy := layout.LegacyWorktreePath(name)
			if _, err2 := os.Stat(legacy); err2 == nil {
				path = legacy
			} else {
				return fmt.Errorf("worktree not found: %s", path)
			}
		}
		var editor string
		if len(args) == 2 {
			editor = args[1]
			if _, err := exec.LookPath(editor); err != nil {
				return fmt.Errorf("editor %q not on PATH", editor)
			}
		} else {
			editor = pickEditor()
		}
		if editor == "" {
			return fmt.Errorf("no editor found — pass one as 2nd arg or set KIT_EDITOR")
		}
		c := exec.Command(editor, path)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Start(); err != nil {
			return err
		}
		// Best-effort: bump last_used so lineup sorts this kit to the top.
		if st, err := liftoff.LoadState(); err == nil {
			st.TouchLastUsed(name)
			_ = st.Save()
		}
		fmt.Printf("opened %s in %s\n", path, editor)
		return nil
	},
}

func pickEditor() string {
	candidates := []string{
		os.Getenv("KIT_EDITOR"),
		os.Getenv("VISUAL"),
		os.Getenv("EDITOR"),
		"zed",
		"cursor",
		"code",
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}
	return ""
}

func init() {
	rootCmd.AddCommand(swapCmd)
}
