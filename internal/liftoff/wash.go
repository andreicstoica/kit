package liftoff

import (
	"fmt"
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

// RunWash executes removal: stop services → Herdr → worktree → branch → DB →
// gtab → free slot. A worktree wash is an explicit deletion, so its paired
// Herdr workspace is removed too. Worktree failures are fatal; cleanup of
// terminal/editor state is best-effort so a stale client cannot strand the
// Git worktree.
func (l Layout) RunWash(p WashPlan) <-chan StepUpdate {
	ch := make(chan StepUpdate, 32)
	go func() {
		defer close(ch)
		dbName := DBName(p.Name)
		steps := []step{
			{
				title: "stop running services",
				run: func(emit func(string)) error {
					st, _ := LoadState()
					var slot int
					if st != nil {
						if meta, ok := st.Worktrees[p.Name]; ok {
							slot = meta.Slot
						}
					}
					ports := PortsForSlot(slot)
					stopped := 0
					for _, svc := range AllServices {
						s := StatusOf(p.Name, svc, ports)
						if s.Alive {
							_ = StopService(p.Name, svc)
							stopped++
							emit("stopped " + svc.Label())
						}
					}
					if stopped == 0 {
						emit("nothing running")
					}
					return nil
				},
			},
			{
				title: "remove Herdr workspace",
				run: func(emit func(string)) error {
					if !HerdrAvailable() {
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
				// Only worktree-remove (step 2) is fatal; the rest are
				// best-effort so a late failure still frees the slot (step 6).
				if i == 2 {
					return
				}
				continue
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepDone, Elapsed: time.Since(start)}
		}
	}()
	return ch
}
