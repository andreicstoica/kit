package liftoff

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func idleCommand(dir string, env []string, command string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return output, err
}

func insideWorktree(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (l Layout) observeIdle() ([]IdleObservation, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	rex, rexErr := ReadRexState()
	cwdOutput, cwdErr := idleCommand("", nil, "lsof", "-nP", "-a", "-u", strconv.Itoa(os.Getuid()), "-d", "cwd", "-Fpn")
	cwds := idleProcessDirectories(string(cwdOutput))
	tags := []string{}
	for name := range config.Worktrees {
		for _, svc := range AllServices {
			tags = append(tags, serviceTag(name, svc))
		}
	}
	procs, tagged, procErr := snapshotProcs(tags...)
	names := config.SortedNames()
	sort.Strings(names)
	results := make([]IdleObservation, 0, len(names))
	for _, name := range names {
		meta := config.Worktrees[name]
		o := IdleObservation{Name: name, Activity: meta.LastUsed, Services: map[Service]int{}}
		if name == "master" || meta.CleanupPending || meta.Parked {
			o.Blocked = "protected workspace"
			results = append(results, o)
			continue
		}
		path, err := l.ResolveWorktreePath(name)
		if err != nil || cleanPath(path) == cleanPath(l.Master) {
			o.Blocked = "workspace path is unknown or protected"
			results = append(results, o)
			continue
		}
		o.Path = path
		for _, svc := range AllServices {
			pid := ReadPID(name, string(svc))
			if p, ok := procs[pid]; pid > 0 && ok && !p.Zombie {
				o.Services[svc] = pid
			}
		}
		if procErr != nil {
			o.Blocked = "process activity unavailable"
		} else if len(o.Services) == 0 {
			o.Blocked = "no recorded services running"
		} else if rexErr != nil {
			o.Blocked = "terminal activity unavailable"
		} else if cwdErr != nil {
			o.Blocked = "job activity unavailable"
		}
		if o.Blocked != "" {
			results = append(results, o)
			continue
		}
		allowed := map[int]bool{os.Getpid(): true}
		for svc, pid := range o.Services {
			if !insideWorktree(cwds[pid], path) || !tagged[serviceTag(name, svc)][pid] || procs[pid].PGID != pid {
				o.Blocked = "service ownership is unknown"
				break
			}
			serviceProcs := make(map[int]procInfo, len(procs))
			for id, p := range procs {
				p.Tagged = tagged[serviceTag(name, svc)][id]
				serviceProcs[id] = p
			}
			owned, _ := serviceTree(serviceProcs, pid, os.Getpid())
			for _, id := range owned {
				allowed[id] = true
			}
			if started, ok := ReadStarted(name, string(svc)); ok && started.After(o.Activity) {
				o.Activity = started
			}
		}
		if o.Blocked == "" {
			o.Blocked = idleTerminalActivity(rex, meta.RexID, path, allowed, &o.Activity)
			allowIdleAncestors(procs, allowed)
		}
		if o.Blocked == "" {
			for pid, cwd := range cwds {
				if insideWorktree(cwd, path) && !allowed[pid] {
					o.Blocked = fmt.Sprintf("job or editor process %d is present", pid)
					break
				}
			}
		}
		if o.Blocked == "" {
			ports := PortsForSlot(meta.Slot)
			for svc := range o.Services {
				if port := ServicePort(svc, ports); port > 0 {
					out, err := idleCommand("", nil, "lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:ESTABLISHED", "-t")
					if len(out) > 0 {
						o.Blocked = "open client connection"
						break
					}
					if err != nil {
						if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 || len(exit.Stderr) > 0 {
							o.Blocked = "client activity unavailable"
							break
						}
					}
				}
			}
		}
		if o.Blocked == "" && (o.Services[SvcCelery] > 0 || o.Services[SvcBeat] > 0) {
			o.Blocked = idleWorkerActivity(name, path)
		}
		if o.Blocked == "" {
			o.Fingerprint, err = idleCodeFingerprint(path)
			if err != nil {
				o.Blocked = "code activity unavailable: " + err.Error()
			} else {
				ids := []string{}
				for svc, pid := range o.Services {
					ids = append(ids, fmt.Sprintf("%s:%d", svc, pid))
				}
				sort.Strings(ids)
				o.Fingerprint += "/" + path + "/" + strings.Join(ids, ",") + "/" + meta.RexID
			}
		}
		results = append(results, o)
	}
	return results, nil
}

func idleProcessDirectories(output string) map[int]string {
	result := map[int]string{}
	pid := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "p") {
			pid, _ = strconv.Atoi(line[1:])
		} else if strings.HasPrefix(line, "n") && pid > 0 {
			result[pid] = line[1:]
		}
	}
	return result
}

func idleTerminalActivity(state RexState, sessionID, path string, allowed map[int]bool, activity *time.Time) string {
	for _, session := range state.Sessions {
		blocks := append([]RexBlock(nil), session.DetachedBlocks...)
		for _, window := range session.Windows {
			blocks = append(blocks, window.Blocks...)
		}
		for _, block := range blocks {
			foreground := block.Process.Foreground
			relevant := session.SessionID == sessionID
			if foreground != nil && insideWorktree(foreground.Cwd, path) {
				relevant = true
			}
			if !relevant {
				continue
			}
			if block.Creator != "com.superlogical.terminal" || foreground == nil {
				return "unknown terminal activity"
			}
			isShell := func(p *RexProcess) bool {
				if p == nil {
					return false
				}
				switch strings.TrimPrefix(filepath.Base(p.Name), "-") {
				case "zsh", "bash", "sh", "fish", "login":
					return true
				}
				return false
			}
			if isShell(block.Process.Child) {
				allowed[block.Process.Child.PID] = true
			}
			if isShell(foreground) {
				allowed[foreground.PID] = true
				continue
			}
			verified := false
			for _, record := range block.ProgramStatus.Records {
				if record.Owner == nil || record.Owner.PID != foreground.PID || record.Owner.Name != foreground.Name {
					continue
				}
				if record.UpdatedAt.IsZero() {
					return "unknown agent activity"
				}
				verified = true
				if record.State != "idle" && record.State != "done" {
					return "agent is " + record.State
				}
				if record.UpdatedAt.After(*activity) {
					*activity = record.UpdatedAt
				}
			}
			if !verified {
				return "unknown agent or foreground job activity"
			}
			allowed[foreground.PID] = true
		}
	}
	return ""
}

func idleCodeFingerprint(path string) (string, error) {
	status, err := idleCommand(path, nil, "git", "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	head, err := idleCommand(path, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	hash.Write(head)
	hash.Write(status)
	entries := strings.Split(string(status), "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		file := filepath.Join(path, entry[3:])
		if !insideWorktree(file, path) {
			return "", fmt.Errorf("git returned a path outside the worktree")
		}
		st, err := os.Lstat(file)
		if err == nil {
			fmt.Fprintf(hash, "%s:%d:%d\x00", entry[3:], st.Size(), st.ModTime().UnixNano())
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if strings.ContainsAny(entry[:2], "RC") {
			i++
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func idleWorkerActivity(name, path string) string {
	broker, _, managed := PlanCeleryBroker(name, path)
	if !managed || !broker.Isolated() {
		return "shared or unmanaged worker"
	}
	if len(ServicesOnOtherBroker(name, Ports{}, broker)) > 0 {
		return "worker broker ownership changed"
	}
	output, err := idleCommand(filepath.Join(path, "backend"), append(broker.Env(), "environment=dev"), venvBin("celery"), "-A", "common.celery", "inspect", "active", "--json", "--timeout", "2")
	if err != nil {
		return "worker task activity unavailable"
	}
	var workers map[string][]json.RawMessage
	if json.Unmarshal(output, &workers) != nil || len(workers) == 0 {
		return "worker task activity unknown"
	}
	for _, tasks := range workers {
		if len(tasks) > 0 {
			return "worker has active tasks"
		}
	}
	return ""
}

// Login shells can sit between the terminal root and a verified idle agent.
// Only admit the connecting ancestors, never their other background children.
func allowIdleAncestors(procs map[int]procInfo, allowed map[int]bool) {
	for pid := range allowed {
		chain := []int{}
		parent := procs[pid].PPID
		for steps := 0; parent > 1 && steps < 64; steps++ {
			if allowed[parent] {
				for _, id := range chain {
					allowed[id] = true
				}
				break
			}
			chain = append(chain, parent)
			next, ok := procs[parent]
			if !ok || next.PPID == parent {
				break
			}
			parent = next.PPID
		}
	}
}
