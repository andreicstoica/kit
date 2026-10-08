package cmd

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var rexContextSession string
var rexActionNotify bool
var rexCmd = &cobra.Command{Use: "rex", Short: "Workspace-aware Rex palette integration"}

var rexCurrentCmd = &cobra.Command{
	Use: "current", Short: "Print the Kit workspace owned by a Rex session", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := liftoff.RexWorkspaceName(rexContextSession)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), name)
		return nil
	},
}

var rexActionCmd = &cobra.Command{
	Use: "action <play|pause|park|resume>", Short: "Run a current-workspace operation without a picker", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		action := args[0]
		switch action {
		case "play", "pause", "park", "resume":
		default:
			return fmt.Errorf("unsupported direct Rex action %q", action)
		}
		name, err := liftoff.RexWorkspaceName(rexContextSession)
		if err != nil {
			return err
		}
		return liftoff.WithWorkspaceLock("rex-operation:"+name, func() error {
			dir, err := liftoff.RunDir(name)
			if err != nil {
				return err
			}
			log, err := os.OpenFile(filepath.Join(dir, "rex-operation.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			defer log.Close()
			w := io.MultiWriter(cmd.OutOrStdout(), log)
			fmt.Fprintf(w, "Kit %s: %s\n", action, name)
			rexProgramStatus(cmd.OutOrStdout(), "working", action+" "+name)
			layout := liftoff.DefaultLayout()
			switch action {
			case "play":
				var cfg *liftoff.Config
				cfg, err = liftoff.LoadConfig()
				if err == nil {
					if cfg.Worktrees[name].Parked {
						err = fmt.Errorf("%s is parked; resume it before starting services", name)
					} else {
						err = tui.RunPlayHeadless(layout, tui.PlayConfig{Name: name}, w)
					}
				}
			case "pause":
				err = tui.RunPauseHeadless(layout, tui.PauseConfig{Name: name}, w)
			case "park":
				err = layout.Park(name)
			case "resume":
				err = layout.Resume(name)
			}
			state, message := "done", action+" finished for "+name
			if err != nil {
				state, message = "error", action+" failed for "+name
				fmt.Fprintln(w, err)
			}
			fmt.Fprintln(w, message)
			rexProgramStatus(cmd.OutOrStdout(), state, message)
			if rexActionNotify {
				_ = exec.Command("/usr/bin/osascript", "-e", `on run argv
display notification (item 1 of argv) with title "Kit"
end run`, message).Run()
			}
			return err
		})
	},
}

var rexLogCmd = &cobra.Command{
	Use: "log", Short: "Show the current workspace's last direct palette-operation log", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := liftoff.RexWorkspaceName(rexContextSession)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(liftoff.RunDirPath(name), "rex-operation.log"))
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	},
}

func rexProgramStatus(w io.Writer, state, message string) {
	fmt.Fprintf(w, "\x1b]7501;state=%s:app=kit:msg=%s\x1b\\", state, base64.StdEncoding.EncodeToString([]byte(message)))
}

func init() {
	rexCmd.PersistentFlags().StringVar(&rexContextSession, "session", "", "exact Rex session ID")
	_ = rexCmd.MarkPersistentFlagRequired("session")
	rexActionCmd.Flags().BoolVar(&rexActionNotify, "notify", false, "send a macOS completion notification")
	rexCmd.AddCommand(rexCurrentCmd, rexActionCmd, rexLogCmd)
	rootCmd.AddCommand(rexCmd)
}
