package cmd

import (
	"fmt"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
)

var linksCmd = &cobra.Command{
	Use:     "links [name]",
	Aliases: []string{"ports", "urls"},
	Short:   "Show the URLs for a worktree",
	Long: "**links** prints the localhost URLs assigned to a worktree's port " +
		"slot. Pass a name, or run from inside a worktree, or pick one from " +
		"the list. Useful for pasting into Slack/Linear/notes without " +
		"recomputing `3000 + slot*10`.",
	Args: cobra.MaximumNArgs(1),
	RunE: runLinks,
}

func init() {
	rootCmd.AddCommand(linksCmd)
}

func runLinks(cmd *cobra.Command, args []string) error {
	layout := liftoff.DefaultLayout()
	name, err := resolveTarget(layout, args, "kit links — pick a kit", false)
	if err != nil {
		return err
	}
	if name == "" {
		return nil
	}

	slot := 0
	if name != "master" {
		st, _ := liftoff.LoadState()
		if st != nil {
			if meta, ok := st.Worktrees[name]; ok {
				slot = meta.Slot
			}
		}
		if slot == 0 {
			fmt.Println(tui.StyleWarn.Render(fmt.Sprintf("%s has no port slot — run `kit design` flow or `kit play` to allocate one.", name)))
			return nil
		}
	}

	ports := liftoff.PortsForSlot(slot)

	emoji := liftoff.EmojiFor(name)
	if name == "master" {
		emoji = "🏠"
	}
	header := emoji + " " + name
	if slot > 0 {
		header += "  " + tui.StyleDim.Render(fmt.Sprintf("slot %d", slot))
	} else {
		header += "  " + tui.StyleDim.Render("slot 0 · master")
	}
	fmt.Println(tui.StyleHi.Render(header))

	rows := []struct {
		Svc  string
		URL  string
		Port int
	}{
		{"app", urlFor(ports.App, ""), ports.App},
		{"admin", urlFor(ports.Admin, ""), ports.Admin},
		{"api", urlFor(ports.API, "/api"), ports.API},
		{"admin_be", urlFor(ports.AdminBE, "/api"), ports.AdminBE},
		{"mcp", urlFor(ports.MCP, ""), ports.MCP},
	}

	cellHdr := lipgloss.NewStyle().Bold(true).Foreground(tui.ColorAccent).Padding(0, 1)
	cellSvc := lipgloss.NewStyle().Padding(0, 1).Foreground(tui.ColorMuted)
	cellURLLive := lipgloss.NewStyle().Padding(0, 1).Foreground(tui.ColorOK).Bold(true)
	cellURLDead := lipgloss.NewStyle().Padding(0, 1).Foreground(tui.ColorDim)
	cellState := lipgloss.NewStyle().Padding(0, 1).Foreground(tui.ColorDim)

	tbl := table.New().
		Border(lipgloss.HiddenBorder()).
		BorderRow(false).
		BorderColumn(false).
		StyleFunc(func(r, c int) lipgloss.Style {
			if r == table.HeaderRow {
				return cellHdr
			}
			row := rows[r]
			live := liftoff.PortListening(row.Port)
			switch c {
			case 0:
				return cellSvc
			case 1:
				if live {
					return cellURLLive
				}
				return cellURLDead
			case 2:
				return cellState
			}
			return cellSvc
		}).
		Headers("SERVICE", "URL", "STATE")

	for _, r := range rows {
		state := "stopped"
		if liftoff.PortListening(r.Port) {
			state = "● running"
		}
		tbl.Row(r.Svc, r.URL, state)
	}

	fmt.Println(tbl.Render())
	fmt.Println(tui.StyleDim.Render("tip: " + strings.Join([]string{
		"kit play " + name,
		"kit pause " + name,
		"kit swap " + name,
	}, " · ")))
	return nil
}

func urlFor(port int, suffix string) string {
	return fmt.Sprintf("http://localhost:%d%s", port, suffix)
}
