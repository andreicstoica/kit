package tui

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
)

// wtNode is one worktree row in the tree. Children are populated from the
// graphite parent relationships when `gt` is available.
type wtNode struct {
	name         string
	slot         int
	running      int
	total        int
	dirty        bool
	ahead        int
	behind       int
	legacy       bool
	emoji        string
	sortKey      int64
	services     []serviceRow
	branch       string
	gtParent     string                // graphite-tracked parent branch name, "" if untracked
	needsRestack bool                  // parent has moved forward; branch should `gt restack`
	gtStack      []liftoff.StackEntry  // full stack (trunk → ... → self), empty if untracked
	children     []*wtNode
}

// RenderLineupTree prints worktrees as a tree.
//
// When `gt` is installed, the tree reflects the graphite stack hierarchy:
// each worktree shows up under its parent branch. Untracked worktrees and
// branches whose parent is master (or the configured main branch) land
// directly under the master root.
//
// When `gt` isn't installed, every worktree is a direct child of master
// (flat tree). Toggled via `kit lineup --tree`.
func RenderLineupTree(layout liftoff.Layout) (string, error) {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return "", err
	}
	state, _ := liftoff.LoadState()
	if state == nil {
		state = &liftoff.State{Worktrees: map[string]liftoff.WorktreeMeta{}}
	}

	type wtIn struct {
		w liftoff.Worktree
	}
	var inputs []wtIn
	for _, w := range wts {
		if w.IsMaster(layout) || w.Bare {
			continue
		}
		inputs = append(inputs, wtIn{w})
	}
	if len(inputs) == 0 {
		return StyleDim.Render("no kits available. start one with `kit design`.") + "\n", nil
	}

	// Build a node per worktree, populating service status + git stats. Run
	// graphite parent queries in parallel (each is a small subprocess).
	nodes := make([]*wtNode, len(inputs))
	parents := make([]string, len(inputs))
	hasGt := liftoff.HasGraphite()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, in := range inputs {
		i, in := i, in
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			name := in.w.Name()
			meta := state.Worktrees[name]
			ports := liftoff.PortsForSlot(meta.Slot)
			running, total := liftoff.RunningCount(name, ports)
			var svcRows []serviceRow
			for _, svc := range liftoff.DisplayServices {
				svcRows = append(svcRows, serviceRow{
					name:  svc.Label(),
					port:  liftoff.ServicePort(svc, ports),
					alive: liftoff.IsServiceAlive(name, svc, ports),
				})
			}
			ahead, behind := layout.AheadBehind(in.w.Path)
			n := &wtNode{
				name:    name,
				slot:    meta.Slot,
				running: running,
				total:   total,
				dirty:   liftoff.IsDirty(in.w.Path),
				ahead:   ahead,
				behind:  behind,
				legacy:  in.w.HasLegacyPrefix(),
				emoji:   liftoff.EmojiFor(name),
				branch:  in.w.Branch,
			}
			if !meta.LastUsed.IsZero() {
				n.sortKey = meta.LastUsed.Unix()
			}
			if running > 0 {
				n.services = svcRows
			}
			if hasGt {
				n.gtParent = layout.GtParentOf(in.w.Path)
				if n.gtParent != "" {
					n.needsRestack = layout.NeedsRestack(in.w.Path, n.gtParent)
					n.gtStack = layout.GtStackOf(in.w.Path)
				}
			}
			nodes[i] = n
			parents[i] = n.gtParent
		}()
	}
	wg.Wait()

	// Index by branch name so children can find their parent worktrees.
	byBranch := map[string]*wtNode{}
	for _, n := range nodes {
		byBranch[n.branch] = n
	}

	mainBranch := layout.MainBranch
	if mainBranch == "" {
		mainBranch = "master"
	}

	var roots []*wtNode
	for _, n := range nodes {
		// Anything whose parent is master (or empty / not a tracked branch we know
		// about) goes to the top-level "under master" group.
		if n.gtParent == "" || n.gtParent == mainBranch || n.gtParent == "main" {
			roots = append(roots, n)
			continue
		}
		parent, ok := byBranch[n.gtParent]
		if !ok {
			// Parent branch isn't one of our worktrees — surface under master and
			// note the dangling parent in the label.
			roots = append(roots, n)
			continue
		}
		parent.children = append(parent.children, n)
	}

	sortNodes := func(s []*wtNode) {
		sort.Slice(s, func(i, j int) bool { return s[i].sortKey > s[j].sortKey })
	}
	sortNodes(roots)
	for _, n := range nodes {
		sortNodes(n.children)
	}

	rootLabel := StyleDim.Render("🚀 master  " + layout.Master)
	t := tree.Root(rootLabel).
		EnumeratorStyle(lipgloss.NewStyle().Foreground(colorDim)).
		RootStyle(lipgloss.NewStyle()).
		ItemStyle(lipgloss.NewStyle())

	// Pre-compute stack size for each node so labels can render it without
	// re-walking the graph during render.
	sizeByName := map[string]int{}
	for _, n := range nodes {
		sizeByName[n.name] = stackSizeFor(n, byBranch)
	}

	var attach func(parent *tree.Tree, n *wtNode)
	attach = func(parent *tree.Tree, n *wtNode) {
		label := wtTreeLabel(n, sizeByName[n.name])
		child := tree.Root(label)
		for _, entry := range stackChildLabels(n.gtStack, mainBranch) {
			child.Child(entry)
		}
		// Services nest under their own "services" subnode so the
		// enumerator clearly separates them from gt branch rows.
		if len(n.services) > 0 {
			svcGroup := tree.Root(StyleDim.Render("services"))
			for _, s := range n.services {
				svcGroup.Child(svcLabel(s))
			}
			child.Child(svcGroup)
		}
		for _, gc := range n.children {
			attach(child, gc)
		}
		parent.Child(child)
	}
	for _, r := range roots {
		attach(t, r)
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
	port  int
	alive bool
}

func wtTreeLabel(n *wtNode, stackSize int) string {
	header := n.name
	if n.emoji != "" {
		header = n.emoji + " " + n.name
	}
	parts := []string{lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(header)}

	status := "clean"
	if n.dirty {
		status = "dirty"
	}
	if n.ahead > 0 || n.behind > 0 {
		status = fmt.Sprintf("%s ↑%d↓%d", status, n.ahead, n.behind)
	}
	if n.dirty {
		parts = append(parts, StyleWarn.Render(status))
	} else {
		parts = append(parts, StyleOK.Render(status))
	}

	if n.slot > 0 {
		parts = append(parts, StyleDim.Render(fmt.Sprintf("slot %d", n.slot)))
	}
	if stackSize >= 2 {
		parts = append(parts, StyleDim.Render(fmt.Sprintf("stack %d", stackSize)))
	}
	if n.legacy {
		parts = append(parts, StyleDim.Render("(legacy)"))
	}
	return strings.Join(parts, "  ")
}

// stackSizeFor returns the total number of worktree nodes in n's connected
// graphite-component (ancestors via gtParent chain + n itself + descendants).
// Worktrees not tracked in graphite count as a stack of 1.
func stackSizeFor(n *wtNode, byBranch map[string]*wtNode) int {
	count := 1
	// Walk up the parent chain.
	cur := n
	for {
		if cur.gtParent == "" {
			break
		}
		parent, ok := byBranch[cur.gtParent]
		if !ok {
			break
		}
		count++
		cur = parent
	}
	// Walk down all descendants.
	var descend func(x *wtNode)
	descend = func(x *wtNode) {
		for _, c := range x.children {
			count++
			descend(c)
		}
	}
	descend(n)
	return count
}

// svcLabel renders a service row distinctly from gt branch rows.
// Branches use ◯/◉ circles; services use ▶/▷ triangles so they don't
// blend together visually. Port number sits dim after the service name.
func svcLabel(s serviceRow) string {
	glyph := "▷"
	style := StyleDim
	if s.alive {
		glyph = "▶"
		style = StyleOK
	}
	name := lipgloss.NewStyle().Foreground(colorMuted).Render(pad(s.name, 11))
	if !s.alive {
		name = StyleDim.Render(pad(s.name, 11))
	}
	out := style.Render(glyph+" ") + name
	if s.port > 0 {
		out += "  " + StyleDim.Render(fmt.Sprintf(":%d", s.port))
	}
	return out
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

// stackChildLabels renders the gt stack as one tree child per branch.
// Returns nil for stacks of <2 (boring: just trunk + self).
//
// Styling rules:
//   - trunk (master/main): dim grey — structural
//   - current branch: accent + bold
//   - other branches: accent, not bold
//
// "(needs restack)" / "(liftoff-X)" hints stay inline.
func stackChildLabels(stack []liftoff.StackEntry, trunk string) []string {
	if len(stack) < 2 {
		return nil
	}
	branchStyle := lipgloss.NewStyle().Foreground(colorAccent)
	out := make([]string, len(stack))
	for i, e := range stack {
		glyph := e.Glyph
		if glyph == "" {
			glyph = "◯"
		}
		body := glyph + " " + e.Branch
		var line string
		switch {
		case e.Branch == trunk || e.Branch == "master" || e.Branch == "main":
			line = StyleDim.Render(body)
		case e.Current:
			line = lipgloss.NewStyle().Foreground(ColorLiftoff).Render(body)
		default:
			line = branchStyle.Render(body)
		}
		if e.Hint != "" {
			styled := StyleDim.Render(" " + e.Hint)
			if strings.Contains(e.Hint, "needs restack") {
				styled = StyleWarn.Render(" " + e.Hint)
			}
			line += styled
		}
		out[i] = line
	}
	return out
}
