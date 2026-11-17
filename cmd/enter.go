package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

var fxSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

var enterCmd = newEnterCmd(enterInteractive, syscall.Exec)

func newEnterCmd(isInteractive func() bool, replaceProcess func(string, []string, []string) error) *cobra.Command {
	var session string
	var printOnly bool
	cmd := &cobra.Command{
		Use:   "enter [name] --session <fx-id>",
		Short: "Resume an explicit fx session in a worktree",
		Long: `enter resolves a worktree by name, or from the current directory when omitted.

Finish the current turn and exit the original fx instance first, then run enter
from a foreground terminal. Never resume the same session concurrently.
It changes directory and replaces only this kit process with fx --resume ID.
It cannot change the cwd of an already-running parent shell or agent, and never
launches a detached fx process. fx associates the resumed session with the
worktree; kit does not move or edit fx session files.

Without a foreground terminal, --print is required. It only prints a safely
quoted shell command to run after exiting the existing fx instance, without
changing directory, starting fx, or modifying kit state.`,
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeWorktreeNames,
		// Setup and update hooks may mutate state; enter must remain read-only until exec.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateFXSessionID(session); err != nil {
				return err
			}
			if !printOnly && !isInteractive() {
				return fmt.Errorf("kit enter requires a foreground interactive terminal; use --print, then exit the existing fx instance and run the printed command")
			}

			layout := liftoff.DefaultLayout()
			cfg, err := liftoff.LoadConfig()
			if err != nil {
				return err
			}
			var name string
			if len(args) == 1 {
				if _, ok := cfg.Worktrees[args[0]]; ok && filepath.Base(args[0]) == args[0] && args[0] != "." && args[0] != ".." {
					name = args[0]
				}
			}
			if name == "" {
				name, err = resolveArgOrCwd(layout, args)
				if err != nil {
					return err
				}
			}
			if name == "" {
				return fmt.Errorf("specify a worktree name or run kit enter inside a kit worktree")
			}
			meta := cfg.Worktrees[name]
			path := meta.Path
			if name == "master" || path == "" {
				path, err = layout.ResolveWorktreePath(name)
				if err != nil {
					return err
				}
			}
			path, err = filepath.Abs(path)
			if err != nil {
				return err
			}
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("worktree directory: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("worktree path is not a directory: %s", path)
			}
			if err := layout.VerifyGitCheckout(name, path, meta.Branch); err != nil {
				return err
			}
			if printOnly {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "cd -- %s && fx --resume %s\n", liftoff.ShellQuote(path), liftoff.ShellQuote(session))
				return err
			}
			return execFXInWorktree(path, session, replaceProcess)
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "explicit saved fx session ID (not last)")
	cmd.Flags().BoolVar(&printOnly, "print", false, "only print the shell command; do not resume or change state")
	_ = cmd.MarkFlagRequired("session")
	return cmd
}

func validateFXSessionID(id string) error {
	if id == "last" || !fxSessionIDPattern.MatchString(id) {
		return fmt.Errorf("--session requires an explicit fx session ID using letters, digits, underscores, or dashes, starting with a letter or digit (not last)")
	}
	return nil
}

func enterInteractive() bool {
	if !interactive() {
		return false
	}
	foreground, err := unix.IoctlGetInt(int(os.Stdin.Fd()), unix.TIOCGPGRP)
	return err == nil && foreground == unix.Getpgrp()
}

func execFXInWorktree(path, session string, replaceProcess func(string, []string, []string) error) (err error) {
	binary, err := exec.LookPath("fx")
	if err != nil {
		return fmt.Errorf("find fx on PATH: %w", err)
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return err
	}
	cwd, err := os.Open(".")
	if err != nil {
		return err
	}
	defer cwd.Close()
	if err := os.Chdir(path); err != nil {
		return fmt.Errorf("enter worktree: %w", err)
	}
	// Exec never returns on success; restore cwd if replacement fails.
	defer func() { err = errors.Join(err, cwd.Chdir()) }()

	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PWD=") {
			env = append(env, entry)
		}
	}
	env = append(env, "PWD="+path)
	if err := replaceProcess(binary, []string{"fx", "--resume", session}, env); err != nil {
		return fmt.Errorf("resume fx session: %w", err)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(enterCmd)
}
