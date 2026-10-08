package liftoff

import (
	"fmt"
	"strings"
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
			snap := SnapshotProcs(p.ReplaceVictim, []Service{SvcCelery, SvcBeat})
			var err1, err2 error
			var stopWG sync.WaitGroup
			stopWG.Add(2)
			go func() { defer stopWG.Done(); err1 = snap.Stop(p.ReplaceVictim, SvcCelery) }()
			go func() { defer stopWG.Done(); err2 = snap.Stop(p.ReplaceVictim, SvcBeat) }()
			stopWG.Wait()
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

		// Updates with no Service are notes about the run as a whole.
		broker := CeleryBroker{}
		skipCelery := false
		vhostEnsured := false
		var lateEnsure sync.Once
		var lateErr error
		brokerReady := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(brokerReady)
			if !needsBroker(p.Services) {
				return
			}
			planned, note, managed := PlanCeleryBroker(p.Worktree, p.WorktreePath)
			broker = planned
			warn := false
			// rabbitmqctl boots an Erlang VM per call; only pay for it when a
			// backend service has to start.
			if managed && backendNeedsStart(p) {
				broker, note, warn = PrepareCeleryBroker(p.Worktree, p.WorktreePath)
				vhostEnsured = !warn
			}
			status := StepDone
			if warn {
				status = StepSkipped
			}
			ch <- PlayUpdate{Status: status, Title: note}
			if stale := ServicesOnOtherBroker(p.Worktree, p.Ports, broker); len(stale) > 0 {
				ch <- PlayUpdate{Status: StepSkipped, Title: fmt.Sprintf(
					"%s still on another celery broker; run `kit restart %s`",
					serviceLabels(stale), p.Worktree)}
			}
			// A worker on a shared broker consumes other worktrees' tasks.
			// Never start one beside a live worker; the user decides.
			if !broker.Isolated() && (hasService(p.Services, SvcCelery) || hasService(p.Services, SvcBeat)) {
				if owner, pid := FindSharedCeleryOwner(p.Worktree); owner != "" {
					skipCelery = true
					ch <- PlayUpdate{Status: StepSkipped, Title: fmt.Sprintf(
						"celery skipped: %s runs the worker (pid %d) on a shared broker; run `kit pause %s --only celery` first",
						owner, pid, owner)}
				}
			}
		}()

		// Fan out one goroutine per service. Emit StepRunning immediately
		// (so the UI shows a spinner) then StartService + readiness wait.
		for _, svc := range orderedServices(p.Services) {
			svc := svc
			wg.Add(1)
			go func() {
				defer wg.Done()
				serviceBroker := CeleryBroker{}
				if svc.IsBackend() {
					<-brokerReady
					serviceBroker = broker
					if skipCelery && (svc == SvcCelery || svc == SvcBeat) {
						ch <- PlayUpdate{Service: svc, Status: StepSkipped, Title: svc.Label() + " skipped (shared broker)"}
						return
					}
				}
				SweepStalePID(p.Worktree, string(svc))
				port := ServicePort(svc, p.Ports)

				// Idempotent: skip services already up so `kit play` on a
				// running worktree doesn't spawn a duplicate that fails to bind
				// the port. Use `kit restart` to force a fresh process.
				// serviceUp, not IsServiceAlive: beat starting in a sibling
				// goroutine must not read as "worker already running".
				if serviceUp(p.Worktree, svc, p.Ports) {
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

				// A backend that went down after the up-front check still needs
				// its private vhost before it connects.
				if svc.IsBackend() && serviceBroker.Isolated() && !vhostEnsured {
					lateEnsure.Do(func() { lateErr = ensureVHost(serviceBroker.VHost) })
					if lateErr != nil {
						ch <- PlayUpdate{Service: svc, Status: StepFailed, Title: title, Err: lateErr, Elapsed: time.Since(start)}
						return
					}
				}
				spec := SpecFor(p.Worktree, p.WorktreePath, svc, p.Ports, serviceBroker)
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
	Ports    Ports // Ports lets RunPause fall back to killing by port when a recorded PID is stale.
}

// RunPause kills selected services in parallel and removes their PID
// files. Best-effort: continues past individual failures. Each kill is
// independent so there's no startup-order dependency to respect.
func (l Layout) RunPause(p PausePlan) <-chan PlayUpdate {
	ch := make(chan PlayUpdate, 16)
	go func() {
		defer close(ch)
		var wg sync.WaitGroup
		snap := SnapshotProcs(p.Worktree, p.Services)
		for _, svc := range orderedServices(p.Services) {
			svc := svc
			wg.Add(1)
			go func() {
				defer wg.Done()
				pid := ReadPID(p.Worktree, string(svc))
				port := ServicePort(svc, p.Ports)
				// pid 0 usually means not running — but a reloaded service
				// (uvicorn --reload re-exec) can outlive its recorded pid, so
				// a still-listening port proceeds to the port-kill fallback.
				if pid == 0 && !(port > 0 && PortListening(port)) {
					ch <- PlayUpdate{Service: svc, Status: StepSkipped, Title: "stop " + svc.Label() + " (not running)"}
					return
				}
				title := fmt.Sprintf("stop %s", svc.Label())
				if pid > 0 {
					title += fmt.Sprintf(" (pid %d)", pid)
				}
				ch <- PlayUpdate{Service: svc, Status: StepRunning, Title: title, PID: pid}
				start := time.Now()
				err := snap.Stop(p.Worktree, svc)
				if err == nil && port > 0 && PortListening(port) {
					// Recorded pid was stale (or absent) but the port is still
					// bound — kill whatever is actually listening.
					_ = KillListenersOnPort(port)
				}
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

func needsBroker(svcs []Service) bool {
	for _, s := range svcs {
		if s.IsBackend() {
			return true
		}
	}
	return false
}

func hasService(svcs []Service, want Service) bool {
	for _, s := range svcs {
		if s == want {
			return true
		}
	}
	return false
}

// backendNeedsStart reports whether any selected backend service is down.
func backendNeedsStart(p PlayPlan) bool {
	for _, s := range p.Services {
		if s.IsBackend() && !serviceUp(p.Worktree, s, p.Ports) {
			return true
		}
	}
	return false
}

func serviceLabels(svcs []Service) string {
	labels := make([]string, len(svcs))
	for i, s := range svcs {
		labels[i] = s.Label()
	}
	return strings.Join(labels, ", ")
}
