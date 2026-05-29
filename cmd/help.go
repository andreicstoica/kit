package cmd

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// RenderMarkdownLongs walks the cobra command tree and replaces every
// command's Long body with a glamour-rendered version. fang then prints the
// result verbatim — markdown turns into styled terminal output (bold,
// headings, inline code, lists) without giving up fang's USAGE/COMMANDS/FLAGS
// scaffolding.
//
// It is a no-op unless help is actually being requested: rendering is only
// observable on `-h`/`--help`/`kit help`, but it spins up a glamour renderer
// (chroma styles + a markdown AST pass) which is wasted work on every other
// invocation — `kit play`, `kit log`, shell completion, etc. Guarding here
// keeps the hot path (running an actual command) free of that cost.
//
// Falls back to the raw text if glamour fails or stdout isn't a TTY.
func RenderMarkdownLongs() {
	if !helpRequested() {
		return
	}
	r := newMarkdownRenderer()
	renderTree(rootCmd, r)
}

// helpRequested reports whether this invocation will actually show help text.
func helpRequested() bool {
	for _, a := range os.Args[1:] {
		switch a {
		case "-h", "--help", "help":
			return true
		}
	}
	return false
}

// newMarkdownRenderer builds one glamour renderer to reuse across the tree,
// or nil when output isn't a terminal (help is then printed as raw markdown).
func newMarkdownRenderer() *glamour.TermRenderer {
	if !isTerminal(os.Stdout) {
		return nil
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
		return nil
	}
	return r
}

func renderTree(c *cobra.Command, r *glamour.TermRenderer) {
	if c.Long != "" {
		c.Long = renderMarkdown(c.Long, r)
	}
	for _, sub := range c.Commands() {
		renderTree(sub, r)
	}
}

func renderMarkdown(body string, r *glamour.TermRenderer) string {
	if r == nil {
		return strings.TrimRight(body, "\n")
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
