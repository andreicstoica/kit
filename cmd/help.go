package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// helpHeader styles the command title.
var helpHeader = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.AdaptiveColor{Light: "#0F8A4E", Dark: "#5DD39E"})

var helpUse = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#3F4750", Dark: "#9aa5b1"})

var helpDim = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#7C8590", Dark: "#6c7086"})

func init() {
	rootCmd.SetHelpFunc(prettyHelp)
	for _, c := range rootCmd.Commands() {
		c.SetHelpFunc(prettyHelp)
	}
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
func renderMarkdown(body string) string {
	if !isTerminal(os.Stdout) {
		return strings.TrimRight(body, "\n")
	}
	style := "auto"
	if v := os.Getenv("KIT_GLAMOUR_STYLE"); v != "" {
		style = v
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		return body
	}
	out, err := r.Render(body)
	if err != nil {
		return body
	}
	return strings.TrimRight(out, "\n")
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
