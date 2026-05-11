package cmd

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// RenderMarkdownLongs walks the cobra command tree once at startup and
// replaces every command's Long body with a glamour-rendered version.
// fang then prints the result verbatim — markdown turns into styled
// terminal output (bold, headings, inline code, lists) without giving
// up fang's USAGE/COMMANDS/FLAGS scaffolding.
//
// Falls back to the raw text if glamour fails or stdout isn't a TTY.
func RenderMarkdownLongs() {
	renderTree(rootCmd)
}

func renderTree(c *cobra.Command) {
	if c.Long != "" {
		c.Long = renderMarkdown(c.Long)
	}
	for _, sub := range c.Commands() {
		renderTree(sub)
	}
}

func renderMarkdown(body string) string {
	if !isTerminal(os.Stdout) {
		return strings.TrimRight(body, "\n")
	}
	wrap := termWidth() - 4
	if wrap < 40 {
		wrap = 40
	}
	if wrap > 100 {
		wrap = 100
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(wrap),
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

func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
