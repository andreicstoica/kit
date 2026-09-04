package liftoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// OrphanCandidate is a config record with no matching Git worktree. The
// branch is intentionally retained during reconciliation because the record
// may have been removed outside Kit while its local work is still valuable.
type OrphanCandidate struct {
	Name           string
	Path           string
	Branch         string
	Slot           int
	CleanupPending bool
	HasDB          bool
	HasGtab        bool
	HasRunDir      bool
	HasHerdr       bool
}

// Resources returns the durable resources that reconciliation can remove.
func (o OrphanCandidate) Resources() []string {
	resources := []string{"config"}
	if o.HasDB {
		resources = append(resources, "db")
	}
	if o.HasGtab {
		resources = append(resources, "gtab")
	}
	if o.HasRunDir {
		resources = append(resources, "run state")
	}
	if o.HasHerdr {
		resources = append(resources, "Herdr")
	}
	return resources
}

// FindOrphanedWorktrees returns config records that no longer have a live Git
// worktree. A moved checkout is not an orphan when its path or recorded branch
// still appears in Git's worktree list.
func (l Layout) FindOrphanedWorktrees() ([]OrphanCandidate, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	wts, err := l.ListWorktrees()
	if err != nil {
		return nil, err
	}
	liveNames := map[string]bool{}
	liveBranches := map[string]bool{}
	livePaths := map[string]bool{}
	for _, wt := range wts {
		if wt.IsMaster(l) || wt.Bare {
			continue
		}
		liveNames[wt.Name()] = true
		if wt.Branch != "" {
			liveBranches[wt.Branch] = true
		}
		livePaths[filepath.Clean(wt.Path)] = true
	}

	hasPostgres := HasPostgres()
	out := make([]OrphanCandidate, 0)
	for name, meta := range cfg.Worktrees {
		if name == "master" || liveNames[name] ||
			(meta.Branch != "" && liveBranches[meta.Branch]) ||
			(meta.Path != "" && livePaths[filepath.Clean(meta.Path)]) {
			continue
		}
		_, runErr := os.Stat(RunDirPath(name))
		out = append(out, OrphanCandidate{
			Name:           name,
			Path:           meta.Path,
			Branch:         meta.Branch,
			Slot:           meta.Slot,
			CleanupPending: meta.CleanupPending,
			HasDB:          hasPostgres && HasDB(name),
			HasGtab:        l.HasGtab(name),
			HasRunDir:      runErr == nil,
			HasHerdr:       meta.HerdrID != "",
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ReconcileOrphan removes resources for one orphaned config record. It does
// not delete the local branch, which avoids turning an externally removed
// checkout into permanent loss of unmerged work.
func (l Layout) ReconcileOrphan(candidate OrphanCandidate, onLine LineFn) error {
	current, err := l.FindOrphanedWorktrees()
	if err != nil {
		return err
	}
	found := false
	for _, item := range current {
		if item.Name == candidate.Name {
			found = true
			candidate = item
			break
		}
	}
	if !found {
		return fmt.Errorf("worktree %q exists or config record was already removed", candidate.Name)
	}

	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	meta := cfg.Worktrees[candidate.Name]
	ports := PortsForSlot(meta.Slot)
	for _, svc := range AllServices {
		if !StatusOf(candidate.Name, svc, ports).Alive {
			continue
		}
		if err := StopService(candidate.Name, svc); err != nil {
			return fmt.Errorf("stop %s: %w", svc.Label(), err)
		}
		if onLine != nil {
			onLine("stopped " + svc.Label())
		}
	}

	var errs []error
	if candidate.HasHerdr {
		if !HerdrAvailable() {
			errs = append(errs, errors.New("Herdr is not installed; saved workspace still exists"))
		} else if err := CloseHerdr(candidate.Name, candidate.Path); err != nil {
			errs = append(errs, fmt.Errorf("remove Herdr workspace: %w", err))
		}
	}
	if err := RemoveRunDir(candidate.Name); err != nil {
		errs = append(errs, err)
	}
	if candidate.HasDB {
		if err := DropDB(DBName(candidate.Name), onLine); err != nil {
			errs = append(errs, fmt.Errorf("drop database: %w", err))
		}
	}
	if candidate.HasGtab {
		if err := l.RemoveGtab(candidate.Name); err != nil {
			errs = append(errs, fmt.Errorf("remove gtab workspace: %w", err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	return WithConfigLock(func(c *Config) error {
		c.FreeSlot(candidate.Name)
		return nil
	})
}
