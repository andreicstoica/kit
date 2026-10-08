package liftoff

import (
	"fmt"
	"sync"
	"sync/atomic"
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
	templated := false // set by the create step, read by the clone step that follows it
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
			title: "create database " + dbName + " (template copy of liftoff when possible)",
			skip:  !p.CloneDB,
			run: func(emit func(string)) error {
				if cloneByTemplate {
					copied, err := CreateDBFromTemplate(dbName, "liftoff", emit)
					if err != nil {
						return err
					}
					if copied {
						templated = true
						return nil
					}
				}
				return CreateDB(dbName, emit)
			},
		},
		{
			title: "clone database liftoff -> " + dbName,
			skip:  !p.CloneDB,
			run: func(emit func(string)) error {
				if templated {
					emit("already copied by template")
					return nil
				}
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
					// Merge into the existing record: DatabaseName, paths,
					// and session ownership recorded earlier in this run
					// must survive slot allocation.
					meta := c.Worktrees[p.Name]
					meta.Slot = freeSlot
					c.Worktrees[p.Name] = meta
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
		// Snapshot any pre-existing record so a fully successful rollback
		// restores it instead of deleting a record this run did not create.
		var priorMeta WorktreeMeta
		hadRecord := false
		if cfg, err := LoadConfig(); err == nil {
			priorMeta, hadRecord = cfg.Worktrees[p.Name]
		}
		steps := l.planSteps(p, &slot)

		type pResult struct {
			index   int
			err     error
			elapsed time.Duration
		}
		var wg sync.WaitGroup
		var dbResult pResult

		// The database chain (create, clone) needs neither the fetch nor the
		// checkout, so it starts now. Only the backend DB-name update edits
		// files in the checkout and waits for steps 0-2. Every failure path
		// joins this goroutine before rollback so dbCreated is settled.
		prefixDone := make(chan struct{})
		prefixOK := false
		var abort atomic.Bool
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 3; i <= 5; i++ {
				s := steps[i]
				if s.skip {
					ch <- StepUpdate{Index: i, Title: s.title, Status: StepSkipped}
					continue
				}
				if i < 5 && abort.Load() {
					return // checkout steps failed; do not start more database work
				}
				if i == 5 {
					<-prefixDone
					if !prefixOK {
						return
					}
				}
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning}
				start := time.Now()
				err := s.run(func(line string) { ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning, Line: line} })
				if err == nil && i == 3 {
					dbCreated = true
					err = persistDressDatabase(p)
				}
				elapsed := time.Since(start)
				if err != nil {
					dbResult = pResult{index: i, err: err, elapsed: elapsed}
					ch <- StepUpdate{Index: i, Title: s.title, Status: StepFailed, Err: err, Elapsed: elapsed}
					return
				}
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepDone, Elapsed: elapsed}
			}
		}()

		// Fetch, create the checkout, and copy env files before the checkout-only work.
		for i := 0; i <= 2 && i < len(steps); i++ {
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
			if err == nil && i == 1 {
				worktreeAdded = true
				err = persistDressCheckout(p)
			}
			if err != nil {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepFailed, Err: err, Elapsed: elapsed}
				abort.Store(true)
				close(prefixDone)
				wg.Wait()
				l.rollbackDress(p, worktreeAdded, dbCreated, gtabWritten, priorMeta, hadRecord, func(line string) {
					ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning, Line: line}
				})
				return
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepDone, Elapsed: elapsed, AllocatedSlot: slot}
		}
		prefixOK = true
		close(prefixDone)

		// Dependency installation, frontend links, Graphite and gtab need only
		// the checkout. Run them alongside the rest of the DB chain, then join
		// every branch before rollback or slot allocation. No rollback may
		// delete files a sibling is still using.
		parallelStart := 6
		parallelEnd := 9
		if parallelEnd >= len(steps) {
			parallelEnd = len(steps) - 1
		}
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
				status := StepDone
				if err != nil {
					status = StepFailed
				}
				ch <- StepUpdate{Index: i, Title: s.title, Status: status, Err: err, Elapsed: results[idx].elapsed}
			}(i, s, idx)
		}
		wg.Wait()

		// All branches have joined. Mark every resource success first: a
		// failure must still roll back a sibling step that succeeded in the
		// same batch (notably a written gtab file), not abort on the first
		// error in index order and leak the later success.
		for _, r := range results {
			if r.err == nil && !steps[r.index].skip && r.index == 9 {
				gtabWritten = true
			}
		}
		if dbResult.err != nil {
			l.rollbackDress(p, worktreeAdded, dbCreated, gtabWritten, priorMeta, hadRecord, func(line string) {
				ch <- StepUpdate{Index: dbResult.index, Title: steps[dbResult.index].title, Status: StepRunning, Line: line}
			})
			return
		}
		for _, r := range results {
			s := steps[r.index]
			if r.err != nil {
				l.rollbackDress(p, worktreeAdded, dbCreated, gtabWritten, priorMeta, hadRecord, func(line string) {
					ch <- StepUpdate{Index: r.index, Title: s.title, Status: StepRunning, Line: line}
				})
				return
			}
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
				l.rollbackDress(p, worktreeAdded, dbCreated, gtabWritten, priorMeta, hadRecord, func(line string) {
					ch <- StepUpdate{Index: i, Title: s.title, Status: StepRunning, Line: line}
				})
				return
			}
			ch <- StepUpdate{Index: i, Title: s.title, Status: StepDone, Elapsed: elapsed, AllocatedSlot: slot}
		}
	}()
	return ch
}

func (l Layout) rollbackDress(p DressPlan, worktreeAdded, dbCreated, gtabWritten bool, priorMeta WorktreeMeta, hadRecord bool, emit func(string)) {
	failed := false
	cleanup := func(label string, fn func() error) {
		if err := fn(); err != nil {
			failed = true
			emit(fmt.Sprintf("cleanup %s failed: %v", label, err))
			return
		}
		emit("cleaned " + label)
	}

	// Prefer the exact persisted database name: the run recorded ownership
	// right after createdb, before any clone ran.
	dbName := DBName(p.Name)
	if cfg, err := LoadConfig(); err == nil {
		if m, ok := cfg.Worktrees[p.Name]; ok && m.DatabaseName != "" {
			dbName = m.DatabaseName
		}
	}

	if gtabWritten {
		cleanup("gtab workspace", func() error { return l.RemoveGtab(p.Name) })
	}
	if worktreeAdded {
		cleanup("run directory", func() error { return RemoveRunDir(p.Name) })
	}
	if dbCreated {
		cleanup("database "+dbName, func() error { return DropDB(dbName, nil) })
	}
	if worktreeAdded {
		cleanup("worktree", func() error { return l.RemoveWorktree(p.Worktree, nil) })
		cleanup("branch", func() error { return l.DeleteBranch(p.Name, nil) })
	}
	if failed {
		// A failed rollback must retain the recovery record — checkout
		// pointer, branch, and exact database name with CleanupPending —
		// instead of silently losing the resources it could not remove.
		// Same retain-on-failure contract as wash/reconcile cleanup.
		if err := WithConfigLock(func(c *Config) error {
			meta := c.Worktrees[p.Name]
			meta.CleanupPending = true
			if meta.Branch == "" {
				meta.Branch = p.Name
			}
			if meta.Path == "" {
				meta.Path = p.Worktree
			}
			if meta.DatabaseName == "" && dbCreated {
				meta.DatabaseName = dbName
			}
			c.Worktrees[p.Name] = meta
			return nil
		}); err != nil {
			emit(fmt.Sprintf("cleanup incomplete; failed to persist recovery config: %v", err))
		} else {
			emit("cleanup incomplete; keeping config for retry")
		}
		return
	}
	// Fully rolled back: remove only the progressive traces this run wrote,
	// restoring any record that predates the run.
	if err := WithConfigLock(func(c *Config) error {
		if hadRecord {
			c.Worktrees[p.Name] = priorMeta
		} else {
			c.FreeSlot(p.Name)
		}
		return nil
	}); err != nil {
		emit(fmt.Sprintf("failed to restore config after rollback: %v", err))
	}
}

// persistDressCheckout records the new checkout immediately after `worktree
// add` succeeds, merging into any existing record. A later failure (or crash)
// keeps a typed pointer to the checkout instead of an untracked directory.
func persistDressCheckout(p DressPlan) error {
	return WithConfigLock(func(c *Config) error {
		meta := c.Worktrees[p.Name]
		if meta.Path == "" {
			meta.Path = p.Worktree
		}
		if meta.Branch == "" {
			meta.Branch = p.Name
		}
		c.Worktrees[p.Name] = meta
		return nil
	})
}

// persistDressDatabase records exact database ownership right after `createdb`
// succeeds and before any clone runs, so a clone or rollback failure can
// still identify the database this run created.
func persistDressDatabase(p DressPlan) error {
	dbName := DBName(p.Name)
	return WithConfigLock(func(c *Config) error {
		meta := c.Worktrees[p.Name]
		if meta.DatabaseName == "" {
			meta.DatabaseName = dbName
		}
		c.Worktrees[p.Name] = meta
		return nil
	})
}
