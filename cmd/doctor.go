package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var doctorSelfTest bool

var doctorCmd = &cobra.Command{
	Use:     "doctor",
	Aliases: []string{"physio"},
	Short:   "Diagnose your kit setup",
	Long: "**doctor** runs a series of read-only checks to confirm your toolchain " +
		"is ready for `kit`.\n\nChecks:\n\n" +
		"- Homebrew (and whether brew is on PATH)\n" +
		"- git, gh (with auth), node + yarn, python (with venv)\n" +
		"- redis, postgres (running on default ports)\n" +
		"- Ghostty, an editor (Zed / Cursor / VS Code)\n" +
		"- The Liftoff master repo and its node_modules\n\n" +
		"Exits non-zero on any failure. Pair with `kit setup` to fix what's " +
		"missing. Pass `--self-test` for runtime smoke checks on top of the " +
		"static toolchain audit (master git ping, config round-trip, port " +
		"bindability).",
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		results := liftoff.RunChecks(liftoff.DefaultChecks(layout))
		fmt.Print(tui.RenderDoctor(results))

		failed := liftoff.AnyFailed(results)
		if doctorSelfTest {
			fmt.Println()
			runtimeFails := runSelfTest(layout)
			if runtimeFails > 0 {
				failed = true
			}
		}
		if failed {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorSelfTest, "self-test", false, "extra runtime smoke checks (master git, config IO, port bindable)")
	rootCmd.AddCommand(doctorCmd)
}

// runSelfTest performs end-to-end smoke checks that the static tool
// audit doesn't catch: can kit actually talk to the repo, write its
// config, and bind ports? Returns the number of failed probes.
func runSelfTest(layout liftoff.Layout) int {
	fmt.Println(tui.StyleTitle.Render("self-test"))

	type probe struct {
		name string
		run  func() error
	}
	probes := []probe{
		{"master git rev-parse", func() error {
			c := exec.Command("git", "rev-parse", "HEAD")
			c.Dir = layout.Master
			out, err := c.Output()
			if err != nil {
				return err
			}
			if len(strings.TrimSpace(string(out))) == 0 {
				return fmt.Errorf("empty HEAD")
			}
			return nil
		}},
		{"config round-trip", func() error {
			return liftoff.WithConfigLock(func(c *liftoff.Config) error { return nil })
		}},
		{"port slot 99 bindable", func() error {
			if !liftoff.PortsBindable(99) {
				return fmt.Errorf("slot 99 ports occupied — something else on those ports")
			}
			return nil
		}},
		{"worktree list reachable", func() error {
			_, err := layout.ListWorktrees()
			return err
		}},
		{"run dir writable", func() error {
			home, _ := os.UserHomeDir()
			dir := filepath.Join(home, ".config", "kit", "run", ".selftest")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			return os.RemoveAll(dir)
		}},
	}

	failed := 0
	for _, p := range probes {
		err := p.run()
		if err != nil {
			fmt.Printf("  %s %s  %s\n", tui.StyleErr.Render("✗"), p.name, tui.StyleErr.Render(err.Error()))
			failed++
		} else {
			fmt.Printf("  %s %s\n", tui.StyleOK.Render("✓"), p.name)
		}
	}
	fmt.Println()
	if failed > 0 {
		fmt.Println(tui.StyleErr.Render(fmt.Sprintf("%d self-test probe(s) failed", failed)))
	} else {
		fmt.Println(tui.StyleOK.Render(fmt.Sprintf("✓ all %d self-test probes passed", len(probes))))
	}
	return failed
}
