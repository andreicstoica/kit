package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/lipgloss"
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
		name     string
		emoji    string
		branch   string
		status   string
		slot     int
		hasSlot  bool
		services string
		db       string
		gtab     string
		isLegacy bool
		lastUsed string
		sortKey  int64 // unix seconds, descending
	}
	var rows []row
	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		name := w.Name()
		st := "clean"
		if liftoff.IsDirty(w.Path) {
			st = "dirty"
		}
		ahead, behind := layout.AheadBehind(w.Path)
		if ahead > 0 || behind > 0 {
			st = fmt.Sprintf("%s ↑%d↓%d", st, ahead, behind)
		}
		db := "—"
		if liftoff.HasPostgres() && liftoff.HasDB(name) {
			db = liftoff.DBName(name)
		}
		gtab := "—"
		if layout.HasGtab(name) {
			gtab = "✓"
		}
		meta, hasMeta := state.Worktrees[name]
		ports := liftoff.PortsForSlot(meta.Slot)
		svcStatus := serviceMiniStatus(name, ports)
		lastUsed := "—"
		var sortKey int64
		if hasMeta && !meta.LastUsed.IsZero() {
			lastUsed = relativeTime(meta.LastUsed)
			sortKey = meta.LastUsed.Unix()
		}
		rows = append(rows, row{
			name:     name,
			emoji:    liftoff.EmojiFor(name),
			branch:   w.Branch,
			status:   st,
			slot:     meta.Slot,
			hasSlot:  hasMeta,
			services: svcStatus,
			db:       db,
			gtab:     gtab,
			isLegacy: w.HasLegacyPrefix(),
			lastUsed: lastUsed,
			sortKey:  sortKey,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].sortKey > rows[j].sortKey
	})

	header := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	var b strings.Builder
	b.WriteString(header.Render(fmt.Sprintf("%-26s %-6s %-26s %-18s %-8s %-12s %s",
		"NAME", "SLOT", "BRANCH", "STATUS", "SERVICES", "LAST USED", "GTAB")) + "\n")

	if len(rows) == 0 {
		b.WriteString(StyleDim.Render("no kits on the field. start one with `kit dress`.") + "\n")
		return b.String(), nil
	}

	for _, r := range rows {
		nameDisp := r.name
		if r.emoji != "" {
			nameDisp = r.emoji + " " + nameDisp
		}
		if r.isLegacy {
			nameDisp += StyleDim.Render(" (legacy)")
		}
		stStyle := StyleOK
		if strings.Contains(r.status, "dirty") {
			stStyle = StyleWarn
		}
		slotStr := "—"
		if r.hasSlot {
			slotStr = fmt.Sprintf("%d", r.slot)
		}
		b.WriteString(fmt.Sprintf("%-26s %-6s %-26s %-18s %-8s %-12s %s\n",
			nameDisp,
			slotStr,
			r.branch,
			stStyle.Render(fmt.Sprintf("%-18s", r.status)),
			r.services,
			r.lastUsed,
			r.gtab,
		))
	}
	b.WriteString("\n")
	b.WriteString(StyleDim.Render(fmt.Sprintf("master: %s", layout.Master)) + "\n")
	if owner, pid := liftoff.FindCeleryOwner(); owner != "" {
		b.WriteString(StyleDim.Render(fmt.Sprintf("celery: %s (pid %d)", owner, pid)) + "\n")
	}
	return b.String(), nil
}

// serviceMiniStatus returns a six-char compact status for app/admin/api/admin_be/celery/beat.
// Glyphs: green dot = running, dim dot = stopped.
func serviceMiniStatus(name string, ports liftoff.Ports) string {
	order := []liftoff.Service{
		liftoff.SvcApp, liftoff.SvcAdmin,
		liftoff.SvcAPI, liftoff.SvcAdminBE,
		liftoff.SvcCelery, liftoff.SvcBeat,
	}
	var b strings.Builder
	for _, svc := range order {
		s := liftoff.StatusOf(name, svc, ports)
		if s.Alive {
			b.WriteString(StyleOK.Render("●"))
		} else {
			b.WriteString(StyleDim.Render("·"))
		}
	}
	return b.String()
}

// relativeTime converts a timestamp to "5m ago" / "3d ago" style.
func relativeTime(t interface{ Unix() int64 }) string {
	now := nowUnix()
	then := t.Unix()
	diff := now - then
	switch {
	case diff < 60:
		return "just now"
	case diff < 3600:
		return fmt.Sprintf("%dm ago", diff/60)
	case diff < 86400:
		return fmt.Sprintf("%dh ago", diff/3600)
	case diff < 604800:
		return fmt.Sprintf("%dd ago", diff/86400)
	default:
		return fmt.Sprintf("%dw ago", diff/604800)
	}
}

func nowUnix() int64 { return nowFn() }

// nowFn is a var so tests can stub it; default uses real time.
var nowFn = func() int64 { return realNow() }
