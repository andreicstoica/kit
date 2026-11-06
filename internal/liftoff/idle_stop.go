package liftoff

import (
	"fmt"
	"syscall"
)

func stopIdleService(name string, svc Service, expected int) error {
	return withWorkspaceLock("start:"+name+":"+string(svc), func() error {
		if expected <= 0 || ReadPID(name, string(svc)) != expected {
			return fmt.Errorf("%s changed since idle observation", svc)
		}
		snapshot := SnapshotProcs(name, []Service{svc})
		if snapshot.err != nil {
			return snapshot.err
		}
		tag := serviceTag(name, svc)
		root, ok := snapshot.procs[expected]
		if !ok || root.Zombie {
			return RemovePID(name, string(svc))
		}
		if !snapshot.tagged[tag][expected] || root.PGID != expected {
			return fmt.Errorf("%s process ownership is unknown", svc)
		}
		procs := make(map[int]procInfo, len(snapshot.procs))
		for id, p := range snapshot.procs {
			p.Tagged = snapshot.tagged[tag][id]
			procs[id] = p
		}
		targets, groups := serviceTree(procs, expected, 0)
		if svc == SvcCelery {
			// Only the worker parent gets TERM; it drains in-flight tasks before exiting.
			if err := syscall.Kill(expected, syscall.SIGTERM); err != nil {
				return err
			}
		} else {
			for _, group := range groups {
				if err := syscall.Kill(-group, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
					return err
				}
			}
			for _, pid := range targets {
				if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
					return err
				}
			}
		}
		if remaining := waitGone(targets, stopGrace); len(remaining) > 0 {
			return fmt.Errorf("%s is still draining; no SIGKILL sent", svc)
		}
		return RemovePID(name, string(svc))
	})
}
