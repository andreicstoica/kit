package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/spf13/cobra"
)

// The brief travels as one command-line argument through Rex to the harness,
// so it must stay well under the platform argument limit.
const agentBriefLimit = 32 << 10

type agentLauncher func(layout liftoff.Layout, name string, argv []string) (liftoff.RexAgentLaunch, error)

func newAgentLaunchCommand(launch agentLauncher, lookPath func(string) (string, error)) *cobra.Command {
	var harness, model, briefPath, prompt string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "launch NAME",
		Short: "Open a fresh agent in a worktree's Rex session",
		Long: `launch starts a fresh interactive agent in the named worktree's Rex session
and selects its window. The calling agent keeps running.

The default is Claude Code with Opus. fx, Codex, and OpenCode use their own
configured model unless overridden. OpenCode does not accept a model override.

NAME must be an existing Kit feature worktree that is not parked or pending
cleanup. Create it first with kit design. launch never creates a worktree,
database, or services, and never passes a permission-bypass flag.

Use --prompt TEXT or --brief FILE (or --brief - for stdin) to send a first
message. Omit both to open an empty thread. The message must be UTF-8 text
of at most 32 KiB. Kit passes it without a shell and does not print it. It is
visible in process arguments, so do not include secrets.

If the agent window opens but Rex cannot select it, launch still exits 0 and
prints a warning. If the Rex request fails and Kit cannot confirm whether the
window opened, the error says so. In both cases, check the worktree's Rex
session before you launch again.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeWorktreeNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			model, err := launchModel(harness, model, cmd.Flags().Changed("model"))
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("brief") && cmd.Flags().Changed("prompt") {
				return fmt.Errorf("choose either --brief or --prompt")
			}
			var firstMessage string
			switch {
			case cmd.Flags().Changed("brief"):
				if briefPath == "" {
					return fmt.Errorf("--brief requires a file path or - for stdin")
				}
				firstMessage, err = readAgentBrief(briefPath, cmd.InOrStdin())
			case cmd.Flags().Changed("prompt"):
				firstMessage, err = validateAgentBrief([]byte(prompt))
			}
			if err != nil {
				return err
			}
			bin, err := lookPath(harness)
			if err != nil {
				return fmt.Errorf("find %s on PATH: %w", harness, err)
			}
			bin, err = filepath.Abs(bin)
			if err != nil {
				return err
			}
			argv := []string{bin}
			if model != "" {
				argv = append(argv, "--model", model)
			}
			if firstMessage != "" {
				if harness == "opencode" {
					argv = append(argv, "--prompt")
				}
				argv = append(argv, firstMessage)
			}
			result, err := launch(liftoff.DefaultLayout(), args[0], argv)
			if err != nil {
				return err
			}
			return printAgentLaunch(cmd, result, harness, model, asJSON)
		},
	}
	cmd.Flags().StringVar(&harness, "harness", "claude", "interactive harness: claude, fx, codex, or opencode")
	cmd.Flags().StringVar(&model, "model", "", "model ID or alias (default: Opus for Claude, harness configuration otherwise)")
	cmd.Flags().StringVar(&prompt, "prompt", "", "first message as inline text")
	cmd.Flags().StringVar(&briefPath, "brief", "", "task brief file, or - for stdin")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print one JSON object")
	return cmd
}

func launchModel(harness, model string, specified bool) (string, error) {
	switch harness {
	case "claude", "fx", "codex", "opencode":
	default:
		return "", fmt.Errorf("unknown harness %q; choose claude, fx, codex, or opencode", harness)
	}
	if !specified {
		if harness == "claude" {
			return "opus", nil
		}
		return "", nil
	}
	if harness == "opencode" {
		return "", fmt.Errorf("OpenCode's interactive CLI does not support --model; configure its default model instead")
	}
	model = strings.TrimSpace(model)
	if model == "" || strings.HasPrefix(model, "-") || !utf8.ValidString(model) || strings.ContainsFunc(model, unicode.IsControl) {
		return "", fmt.Errorf("--model requires a model ID or alias, not a CLI option or control text")
	}
	return model, nil
}

func readAgentBrief(path string, stdin io.Reader) (string, error) {
	r := stdin
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("open brief: %w", err)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return "", fmt.Errorf("read brief: %w", err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("brief must be a regular file or - for stdin")
		}
		r = file
	}
	data, err := io.ReadAll(io.LimitReader(r, agentBriefLimit+1))
	if err != nil {
		return "", fmt.Errorf("read brief: %w", err)
	}
	return validateAgentBrief(data)
}

// validateAgentBrief never quotes the brief in its errors.
func validateAgentBrief(data []byte) (string, error) {
	if len(data) > agentBriefLimit {
		return "", fmt.Errorf("brief is larger than %d bytes; shorten it", agentBriefLimit)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("brief must be UTF-8 text")
	}
	brief := strings.TrimSpace(string(data))
	if brief == "" {
		return "", fmt.Errorf("brief is empty")
	}
	// A leading dash can make a harness parse the brief as an option.
	if strings.HasPrefix(brief, "-") {
		return "", fmt.Errorf("brief must not start with -")
	}
	for _, r := range brief {
		if r == '\n' || r == '\t' || r == '\r' {
			continue
		}
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return "", fmt.Errorf("brief contains a NUL or terminal control character")
		}
	}
	return brief, nil
}

func printAgentLaunch(cmd *cobra.Command, result liftoff.RexAgentLaunch, harness, model string, asJSON bool) error {
	focusError := ""
	if result.FocusErr != nil {
		focusError = result.FocusErr.Error()
	}
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		return enc.Encode(map[string]any{
			"name":        result.Name,
			"path":        result.Path,
			"harness":     harness,
			"model":       model,
			"session_id":  result.SessionID,
			"window_id":   result.WindowID,
			"launched":    true,
			"focused":     result.FocusErr == nil,
			"focus_error": focusError,
		})
	}
	out := cmd.OutOrStdout()
	label := harness
	if model != "" {
		label += " (" + model + ")"
	}
	fmt.Fprintf(out, "Launched %s in %s\n", label, result.Name)
	fmt.Fprintf(out, "  path:    %s\n", result.Path)
	fmt.Fprintf(out, "  session: %s\n", result.SessionID)
	if result.WindowID != "" {
		fmt.Fprintf(out, "  window:  %s\n", result.WindowID)
	}
	if result.FocusErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: the agent window is open, but Rex did not select it: %s\n", focusError)
		fmt.Fprintln(cmd.ErrOrStderr(), "Select the agent window in the Rex app and check it before you launch again.")
	}
	return nil
}

func defaultAgentLaunchCommand() *cobra.Command {
	return newAgentLaunchCommand(liftoff.LaunchRexAgent, exec.LookPath)
}
