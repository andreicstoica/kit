package liftoff

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// serviceTagKey marks every process kit spawns. Children inherit it, so
// StopService can find a service's whole tree even when the pidfile is
// missing, stale or points at only part of it.
const serviceTagKey = "KIT_SERVICE"

func serviceTag(worktree string, svc Service) string {
	return serviceTagKey + "=" + worktree + "/" + string(svc)
}

// Stop timing: SIGTERM, wait stopGrace, SIGKILL, wait killWait, then fail.
// Vars so tests can shorten them.
var (
	stopGrace = 3 * time.Second
	killWait  = 2 * time.Second
)

type procInfo struct {
	PID, PPID, PGID int
	Zombie          bool
	Tagged          bool
}

// StopService stops every process that belongs to the service and removes its
// pidfile. It returns an error, and keeps the pidfile, if any of them survive
// SIGKILL, so callers never start a second copy next to a live one.
//
// The service's processes are: the recorded pid, every member of its process
// group, every descendant of those (a child that moved to its own group
// escapes a group kill), and every process tagged with this service.
//
// A recorded pid that is not tagged (launched by an older kit, or recycled
// after a reboot) is trusted only if it leads its own group and did not start
// before kit's recorded launch. Otherwise kit skips it: killing its group
// could hit unrelated processes.
func StopService(worktree string, svc Service) error {
	pid := ReadPID(worktree, string(svc))
	procs, err := snapshotProcs(serviceTag(worktree, svc))
	if err != nil {
		return stopRecordedGroup(worktree, svc, pid)
	}
	root := 0
	if p, ok := procs[pid]; ok && pid > 0 && !p.Zombie {
		if p.Tagged || ownsRecordedPID(worktree, svc, p) {
			root = pid
		} else {
			fmt.Fprintf(os.Stderr, "kit: stale pid %d for %s/%s — skipping kill\n", pid, worktree, svc)
		}
	}
	targets, groups := serviceTree(procs, root, os.Getpid())
	if len(targets) == 0 {
		return RemovePID(worktree, string(svc))
	}
	if err := terminate(targets, groups); err != nil {
		return fmt.Errorf("stop %s/%s: %w", worktree, svc, err)
	}
	return RemovePID(worktree, string(svc))
}

func ownsRecordedPID(worktree string, svc Service, p procInfo) bool {
	started, _ := ReadStarted(worktree, string(svc))
	return p.PGID == p.PID && !looksStale(p.PID, started)
}

// stopRecordedGroup is the fallback when ps is unavailable: group-kill the
// recorded pid only.
func stopRecordedGroup(worktree string, svc Service, pid int) error {
	if pid == 0 {
		return nil
	}
	if !IsAlive(pid) {
		return RemovePID(worktree, string(svc))
	}
	started, _ := ReadStarted(worktree, string(svc))
	pgid, pgErr := syscall.Getpgid(pid)
	if looksStale(pid, started) || pgErr != nil || pgid != pid {
		fmt.Fprintf(os.Stderr, "kit: stale pid %d for %s/%s — skipping kill\n", pid, worktree, svc)
		return RemovePID(worktree, string(svc))
	}
	if err := KillGroup(pid); err != nil {
		return err
	}
	return RemovePID(worktree, string(svc))
}

// snapshotProcs lists every process with its parent, group and state, and
// marks the ones whose environment contains tag.
func snapshotProcs(tag string) (map[int]procInfo, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,pgid=,stat=").Output()
	if err != nil {
		return nil, err
	}
	procs := parsePS(string(out))
	if tag != "" {
		// macOS ps -E appends each process's environment to its command line.
		// If it fails, kit still stops the recorded tree.
		if envOut, err := exec.Command("ps", "-E", "-ww", "-axo", "pid=,command=").Output(); err == nil {
			for _, pid := range parseTagged(string(envOut), tag) {
				if p, ok := procs[pid]; ok {
					p.Tagged = true
					procs[pid] = p
				}
			}
		}
	}
	return procs, nil
}

// parsePS parses `ps -axo pid=,ppid=,pgid=,stat=` output.
func parsePS(out string) map[int]procInfo {
	procs := map[int]procInfo{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		ppid, e2 := strconv.Atoi(f[1])
		pgid, e3 := strconv.Atoi(f[2])
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		procs[pid] = procInfo{PID: pid, PPID: ppid, PGID: pgid, Zombie: strings.HasPrefix(f[3], "Z")}
	}
	return procs
}

// parseTagged returns the pids whose `ps -E` line has tag as a whole word.
func parseTagged(out, tag string) []int {
	var pids []int
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		for _, w := range f[1:] {
			if w == tag {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids
}

// serviceTree returns the live pids to stop and the process groups to signal.
// Seeds are root (if non-zero) and every tagged process. A seed that leads its
// group brings in the whole group; then every descendant is added. self and
// pid 1 are never returned.
func serviceTree(procs map[int]procInfo, root, self int) (pids, groups []int) {
	in := map[int]bool{}
	leaders := map[int]bool{}
	add := func(pid int) {
		p, ok := procs[pid]
		if !ok || p.Zombie || pid == self || pid <= 1 || in[pid] {
			return
		}
		in[pid] = true
		if p.PGID == pid {
			leaders[pid] = true
		}
	}
	if root > 0 {
		add(root)
	}
	for pid, p := range procs {
		if p.Tagged {
			add(pid)
		}
	}
	for _, p := range procs {
		if leaders[p.PGID] {
			add(p.PID)
		}
	}
	children := map[int][]int{}
	for _, p := range procs {
		children[p.PPID] = append(children[p.PPID], p.PID)
	}
	queue := make([]int, 0, len(in))
	for pid := range in {
		queue = append(queue, pid)
	}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, c := range children[pid] {
			if !in[c] {
				add(c)
				if in[c] {
					queue = append(queue, c)
				}
			}
		}
	}
	for pid := range in {
		pids = append(pids, pid)
	}
	for g := range leaders {
		groups = append(groups, g)
	}
	sort.Ints(pids)
	sort.Ints(groups)
	return pids, groups
}

// terminate sends SIGTERM to the groups and pids, escalates to SIGKILL after
// stopGrace, and fails if any pid is still alive killWait later.
func terminate(pids, groups []int) error {
	signal := func(sig syscall.Signal, targets []int) {
		for _, g := range groups {
			_ = syscall.Kill(-g, sig)
		}
		for _, pid := range targets {
			_ = syscall.Kill(pid, sig)
		}
	}
	signal(syscall.SIGTERM, pids)
	if waitGone(pids, stopGrace) == nil {
		return nil
	}
	signal(syscall.SIGKILL, livePIDs(pids))
	survivors := waitGone(pids, killWait)
	if len(survivors) > 0 {
		return fmt.Errorf("pids %v still alive after SIGKILL", survivors)
	}
	return nil
}

// waitGone polls until every pid has exited (or is a zombie) and returns the
// ones still alive at the deadline.
func waitGone(pids []int, timeout time.Duration) []int {
	deadline := time.Now().Add(timeout)
	for {
		live := livePIDs(pids)
		if len(live) == 0 || time.Now().After(deadline) {
			return live
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// livePIDs returns the pids that are running. Zombies count as gone: they
// only wait for their parent to reap them.
func livePIDs(pids []int) []int {
	if len(pids) == 0 {
		return nil
	}
	ids := make([]string, len(pids))
	for i, pid := range pids {
		ids[i] = strconv.Itoa(pid)
	}
	out, err := exec.Command("ps", "-p", strings.Join(ids, ","), "-o", "pid=,ppid=,pgid=,stat=").Output()
	// ps exits 1 with no output when all requested processes are gone.
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 && len(out) == 0 && len(exit.Stderr) == 0 {
		return nil
	}
	procs := parsePS(string(out))
	var live []int
	for _, pid := range pids {
		if err != nil {
			if IsAlive(pid) {
				live = append(live, pid)
			}
			continue
		}
		if p, ok := procs[pid]; ok && !p.Zombie {
			live = append(live, pid)
		}
	}
	return live
}
