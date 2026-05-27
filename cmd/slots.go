package cmd

import (
	"fmt"
	"sort"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var slotsCmd = &cobra.Command{
	Use:   "slots",
	Short: "Show port-slot allocations",
	Long: "**slots** lists every adopted worktree's slot. Use `kit slots renumber` " +
		"to compact gaps left after washing kits.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := liftoff.LoadConfig()
		if err != nil {
			return err
		}
		type row struct {
			name string
			meta liftoff.WorktreeMeta
		}
		var rows []row
		for n, m := range cfg.Worktrees {
			rows = append(rows, row{n, m})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].meta.Slot < rows[j].meta.Slot })
		if len(rows) == 0 {
			fmt.Println(tui.StyleDim.Render("no slots allocated yet — run `kit design` or `kit adopt`."))
			return nil
		}
		fmt.Println(tui.StyleTitle.Render("port-slot allocations"))
		for _, r := range rows {
			ports := liftoff.PortsForSlot(r.meta.Slot)
			fmt.Printf("  slot %d  %s  %s\n",
				r.meta.Slot,
				r.name,
				tui.StyleDim.Render(fmt.Sprintf(":%d :%d :%d :%d", ports.App, ports.Admin, ports.API, ports.AdminBE)),
			)
		}
		return nil
	},
}

var slotsRenumberCmd = &cobra.Command{
	Use:   "renumber",
	Short: "Compact slot gaps left after washing kits",
	Long: "**renumber** reassigns port slots so they're sequential starting at 1, " +
		"closing any gaps left after `kit wash`. Master keeps slot 0.\n\n" +
		"Refuses to run if any kit-managed services are running — stop them first " +
		"with `kit pause --all`.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := liftoff.LoadConfig()
		if err != nil {
			return err
		}
		// Refuse if any worktree has running services — new ports would
		// orphan the existing PIDs.
		for name, meta := range cfg.Worktrees {
			if name == "master" {
				continue
			}
			ports := liftoff.PortsForSlot(meta.Slot)
			if running, _ := liftoff.RunningCount(name, ports); running > 0 {
				return fmt.Errorf("%s has %d running service(s) — run `kit pause --all` first", name, running)
			}
		}

		type row struct {
			name string
			from int
			to   int
		}
		var rows []row
		for n, m := range cfg.Worktrees {
			if n == "master" {
				continue
			}
			rows = append(rows, row{name: n, from: m.Slot})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].from < rows[j].from })
		for i := range rows {
			rows[i].to = i + 1
		}

		// Build a diff to show; bail if no changes.
		changed := 0
		for _, r := range rows {
			if r.from != r.to {
				changed++
			}
		}
		if changed == 0 {
			fmt.Println(tui.StyleOK.Render("slots are already sequential — nothing to do."))
			return nil
		}

		fmt.Println(tui.StyleTitle.Render("renumber plan"))
		for _, r := range rows {
			mark := " "
			if r.from != r.to {
				mark = "→"
			}
			fmt.Printf("  %s  slot %d %s slot %d  %s\n",
				mark, r.from, tui.StyleDim.Render("⇒"), r.to, r.name)
		}
		fmt.Println()

		accept := true
		if err := huh.NewConfirm().
			Title("Reassign slots?").
			Description("Updates config.toml. Services were already stopped — restart with `kit play` after.").
			Affirmative("Yes").
			Negative("Cancel").
			Value(&accept).Run(); err != nil {
			return err
		}
		if !accept {
			return nil
		}

		if err := liftoff.WithConfigLock(func(c *liftoff.Config) error {
			for _, r := range rows {
				m := c.Worktrees[r.name]
				m.Slot = r.to
				c.Worktrees[r.name] = m
			}
			return nil
		}); err != nil {
			return err
		}
		fmt.Println(tui.StyleOK.Render(fmt.Sprintf("✓ renumbered %d slot(s)", changed)))
		return nil
	},
}

func init() {
	slotsCmd.AddCommand(slotsRenumberCmd)
	rootCmd.AddCommand(slotsCmd)
}
