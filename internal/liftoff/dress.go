package liftoff

import (
	"fmt"
	"time"
)

// DressPlan captures every choice for a `kit design` run.
// (The struct name predates the command rename from `dress` → `design`;
// kept as-is to minimize churn in internal call sites.)
type DressPlan struct {
	Name              string
	Worktree          string
	CloneDB           bool
	BackendDeps       bool
	SymlinkFrontend   bool // symlink frontend node_modules from master
	GraphiteTrack     bool
	Gtab              bool
	OverwriteEnvs     bool // force overwrite if env files already exist in worktree

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
			title: "pip install backend",
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
				st, err := LoadState()
				if err != nil {
					return err
				}
				slot, err := st.AllocateSlot(p.Name, PortsBindable)
				if err != nil {
					return err
				}
				st.TouchLastUsed(p.Name)
				if err := st.Save(); err != nil {
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
		steps := l.planSteps(p, &slot)
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
			elapsed := time.Since(start)
			if err != nil {
				ch <- StepUpdate{Index: i, Title: s.title, Status: StepFailed, Err: err, Elapsed: elapsed}
				return
			}
			ch <- StepUpdate{
				Index:         i,
				Title:         s.title,
				Status:        StepDone,
				Elapsed:       elapsed,
				AllocatedSlot: slot,
			}
		}
	}()
	return ch
}
