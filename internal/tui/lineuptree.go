package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
)

// RenderLineupTree prints the same lineup data as RenderLineup but laid
// out as a tree rooted at master, with each worktree as a child and
// (when running) services as grandchildren.
//
// Toggled via `kit lineup --tree`.
func RenderLineupTree(layout liftoff.Layout) (string, error) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return "", err
	}
	state, _ := liftoff.LoadState()
	if state == nil {
		state = &liftoff.State{Worktrees: map[string]liftoff.WorktreeMeta{}}
	}

	type wt struct {
		name     string
		slot     int
		running  int
		total    int
		branch   string
		dirty    bool
		ahead    int
		behind   int
		legacy   bool
		emoji    string
		sortKey  int64
		services []serviceRow
	}
	var rows []wt
	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		name := w.Name()
		meta := state.Worktrees[name]
		ports := liftoff.PortsForSlot(meta.Slot)
		running := 0
		var svcRows []serviceRow
		for _, svc := range liftoff.DefaultServices {
			s := liftoff.StatusOf(name, svc, ports)
			svcRows = append(svcRows, serviceRow{name: string(svc), alive: s.Alive})
			if s.Alive {
				running++
			}
		}

		ahead, behind := layout.AheadBehind(w.Path)
		row := wt{
			name:    name,
			slot:    meta.Slot,
			running: running,
			total:   len(liftoff.DefaultServices),
			branch:  w.Branch,
			dirty:   liftoff.IsDirty(w.Path),
			ahead:   ahead,
			behind:  behind,
			legacy:  w.HasLegacyPrefix(),
			emoji:   liftoff.EmojiFor(name),
		}
		if !meta.LastUsed.IsZero() {
			row.sortKey = meta.LastUsed.Unix()
		}
		if running > 0 {
			row.services = svcRows
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].sortKey > rows[j].sortKey })

	if len(rows) == 0 {
		return StyleDim.Render("no kits available. start one with `kit design`.") + "\n", nil
	}

	rootLabel := StyleHi.Render("🏠 master") + " " + StyleDim.Render(layout.Master)
	t := tree.Root(rootLabel).
		EnumeratorStyle(lipgloss.NewStyle().Foreground(colorDim)).
		RootStyle(lipgloss.NewStyle()).
		ItemStyle(lipgloss.NewStyle())

	for _, r := range rows {
		label := wtTreeLabel(r.emoji, r.name, r.slot, r.running, r.total, r.branch, r.dirty, r.ahead, r.behind, r.legacy)
		child := tree.Root(label)
		if len(r.services) > 0 {
			for _, s := range r.services {
				child.Child(svcLabel(s))
			}
		}
		t.Child(child)
	}

	var b strings.Builder
	b.WriteString(t.String() + "\n")
	if owner, pid := liftoff.FindCeleryOwner(); owner != "" {
		b.WriteString(StyleDim.Render(fmt.Sprintf("celery: %s (pid %d)", owner, pid)) + "\n")
	}
	return b.String(), nil
}

type serviceRow struct {
	name  string
	alive bool
}

func wtTreeLabel(emoji, name string, slot, running, total int, branch string, dirty bool, ahead, behind int, legacy bool) string {
	header := name
	if emoji != "" {
		header = emoji + " " + name
	}
	parts := []string{lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(header)}

	status := "clean"
	if dirty {
		status = "dirty"
	}
	if ahead > 0 || behind > 0 {
		status = fmt.Sprintf("%s ↑%d↓%d", status, ahead, behind)
	}
	if dirty {
		parts = append(parts, StyleWarn.Render(status))
	} else {
		parts = append(parts, StyleOK.Render(status))
	}

	if slot > 0 {
		parts = append(parts, StyleDim.Render(fmt.Sprintf("slot %d", slot)))
	}
	_ = total // running is shown as child service rows instead of a count
	if branch != name {
		parts = append(parts, StyleDim.Render("on "+branch))
	}
	if legacy {
		parts = append(parts, StyleDim.Render("(legacy)"))
	}
	return strings.Join(parts, "  ")
}

func svcLabel(s serviceRow) string {
	if s.alive {
		return StyleOK.Render("● ") + lipgloss.NewStyle().Foreground(colorMuted).Render(s.name)
	}
	return StyleDim.Render("○ " + s.name)
}
