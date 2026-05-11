package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// Column styles
var (
	colHeader = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Padding(0, 1)
	colCell   = lipgloss.NewStyle().Padding(0, 1)
	colDim    = lipgloss.NewStyle().Foreground(colorDim).Padding(0, 1)
)

// RenderLineup prints a static (non-interactive) table of active worktrees.
// Used by `kit lineup` / `kit ls`.
func RenderLineup(layout liftoff.Layout) (string, error) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return "", err
	}
	state, _ := liftoff.LoadState()
	if state == nil {
		state = &liftoff.State{Worktrees: map[string]liftoff.WorktreeMeta{}}
	}

	type row struct {
		name       string
		slot       string
		hasSlot    bool
		running    string
		hasRunning bool
		branch     string
		status     string
		statusOK   bool
		sortKey    int64
	}
	var rows []row

	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		name := w.Name()
		stRaw := "clean"
		if liftoff.IsDirty(w.Path) {
			stRaw = "dirty"
		}
		ahead, behind := layout.AheadBehind(w.Path)
		if ahead > 0 || behind > 0 {
			stRaw = fmt.Sprintf("%s ↑%d↓%d", stRaw, ahead, behind)
		}

		meta, hasMeta := state.Worktrees[name]
		ports := liftoff.PortsForSlot(meta.Slot)

		// Count running services (out of 6 default services).
		running := 0
		total := len(liftoff.DefaultServices)
		for _, svc := range liftoff.DefaultServices {
			if liftoff.StatusOf(name, svc, ports).Alive {
				running++
			}
		}
		runningStr := "—"
		hasRunning := running > 0
		if hasRunning {
			runningStr = fmt.Sprintf("%d/%d", running, total)
		}

		var sortKey int64
		if hasMeta && !meta.LastUsed.IsZero() {
			sortKey = meta.LastUsed.Unix()
		}

		nameDisp := name
		if e := liftoff.EmojiFor(name); e != "" {
			nameDisp = e + " " + nameDisp
		}
		if w.HasLegacyPrefix() {
			nameDisp = nameDisp + " " + StyleDim.Render("(legacy)")
		}

		slotDisp := "—"
		if hasMeta && meta.Slot > 0 {
			slotDisp = fmt.Sprintf("%d", meta.Slot)
		}

		branchDisp := w.Branch
		if len(branchDisp) > 32 {
			branchDisp = branchDisp[:31] + "…"
		}

		rows = append(rows, row{
			name:       nameDisp,
			slot:       slotDisp,
			hasSlot:    hasMeta && meta.Slot > 0,
			running:    runningStr,
			hasRunning: hasRunning,
			branch:     branchDisp,
			status:     stRaw,
			statusOK:   !strings.Contains(stRaw, "dirty"),
			sortKey:    sortKey,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].sortKey > rows[j].sortKey
	})

	var b strings.Builder

	if len(rows) == 0 {
		b.WriteString(StyleDim.Render("no kits available. start one with `kit design`.") + "\n")
		return b.String(), nil
	}

	tbl := table.New().
		Border(lipgloss.HiddenBorder()).
		BorderRow(false).
		BorderColumn(false).
		StyleFunc(func(r, c int) lipgloss.Style {
			if r == table.HeaderRow {
				return colHeader
			}
			data := rows[r]
			switch c {
			case 0: // NAME
				return colCell
			case 1: // SLOT
				if !data.hasSlot {
					return colDim
				}
				return colCell.Foreground(colorAccent)
			case 2: // RUNNING
				if !data.hasRunning {
					return colDim
				}
				return colCell.Foreground(colorOK).Bold(true)
			case 3: // BRANCH
				return colCell.Foreground(colorMuted)
			case 4: // STATUS
				if data.statusOK {
					return colCell.Foreground(colorOK)
				}
				return colCell.Foreground(colorWarn)
			}
			return colCell
		}).
		Headers("NAME", "SLOT", "RUNNING", "BRANCH", "STATUS")

	for _, r := range rows {
		tbl.Row(r.name, r.slot, r.running, r.branch, r.status)
	}

	b.WriteString(tbl.Render() + "\n")
	b.WriteString(StyleDim.Render(fmt.Sprintf("master: %s", layout.Master)) + "\n")
	if owner, pid := liftoff.FindCeleryOwner(); owner != "" {
		b.WriteString(StyleDim.Render(fmt.Sprintf("celery: %s (pid %d)", owner, pid)) + "\n")
	}
	return b.String(), nil
}

