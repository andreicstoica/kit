package liftoff

import (
	"fmt"
	"time"
)

// WashPlan captures choices for a `kit wash` run.
type WashPlan struct {
	Name         string
	WorktreePath string // resolved (could be clean ~/liftoff/<name> or legacy ~/liftoff/liftoff-<name>)
	DropDB       bool
	RemoveGtab   bool
}

// RunWash executes removal: stop services → worktree → branch → DB → gtab → free slot.
// Emits StepUpdate events. Worktree+branch failures are fatal; the rest are best-effort.
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
				title: "remove worktree " + p.WorktreePath,
				run: func(emit func(string)) error {
					return l.RemoveWorktree(p.WorktreePath, emit)
				},
			},
			{
				title: "delete branch " + p.Name,
				run: func(emit func(string)) error {
					return l.DeleteBranch(p.Name, emit)
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
					st, err := LoadState()
					if err != nil {
						return err
					}
					st.FreeSlot(p.Name)
					return st.Save()
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
				// Steps: 0 stop, 1 worktree, 2 branch, 3 db, 4 gtab, 5 slot.
				// Fatal: worktree-remove + branch-delete (1, 2). Best-effort: rest.
				if i == 1 || i == 2 {
					return
				}
				continue
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepDone, Elapsed: time.Since(start)}
		}
	}()
	return ch
}
