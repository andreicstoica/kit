package liftoff

import (
	"fmt"
	"sync"
	"time"
)

// DressPlan captures every choice for a `kit design` run.
// (The struct name predates the command rename from `dress` → `design`;
// kept as-is to minimize churn in internal call sites.)
type DressPlan struct {
	Name            string
	Worktree        string
	CloneDB         bool
	BackendDeps     bool
	SymlinkFrontend bool // symlink frontend node_modules from master
	GraphiteTrack   bool
	Gtab            bool
	OverwriteEnvs   bool // force overwrite if env files already exist in worktree

	// Result fields populated after RunDress completes successfully.
	AllocatedSlot int
}

// StepStatus is the lifecycle of one step.
type StepStatus int

const (
	StepPending StepStatus = iota
	StepRunning
	StepDone
	StepSkipped
	StepFailed
)

func (s StepStatus) String() string {
	switch s {
	case StepPending:
		return "pending"
	case StepRunning:
		return "running"
	case StepDone:
		return "done"
	case StepSkipped:
		return "skipped"
	case StepFailed:
		return "failed"
	}
	return "?"
}

// StepUpdate is one event from the runner.
type StepUpdate struct {
	Index   int
	Title   string
	Status  StepStatus
	Line    string
	Err     error
	Elapsed time.Duration
	// AllocatedSlot is filled in for the slot-allocation step's StepDone update.
	AllocatedSlot int
}

// step is one executable unit.
type step struct {
	title string
	skip  bool
	run   func(emit func(string)) error
	// extras can be filled by the step itself when emitting the final StepDone.
	extras func() (slot int)
}

// PlanSteps returns the ordered step list, including skipped ones (for display).
func (l Layout) planSteps(p DressPlan, slotResult *int) []step {
	dbName := DBName(p.Name)
	return []step{
		{
			title: "fetch origin/" + l.MainBranch,
			run: func(emit func(string)) error {
				return l.FetchMain(emit)
			},
		},
		{
			title: fmt.Sprintf("worktree add %s -b %s %s", p.Worktree, p.Name, l.MainBranch),
			run: func(emit func(string)) error {
				return l.AddWorktree(p.Name, p.Worktree, emit)
			},
		},
		{
			title: "copy env files (root, backend, frontend/app, frontend/admin)",
			run: func(emit func(string)) error {
				_, _, err := l.CopyEnvFiles(l.Master, p.Worktree, p.OverwriteEnvs, emit)
				return err
			},
		},
		{
			title: "create database " + dbName,
			skip:  !p.CloneDB,
			run: func(emit func(string)) error {
				return CreateDB(dbName, emit)
			},
		},
		{
			title: "clone database liftoff -> " + dbName,
			skip:  !p.CloneDB,
			run: func(emit func(string)) error {
				return CloneDB("liftoff", dbName, emit)
			},
		},
		{
			title: "update backend/.env SQLALCHEMY_DATABASE_NAME=" + dbName,
			skip:  !p.CloneDB,
			run: func(emit func(string)) error {
				return l.UpdateBackendDBName(p.Worktree, dbName)
			},
		},
		{
			title: "uv install backend",
			skip:  !p.BackendDeps,
			run: func(emit func(string)) error {
				return InstallBackend(p.Worktree, emit)
			},
		},
		{
			title: "symlink frontend node_modules from master",
			skip:  !p.SymlinkFrontend,
			run: func(emit func(string)) error {
				_, err := LinkNodeModules(l.Master, p.Worktree, emit)
				return err
			},
		},
		{
			title: "gt track --parent " + l.MainBranch,
			skip:  !p.GraphiteTrack,
			run: func(emit func(string)) error {
				return l.GtTrack(p.Worktree, emit)
			},
		},
		{
			title: "write gtab workspace",
			skip:  !p.Gtab,
			run: func(emit func(string)) error {
				path, err := l.WriteGtab(p.Name, p.Worktree)
				if err != nil {
					return err
				}
				emit("wrote " + path)
				return nil
			},
		},
		{
			title: "allocate port slot",
			run: func(emit func(string)) error {
				// Pre-probe candidate slots outside the flock to minimize lock
				// hold time. The probe is best-effort; re-check inside the lock.
				cfg, err := LoadConfig()
				if err != nil {
					return err
				}
				used := map[int]bool{0: true}
				for _, m := range cfg.Worktrees {
					if m.Slot > 0 {
						used[m.Slot] = true
					}
				}
				freeSlot := 0
				for slot := 1; slot <= 99; slot++ {
					if used[slot] {
						continue
					}
					if PortsBindable(slot) {
						freeSlot = slot
						break
					}
				}
				if freeSlot == 0 {
					return fmt.Errorf("no free slot ≤ 99 (you have a lot of worktrees!)")
				}
				// Lock only for the write, using the pre-probed slot.
				var slot int
				err = WithConfigLock(func(c *Config) error {
					// Re-check: another kit process may have grabbed it.
					if existing, ok := c.Worktrees[p.Name]; ok && existing.Slot > 0 {
						slot = existing.Slot
						return nil
					}
					for _, m := range c.Worktrees {
						if m.Slot == freeSlot {
							// Slot taken between probe and lock — fall back to full scan.
							s, err := c.AllocateSlot(p.Name, PortsBindable)
							if err != nil {
								return err
							}
							slot = s
							return nil
						}
					}
					c.Worktrees[p.Name] = WorktreeMeta{Slot: freeSlot}
					c.TouchLastUsed(p.Name)
					slot = freeSlot
					return nil
				})
				if err != nil {
					return err
				}
				ports := PortsForSlot(slot)
				if slotResult != nil {
					*slotResult = slot
				}
				emit(fmt.Sprintf("slot %d → app:%d admin:%d api:%d admin_be:%d",
					slot, ports.App, ports.Admin, ports.API, ports.AdminBE))
				return nil
			},
		},
	}
}

// StepTitles returns just the titles for preview before run.
func (l Layout) StepTitles(p DressPlan) []string {
	steps := l.planSteps(p, nil)
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.title)
	}
	return out
}

// RunDress executes the plan, emitting StepUpdate events on the returned channel.
// Channel is closed when finished. Stops on first failure.
func (l Layout) RunDress(p DressPlan) <-chan StepUpdate {
	ch := make(chan StepUpdate, 64)
	go func() {
		defer close(ch)
		var slot int
		worktreeAdded := false
		dbCreated := false
		gtabWritten := false
		slotAllocated := false
		steps := l.planSteps(p, &slot)

		// Run sequential steps 0-5 (fetch, worktree, env, DB create, DB clone, DB env).
		for i := 0; i <= 5 && i < len(steps); i++ {
			s := steps[i]
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
			elapsed := time.Since(start)
			if err != nil {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepFailed, Err: err, Elapsed: elapsed}
				l.rollbackDress(p, worktreeAdded, dbCreated, gtabWritten, slotAllocated, func(line string) {
					ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning, Line: line}
				})
				return
			}
			switch i {
			case 1:
				worktreeAdded = true
			case 3:
				dbCreated = true
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepDone, Elapsed: elapsed, AllocatedSlot: slot}
		}

		// Fan out independent steps 6-9 in parallel (backend install, frontend
		// symlink, graphite track, write gtab). These have no dependency on each
		// other after the worktree and DB exist.
		parallelStart := 6
		parallelEnd := 9
		if parallelEnd >= len(steps) {
			parallelEnd = len(steps) - 1
		}
		type pResult struct {
			index   int
			err     error
			elapsed time.Duration
		}
		var wg sync.WaitGroup
		results := make([]pResult, parallelEnd-parallelStart+1)
		for i := parallelStart; i <= parallelEnd; i++ {
			s := steps[i]
			idx := i - parallelStart
			if s.skip {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepSkipped}
				results[idx] = pResult{index: i}
				continue
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning}
			wg.Add(1)
			go func(i int, s step, idx int) {
				defer wg.Done()
				start := time.Now()
				emit := func(line string) {
					ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning, Line: line}
				}
				err := s.run(emit)
				results[idx] = pResult{index: i, err: err, elapsed: time.Since(start)}
			}(i, s, idx)
		}
		wg.Wait()

		// Emit results for parallel steps. First failure aborts.
		for _, r := range results {
			s := steps[r.index]
			if r.err != nil {
				ch <- StepUpdate{Index: r.index, Title: s.title, Status: StepFailed, Err: r.err, Elapsed: r.elapsed}
				l.rollbackDress(p, worktreeAdded, dbCreated, gtabWritten, slotAllocated, func(line string) {
					ch <- StepUpdate{Index: r.index, Title: s.title, Status: StepRunning, Line: line}
				})
				return
			}
			if r.index == 9 {
				gtabWritten = true
			}
			ch <- StepUpdate{Index: r.index, Title: s.title, Status: StepDone, Elapsed: r.elapsed, AllocatedSlot: slot}
		}

		// Run remaining sequential steps (10+: allocate port slot).
		for i := parallelEnd + 1; i < len(steps); i++ {
			s := steps[i]
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
			elapsed := time.Since(start)
			if err != nil {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepFailed, Err: err, Elapsed: elapsed}
				l.rollbackDress(p, worktreeAdded, dbCreated, gtabWritten, slotAllocated, func(line string) {
					ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning, Line: line}
				})
				return
			}
			if i == 10 {
				gtabWritten = true
				slotAllocated = true
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepDone, Elapsed: elapsed, AllocatedSlot: slot}
		}
	}()
	return ch
}

func (l Layout) rollbackDress(p DressPlan, worktreeAdded, dbCreated, gtabWritten, slotAllocated bool, emit func(string)) {
	cleanup := func(label string, fn func() error) {
		if err := fn(); err != nil {
			emit(fmt.Sprintf("cleanup %s failed: %v", label, err))
			return
		}
		emit("cleaned " + label)
	}

	if gtabWritten {
		cleanup("gtab workspace", func() error { return l.RemoveGtab(p.Name) })
	}
	if worktreeAdded {
		cleanup("run directory", func() error { return RemoveRunDir(p.Name) })
	}
	if dbCreated {
		cleanup("database "+DBName(p.Name), func() error { return DropDB(DBName(p.Name), nil) })
	}
	if worktreeAdded {
		cleanup("worktree", func() error { return l.RemoveWorktree(p.Worktree, nil) })
		cleanup("branch", func() error { return l.DeleteBranch(p.Name, nil) })
	}
	if slotAllocated {
		cleanup("port slot", func() error {
			return WithConfigLock(func(c *Config) error {
				c.FreeSlot(p.Name)
				return nil
			})
		})
	}
}
