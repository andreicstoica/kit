package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/andreicstoica/kit/internal/tui"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	helpHeader = lipgloss.NewStyle().Bold(true).Foreground(tui.ColorAccent)
	helpUse    = lipgloss.NewStyle().Foreground(tui.ColorMuted)
	helpDim    = lipgloss.NewStyle().Foreground(tui.ColorDim)
)

func init() {
	// Setting on the root only; cobra inherits the help func down the tree
	// for any command that doesn't set its own, so subcommands registered
	// in other init() functions pick this up automatically.
	rootCmd.SetHelpFunc(prettyHelp)
}

// prettyHelp renders cobra help with glamour for the Long body.
// Falls back to plain rendering if glamour fails (e.g. non-tty).
func prettyHelp(cmd *cobra.Command, args []string) {
	out := cmd.OutOrStdout()

	// Header.
	fmt.Fprintln(out)
	if cmd.HasParent() {
		fmt.Fprintln(out, helpHeader.Render(cmd.CommandPath()))
	} else {
		fmt.Fprintln(out, helpHeader.Render("kit"))
	}
	fmt.Fprintln(out, helpDim.Render(strings.Repeat("─", 40)))

	if cmd.Short != "" {
		fmt.Fprintln(out, cmd.Short)
		fmt.Fprintln(out)
	}

	// Glamour-rendered Long.
	if cmd.Long != "" {
		body := renderMarkdown(cmd.Long)
		fmt.Fprintln(out, body)
		fmt.Fprintln(out) // blank line before next section
	}

	// Usage.
	fmt.Fprintln(out, helpHeader.Render("Usage"))
	fmt.Fprintln(out, "  "+helpUse.Render(cmd.UseLine()))
	fmt.Fprintln(out)

	// Aliases.
	if len(cmd.Aliases) > 0 {
		fmt.Fprintln(out, helpHeader.Render("Aliases"))
		fmt.Fprintln(out, "  "+helpUse.Render(strings.Join(cmd.Aliases, ", ")))
		fmt.Fprintln(out)
	}

	// Subcommands.
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintln(out, helpHeader.Render("Subcommands"))
		for _, sub := range cmd.Commands() {
			if sub.Hidden {
				continue
			}
			fmt.Fprintf(out, "  %-12s %s\n", sub.Name(), helpDim.Render(sub.Short))
		}
		fmt.Fprintln(out)
	}

	// Flags.
	if cmd.HasAvailableLocalFlags() {
		fmt.Fprintln(out, helpHeader.Render("Flags"))
		fmt.Fprintln(out, indent(cmd.LocalFlags().FlagUsages(), "  "))
	}
	if cmd.HasAvailableInheritedFlags() {
		fmt.Fprintln(out, helpHeader.Render("Global flags"))
		fmt.Fprintln(out, indent(cmd.InheritedFlags().FlagUsages(), "  "))
	}

	if cmd.HasParent() {
		fmt.Fprintf(out, "%s `%s <command> --help`\n",
			helpDim.Render("subcommand help:"),
			cmd.Root().Name())
	}
}

// renderMarkdown pipes the body through glamour. Returns the body unchanged
// on failure (e.g. when stdout is not a terminal).
//
// Width is the terminal width minus a small margin so list items + headings
// don't run flush against the right edge. Style auto-picks dark/light from
// the terminal background.
func renderMarkdown(body string) string {
	if !isTerminal(os.Stdout) {
		return strings.TrimRight(body, "\n")
	}
	width := termWidth()
	wrap := width - 2
	if wrap < 40 {
		wrap = 40
	}
	if wrap > 100 {
		// Cap to keep prose readable on ultra-wide terminals.
		wrap = 100
	}

	opts := []glamour.TermRendererOption{
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(wrap),
	}
	if v := os.Getenv("KIT_GLAMOUR_STYLE"); v != "" {
		opts = []glamour.TermRendererOption{
			glamour.WithStandardStyle(v),
			glamour.WithWordWrap(wrap),
		}
	}
	r, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return body
	}
	out, err := r.Render(body)
	if err != nil {
		return body
	}
	return strings.TrimRight(out, "\n")
}

// termWidth returns the current terminal width, or 80 if unavailable.
func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
