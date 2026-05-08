package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

var swapCmd = &cobra.Command{
	Use:     "swap <name>",
	Aliases: []string{"open"},
	Short:   "Sub into a kit — open its worktree in your IDE",
	Long: `swap opens the worktree for <name> in your editor.

Editor selection (first match wins):
  $KIT_EDITOR             explicit override (must be on PATH)
  $VISUAL or $EDITOR      generic editor env vars
  zed (if installed)
  cursor (if installed)
  code (if installed)`,
	Args: cobra.ExactArgs(1),
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
		editor := pickEditor()
		if editor == "" {
			return fmt.Errorf("no editor found. Set KIT_EDITOR or install zed/cursor/code")
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
