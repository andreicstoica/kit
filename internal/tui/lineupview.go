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
		if w.Bare {
			continue
		}
		isMaster := w.IsMaster(layout)
		name := w.Name()
		if isMaster {
			name = "master"
		}
		stRaw := "clean"
		if liftoff.IsDirty(w.Path) {
			stRaw = "dirty"
		}
		if !isMaster {
			// Master is the reference; ahead/behind doesn't apply.
			ahead, behind := layout.AheadBehind(w.Path)
			if ahead > 0 || behind > 0 {
				stRaw = fmt.Sprintf("%s ↑%d↓%d", stRaw, ahead, behind)
			}
		}

		meta, hasMeta := state.Worktrees[name]
		ports := liftoff.PortsForSlot(meta.Slot)

		running, total := liftoff.RunningCount(name, ports)
		runningStr := "—"
		hasRunning := running > 0
		if hasRunning {
			runningStr = fmt.Sprintf("%d/%d", running, total)
		}

		var sortKey int64
		if isMaster {
			// Pin master to the top regardless of LastUsed.
			sortKey = 1<<62
		} else if hasMeta && !meta.LastUsed.IsZero() {
			sortKey = meta.LastUsed.Unix()
		}

		emoji := liftoff.EmojiFor(name)
		if isMaster {
			emoji = "🚀"
		}
		nameDisp := name
		if emoji != "" {
			nameDisp = emoji + " " + nameDisp
		}
		if !isMaster && w.HasLegacyPrefix() {
			nameDisp = nameDisp + " " + StyleDim.Render("(legacy)")
		}

		slotDisp := "—"
		switch {
		case isMaster:
			slotDisp = "0"
		case hasMeta && meta.Slot > 0:
			slotDisp = fmt.Sprintf("%d", meta.Slot)
		}

		branchDisp := w.Branch
		if len(branchDisp) > 32 {
			branchDisp = branchDisp[:31] + "…"
		}

		rows = append(rows, row{
			name:       nameDisp,
			slot:       slotDisp,
			hasSlot:    isMaster || (hasMeta && meta.Slot > 0),
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
	if owner, pid := liftoff.FindCeleryOwner(); owner != "" {
		b.WriteString(StyleDim.Render(fmt.Sprintf("celery: %s (pid %d)", owner, pid)) + "\n")
	}
	return b.String(), nil
}

