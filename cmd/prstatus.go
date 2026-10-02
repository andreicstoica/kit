package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func printRemotePRs(out io.Writer, layout liftoff.Layout) error {
	wts, err := layout.ListWorktrees()
	if err != nil {
		return err
	}
	cfg, err := liftoff.LoadConfig()
	if err != nil {
		return err
	}
	type row struct {
		name, branch, checkout string
	}
	var rows []row
	var branches []string
	seenNames := map[string]bool{}
	seenBranches := map[string]bool{}
	seenPaths := map[string]bool{}
	for _, wt := range wts {
		if wt.IsMaster(layout) || wt.Bare {
			continue
		}
		checkout := "present"
		if wt.Missing {
			checkout = "missing"
		}
		if wt.Locked {
			checkout += " (locked)"
		}
		rows = append(rows, row{wt.Name(), wt.Branch, checkout})
		branches = append(branches, wt.Branch)
		seenNames[wt.Name()] = true
		seenBranches[wt.Branch] = true
		seenPaths[wt.Path] = true
	}
	for name, meta := range cfg.Worktrees {
		if name == "master" || seenNames[name] || (meta.Branch != "" && seenBranches[meta.Branch]) || seenPaths[meta.Path] {
			continue
		}
		checkout := "missing"
		if meta.Path != "" {
			if _, err := os.Stat(meta.Path); err == nil {
				checkout = "unregistered"
			} else if !os.IsNotExist(err) {
				checkout = "unknown"
			}
		}
		rows = append(rows, row{name, meta.Branch, checkout})
		branches = append(branches, meta.Branch)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	statuses, lookupErr := layout.RemotePRStatuses(branches)
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "\nNAME\tBRANCH\tCHECKOUT\tPR\tURL")
	for _, row := range rows {
		pr := statuses[row.branch]
		label := pr.State
		if row.branch == "" {
			label = "no branch"
		}
		if pr.Number > 0 {
			label = fmt.Sprintf("#%d %s", pr.Number, label)
			if pr.Draft {
				label += " (draft)"
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", row.name, row.branch, row.checkout, label, pr.URL)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return lookupErr
}
