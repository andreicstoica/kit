package liftoff

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// WashPlan captures choices for a `kit wash` run.
type WashPlan struct {
	Name         string
	Branch       string // actual git branch — may differ from Name (e.g. "acs/foo-cleanup")
	WorktreePath string // resolved (could be clean ~/liftoff/<name> or legacy ~/liftoff/liftoff-<name>)
	DropDB       bool
	RemoveGtab   bool
	ExpectedHead string
	RequireClean bool
}

// branchForDelete returns the actual branch to remove. Falls back to Name
// only when Branch is unset (pre-existing washflow callers, tests).
func branchForDelete(p WashPlan) string {
	if p.Branch != "" {
		return p.Branch
	}
	return p.Name
}

// RunWash executes removal: record intent → stop services → run state → Herdr
// → worktree → branch → DB → gtab → free slot. A worktree wash is an explicit
// deletion, so its paired Herdr workspace is removed too. Failed cleanup keeps
// the config record marked for retry instead of freeing it prematurely.
func (l Layout) RunWash(p WashPlan) <-chan StepUpdate {
	ch := make(chan StepUpdate, 32)
	go func() {
		defer close(ch)
		dbName := DBName(p.Name)
		// washDB/skipDB are resolved before any destructive step runs (see
		// the first step): a disputed database must block the worktree
		// removal, not surface after it. The env proof is read while the
		// checkout still exists.
		var washDB string
		var errMu sync.Mutex
		failed := false
		markFailed := func() { errMu.Lock(); failed = true; errMu.Unlock() }
		var skipDB bool
		steps := []step{
			{
				title: "stop running services",
				run: func(emit func(string)) error {
					if p.ExpectedHead != "" {
						head, err := Run(p.WorktreePath, "git", "rev-parse", "HEAD")
						if err != nil {
							return err
						}
						branch, err := Run(p.WorktreePath, "git", "symbolic-ref", "--short", "HEAD")
						if err != nil {
							return err
						}
						if head != p.ExpectedHead || branch != branchForDelete(p) || (p.RequireClean && IsDirty(p.WorktreePath)) {
							return errors.New("worktree changed since selection; scan again before cleanup")
						}
					}
					if err := markCleanupPending(p); err != nil {
						return err
					}
					if p.DropDB {
						db, skip, err := resolveCleanupDB(p.Name, p.WorktreePath)
						if err != nil {
							return err
						}
						washDB, skipDB = db, skip
					}
					st, _ := LoadState()
					var slot int
					if st != nil {
						if meta, ok := st.Worktrees[p.Name]; ok {
							slot = meta.Slot
						}
					}
					ports := PortsForSlot(slot)
					// Collect alive services first, then stop them in parallel.
					type svcResult struct {
						svc Service
						err error
					}
					var alive []Service
					for _, svc := range AllServices {
						if StatusOf(p.Name, svc, ports).Alive {
							alive = append(alive, svc)
						}
					}
					if len(alive) == 0 {
						emit("nothing running")
						return nil
					}
					var mu sync.Mutex
					var firstErr error
					var wg sync.WaitGroup
					snap := SnapshotProcs(p.Name, alive)
					for _, svc := range alive {
						svc := svc
						wg.Add(1)
						go func() {
							defer wg.Done()
							err := snap.Stop(p.Name, svc)
							mu.Lock()
							defer mu.Unlock()
							if err != nil && firstErr == nil {
								firstErr = err
							}
							emit("stopped " + svc.Label())
						}()
					}
					wg.Wait()
					return firstErr
				},
			},
			{
				title: "remove service run state",
				run: func(emit func(string)) error {
					return RemoveRunDir(p.Name)
				},
			},
			{
				title: "remove Herdr workspace",
				run: func(emit func(string)) error {
					// CloseManagedWorkspaces covers both backends from their
					// persisted mappings, so no backend switch strands a
					// workspace: a stale Herdr space is closed even when Rex
					// is selected, and vice versa.
					if !hasSavedManagedWorkspace(p.Name) {
						emit("no saved workspace; nothing to remove")
						return nil
					}
					if cfg, err := LoadConfig(); err == nil && !HerdrAvailable() {
						if m := cfg.Worktrees[p.Name]; m.HerdrID != "" || m.HerdrSpace != "" {
							emit("Herdr not installed; dropping its saved mapping")
						}
					}
					return CloseManagedWorkspaces(p.Name, p.WorktreePath)
				},
			},
			{
				title: "remove worktree " + p.WorktreePath,
				run: func(emit func(string)) error {
					if err := l.verifyWorktreeOwnership(p.WorktreePath, p.Name); err != nil {
						return err
					}
					return l.RemoveWorktree(p.WorktreePath, emit)
				},
			},
			{
				title: "delete branch " + branchForDelete(p),
				run: func(emit func(string)) error {
					return l.DeleteBranch(branchForDelete(p), emit)
				},
			},
			{
				title: "drop database " + dbName,
				skip:  !p.DropDB,
				run: func(emit func(string)) error {
					if skipDB {
						emit("worktree uses a different database; nothing to drop")
						return nil
					}
					return DropDB(washDB, emit)
				},
			},
			{
				title: "remove gtab workspace",
				skip:  !p.RemoveGtab,
				run: func(emit func(string)) error {
					return l.RemoveGtab(p.Name)
				},
			},
			{
				title: "free port slot",
				run: func(emit func(string)) error {
					errMu.Lock()
					incomplete := failed
					errMu.Unlock()
					if incomplete {
						return errors.New("cleanup incomplete; keeping config for retry")
					}
					return WithConfigLock(func(c *Config) error {
						c.FreeSlot(p.Name)
						return nil
					})
				},
			},
		}
		// runStep reports false when the step failed.
		runStep := func(i int) bool {
			s := steps[i]
			if s.skip {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepSkipped}
				return true
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning}
			start := time.Now()
			err := s.run(func(line string) {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning, Line: line}
			})
			if err != nil {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepFailed, Err: fmt.Errorf("%w", err), Elapsed: time.Since(start)}
				markFailed()
				return false
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepDone, Elapsed: time.Since(start)}
			return true
		}
		// Services, run state and worktree removal are safety stops: a failure
		// there ends the run before any later cleanup, so a worktree that could
		// not be removed keeps its database. Workspace close failing is not a stop.
		for _, i := range []int{0, 1} {
			if !runStep(i) {
				return
			}
		}
		runStep(2)
		if !runStep(3) {
			return
		}
		// Branch, database and gtab cleanups are independent once the
		// worktree is gone.
		var wg sync.WaitGroup
		for _, i := range []int{4, 5, 6} {
			wg.Add(1)
			go func() { defer wg.Done(); runStep(i) }()
		}
		wg.Wait()
		runStep(7)
	}()
	return ch
}

// RunWashBlocking adapts the event stream for non-interactive cleanup flows.
func (l Layout) RunWashBlocking(p WashPlan) error {
	var errs []error
	for update := range l.RunWash(p) {
		if update.Status == StepFailed && update.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", update.Title, update.Err))
		}
	}
	return errors.Join(errs...)
}

func markCleanupPending(p WashPlan) error {
	return WithConfigLock(func(c *Config) error {
		meta := c.Worktrees[p.Name]
		meta.CleanupPending = true
		if meta.Branch == "" {
			meta.Branch = p.Branch
		}
		if meta.Path == "" {
			meta.Path = p.WorktreePath
		}
		c.Worktrees[p.Name] = meta
		return nil
	})
}
