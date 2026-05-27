package liftoff

import (
	"fmt"
	"os"
	"time"
)

// AdoptOptions controls the optional side effects of Adopt. Slot allocation
// + metadata write always happen; the rest are opt-in.
type AdoptOptions struct {
	SymlinkNodeModules bool
	WriteGtab          bool
	GraphiteTrack      bool
}

// AdoptResult is what Adopt did. Returned for the caller to print.
type AdoptResult struct {
	Name            string
	Branch          string
	Path            string
	Slot            int
	Symlinked       bool
	GtabPath        string // "" if not written
	GraphiteTracked bool
}

// AdoptCandidate describes a worktree present on disk + in git but not yet
// in config. Returned by FindAdoptCandidates.
type AdoptCandidate struct {
	Name   string // kit-derived name (basename, stripped of liftoff- prefix)
	Branch string
	Path   string
}

// FindAdoptCandidates returns every git worktree (excluding bare) that
// doesn't already have an entry in config.Worktrees. Master is included
// as "master" so its branch + path + last_used get tracked alongside
// features (its slot stays 0 — no port allocation).
func (l Layout) FindAdoptCandidates(c *Config) ([]AdoptCandidate, error) {
	wts, err := l.ListWorktrees()
	if err != nil {
		return nil, err
	}
	var out []AdoptCandidate
	for _, w := range wts {
		if w.Bare {
			continue
		}
		name := w.Name()
		if w.IsMaster(l) {
			name = "master"
		}
		if _, ok := c.Worktrees[name]; ok {
			continue
		}
		out = append(out, AdoptCandidate{
			Name:   name,
			Branch: w.Branch,
			Path:   w.Path,
		})
	}
	return out, nil
}

// Adopt registers an existing worktree with kit: allocates a port slot,
// persists branch/path/adopted metadata, and runs the requested side
// effects.
//
// Idempotent in the basic case — calling adopt twice keeps the same slot
// (matches AllocateSlot semantics).
func (l Layout) Adopt(name, branch, worktreePath string, opts AdoptOptions, onLine LineFn) (*AdoptResult, error) {
	if _, err := os.Stat(worktreePath); err != nil {
		return nil, fmt.Errorf("worktree path missing: %s", worktreePath)
	}
	// Master gets slot 0 by convention — no allocation, just metadata.
	// kit play / pause / links use slot 0 for master automatically.
	slot := 0
	if err := WithConfigLock(func(c *Config) error {
		if name != "master" {
			s, err := c.AllocateSlot(name, PortsBindable)
			if err != nil {
				return fmt.Errorf("allocate slot: %w", err)
			}
			slot = s
		}
		meta := c.Worktrees[name]
		meta.Slot = slot
		meta.Branch = branch
		meta.Path = worktreePath
		meta.Adopted = true
		if meta.Created.IsZero() {
			meta.Created = time.Now().UTC()
		}
		meta.LastUsed = time.Now().UTC()
		c.Worktrees[name] = meta
		return nil
	}); err != nil {
		return nil, err
	}

	res := &AdoptResult{
		Name:   name,
		Branch: branch,
		Path:   worktreePath,
		Slot:   slot,
	}

	// Side effects don't apply to master — it's not a feature worktree.
	if opts.SymlinkNodeModules && name != "master" {
		if _, err := LinkNodeModules(l.Master, worktreePath, onLine); err != nil {
			if onLine != nil {
				onLine("symlink failed: " + err.Error())
			}
		} else {
			res.Symlinked = true
		}
	}

	if opts.WriteGtab && name != "master" {
		if path, err := l.WriteGtab(name, worktreePath); err != nil {
			if onLine != nil {
				onLine("gtab write failed: " + err.Error())
			}
		} else {
			res.GtabPath = path
		}
	}

	if opts.GraphiteTrack && name != "master" && HasGraphite() && branch != "" && branch != l.MainBranch {
		if err := l.GtTrack(worktreePath, onLine); err != nil {
			if onLine != nil {
				onLine("gt track failed: " + err.Error())
			}
		} else {
			res.GraphiteTracked = true
		}
	}

	return res, nil
}
