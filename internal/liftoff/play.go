package liftoff

import (
	"fmt"
	"sync"
	"time"
)

// PlayPlan captures everything needed for a `kit play` run.
type PlayPlan struct {
	Worktree      string
	WorktreePath  string
	Slot          int
	Ports         Ports
	Services      []Service
	ReplaceCelery bool   // user confirmed killing another worktree's celery
	ReplaceVictim string // name of the worktree losing its celery (display only)
}

// PlayUpdate is one event from the play runner.
type PlayUpdate struct {
	Service Service
	Status  StepStatus
	Title   string
	Message string
	PID     int
	Port    int
	URL     string
	Err     error
	Elapsed time.Duration
}

// RunPlay starts the selected services in parallel, emitting PlayUpdate
// events as each one transitions. Failures are best-effort — one service
// crashing no longer aborts the rest. Channel closes once every selected
// service has reported done/failed.
func (l Layout) RunPlay(p PlayPlan) <-chan PlayUpdate {
	ch := make(chan PlayUpdate, 32)
	go func() {
		defer close(ch)
		// Pre-step: replace existing celery if asked. Synchronous so the
		// new celery doesn't race against the kill.
		if p.ReplaceCelery && p.ReplaceVictim != "" {
			ch <- PlayUpdate{
				Service: SvcCelery,
				Status:  StepRunning,
				Title:   fmt.Sprintf("stop %s's celery (replacing)", p.ReplaceVictim),
			}
			start := time.Now()
			err1 := StopService(p.ReplaceVictim, SvcCelery)
			err2 := StopService(p.ReplaceVictim, SvcBeat)
			elapsed := time.Since(start)
			if err1 != nil || err2 != nil {
				ch <- PlayUpdate{
					Service: SvcCelery,
					Status:  StepFailed,
					Title:   fmt.Sprintf("stop %s's celery", p.ReplaceVictim),
					Err:     fmt.Errorf("worker: %v / beat: %v", err1, err2),
					Elapsed: elapsed,
				}
				return
			}
			ch <- PlayUpdate{
				Service: SvcCelery,
				Status:  StepDone,
				Title:   fmt.Sprintf("stopped %s's celery", p.ReplaceVictim),
				Elapsed: elapsed,
			}
		}

		// Fan out one goroutine per service. Emit StepRunning immediately
		// (so the UI shows a spinner) then StartService + readiness wait.
		var wg sync.WaitGroup
		for _, svc := range orderedServices(p.Services) {
			svc := svc
			wg.Add(1)
			go func() {
				defer wg.Done()
				SweepStalePID(p.Worktree, string(svc))
				port := ServicePort(svc, p.Ports)

				// Idempotent: skip services already up so `kit play` on a
				// running worktree doesn't spawn a duplicate that fails to bind
				// the port. Use `kit restart` to force a fresh process.
				if IsServiceAlive(p.Worktree, svc, p.Ports) {
					url := ""
					if port > 0 {
						url = fmt.Sprintf("http://localhost:%d", port)
					}
					ch <- PlayUpdate{
						Service: svc, Status: StepDone,
						Title: svc.Label() + " already running",
						Port:  port, URL: url,
					}
					return
				}

				title := fmt.Sprintf("start %s", svc.Label())
				if port > 0 {
					title += fmt.Sprintf(" on :%d", port)
				}
				ch <- PlayUpdate{Service: svc, Status: StepRunning, Title: title, Port: port}
				start := time.Now()

				spec := SpecFor(p.Worktree, p.WorktreePath, svc, p.Ports)
				pid, err := StartService(spec)
				if err != nil {
					ch <- PlayUpdate{
						Service: svc, Status: StepFailed, Title: title,
						Err: err, Elapsed: time.Since(start),
					}
					return
				}

				var readyErr error
				if port > 0 {
					readyErr = WaitForPort(port, 30*time.Second)
				} else {
					readyErr = WaitForPID(pid, 2*time.Second)
				}
				elapsed := time.Since(start)
				if readyErr != nil {
					ch <- PlayUpdate{
						Service: svc, Status: StepFailed, Title: title,
						PID: pid, Port: port,
						Err: readyErr, Elapsed: elapsed,
					}
					return
				}
				url := ""
				if port > 0 {
					url = fmt.Sprintf("http://localhost:%d", port)
				}
				ch <- PlayUpdate{
					Service: svc, Status: StepDone, Title: title,
					PID: pid, Port: port, URL: url, Elapsed: elapsed,
				}
			}()
		}
		wg.Wait()
	}()
	return ch
}

// PausePlan captures the choices for `kit pause`.
type PausePlan struct {
	Worktree string
	Services []Service
}

// RunPause kills selected services in parallel and removes their PID
// files. Best-effort: continues past individual failures. Each kill is
// independent so there's no startup-order dependency to respect.
func (l Layout) RunPause(p PausePlan) <-chan PlayUpdate {
	ch := make(chan PlayUpdate, 16)
	go func() {
		defer close(ch)
		var wg sync.WaitGroup
		for _, svc := range orderedServices(p.Services) {
			svc := svc
			wg.Add(1)
			go func() {
				defer wg.Done()
				pid := ReadPID(p.Worktree, string(svc))
				if pid == 0 {
					ch <- PlayUpdate{Service: svc, Status: StepSkipped, Title: "stop " + svc.Label() + " (not running)"}
					return
				}
				title := fmt.Sprintf("stop %s (pid %d)", svc.Label(), pid)
				ch <- PlayUpdate{Service: svc, Status: StepRunning, Title: title, PID: pid}
				start := time.Now()
				err := StopService(p.Worktree, svc)
				elapsed := time.Since(start)
				if err != nil {
					ch <- PlayUpdate{Service: svc, Status: StepFailed, Title: title, Err: err, Elapsed: elapsed}
					return
				}
				ch <- PlayUpdate{Service: svc, Status: StepDone, Title: title, Elapsed: elapsed}
			}()
		}
		wg.Wait()
	}()
	return ch
}

// orderedServices returns the user-selected services in canonical start order.
func orderedServices(selected []Service) []Service {
	want := map[Service]bool{}
	for _, s := range selected {
		want[s] = true
	}
	var out []Service
	for _, s := range AllServices {
		if want[s] {
			out = append(out, s)
		}
	}
	return out
}
