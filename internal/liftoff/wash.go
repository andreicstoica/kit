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
		hadError := false
		steps := []step{
			{
				title: "stop running services",
				run: func(emit func(string)) error {
					if err := markCleanupPending(p); err != nil {
						return err
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
					for _, svc := range alive {
						svc := svc
						wg.Add(1)
						go func() {
							defer wg.Done()
							err := StopService(p.Name, svc)
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
					if !HerdrAvailable() {
						if hasSavedHerdrWorkspace(p.Name) {
							return fmt.Errorf("Herdr is not installed; saved workspace still exists")
						}
						emit("Herdr not installed; nothing to remove")
						return nil
					}
					return CloseHerdr(p.Name, p.WorktreePath)
				},
			},
			{
				title: "remove worktree " + p.WorktreePath,
				run: func(emit func(string)) error {
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
					return DropDB(dbName, emit)
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
					if hadError {
						return errors.New("cleanup incomplete; keeping config for retry")
					}
					return WithConfigLock(func(c *Config) error {
						c.FreeSlot(p.Name)
						return nil
					})
				},
			},
		}
		for i, s := range steps {
			if s.skip {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepSkipped}
				continue
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning}
			start := time.Now()
			emit := func(line string) {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning, Line: line}
			}
			err := s.run(emit)
			if err != nil {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepFailed, Err: fmt.Errorf("%w", err), Elapsed: time.Since(start)}
				hadError = true
				// Service and run-state failures are safety stops. The worktree
				// cannot be removed safely while a service may still be alive.
				if i == 0 || i == 1 || i == 3 {
					return
				}
				continue
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepDone, Elapsed: time.Since(start)}
		}
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
		meta, ok := c.Worktrees[p.Name]
		if !ok {
			return nil
		}
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

func hasSavedHerdrWorkspace(name string) bool {
	c, err := LoadConfig()
	if err != nil {
		return false
	}
	return c.Worktrees[name].HerdrID != ""
}
