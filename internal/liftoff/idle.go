package liftoff

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const idlePollGap = 10 * time.Minute

type IdleObservation struct {
	Name        string          `json:"name"`
	Path        string          `json:"path"`
	Fingerprint string          `json:"-"`
	Activity    time.Time       `json:"last_activity,omitempty"`
	Blocked     string          `json:"reason,omitempty"`
	Services    map[Service]int `json:"services,omitempty"`
	Eligible    bool            `json:"eligible"`
	Since       time.Time       `json:"idle_since,omitempty"`
}

type idleRecord struct {
	Fingerprint string    `json:"fingerprint"`
	Since       time.Time `json:"since"`
	Observed    time.Time `json:"observed"`
}

type IdleTracker map[string]idleRecord

func (t IdleTracker) Observe(o IdleObservation, now time.Time, after time.Duration) bool {
	previous, exists := t[o.Name]
	if o.Blocked != "" {
		delete(t, o.Name)
		return false
	}
	if !exists || previous.Fingerprint != o.Fingerprint || now.Before(previous.Observed) || now.Sub(previous.Observed) > idlePollGap {
		previous = idleRecord{Fingerprint: o.Fingerprint, Since: now}
	}
	if o.Activity.After(previous.Since) {
		previous.Since = o.Activity
	}
	previous.Observed = now
	t[o.Name] = previous
	return !now.Before(previous.Since) && now.Sub(previous.Since) >= after
}

// IdleSweep requires repeated successful observations. A restart after sleep or
// an unavailable probe must not turn unobserved time into permission to stop.
func (l Layout) IdleSweep(after time.Duration, apply bool) ([]IdleObservation, error) {
	if after < 15*time.Minute {
		return nil, errors.New("idle timeout must be at least 15 minutes")
	}
	var results []IdleObservation
	var failures []error
	err := WithWorkspaceLock("idle-sweep", func() error {
		if apply {
			logPath := filepath.Join(RunDirPath("idle-monitor"), "idle.log")
			if info, err := os.Stat(logPath); err == nil && info.Size() > 1024*1024 {
				_ = os.Remove(logPath + ".1")
				if err = os.Rename(logPath, logPath+".1"); err != nil {
					return fmt.Errorf("rotate idle log: %w", err)
				}
			}
		}
		tracker := IdleTracker{}
		statePath := filepath.Join(configDir(), "idle-state.json")
		raw, err := os.ReadFile(statePath)
		if err == nil {
			if err = json.Unmarshal(raw, &tracker); err != nil {
				return fmt.Errorf("read idle state: %w", err)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if tracker == nil {
			tracker = IdleTracker{}
		}
		observations, err := l.observeIdle()
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		present := map[string]bool{}
		for _, o := range observations {
			present[o.Name] = true
			o.Eligible = tracker.Observe(o, now, after)
			o.Since = tracker[o.Name].Since
			if o.Eligible && apply {
				fresh, err := l.observeIdle()
				if err != nil {
					o.Blocked = "recheck failed: " + err.Error()
					failures = append(failures, fmt.Errorf("recheck %s: %w", o.Name, err))
				} else {
					found := false
					for _, current := range fresh {
						if current.Name != o.Name {
							continue
						}
						found = true
						if current.Blocked != "" || current.Fingerprint != o.Fingerprint || current.Activity.After(o.Activity) {
							o.Blocked = "activity changed before pause"
						} else if err = l.pauseIdle(current); err != nil {
							o.Blocked = "pause incomplete: " + err.Error()
							failures = append(failures, fmt.Errorf("pause %s: %w", o.Name, err))
						} else {
							o.Blocked = "paused"
						}
					}
					if !found {
						o.Blocked = "workspace disappeared before pause"
					}
				}
				delete(tracker, o.Name)
			}
			results = append(results, o)
		}
		for name := range tracker {
			if !present[name] {
				delete(tracker, name)
			}
		}
		if apply {
			raw, err = json.MarshalIndent(tracker, "", "  ")
			if err != nil {
				return err
			}
			if err = os.WriteFile(statePath+".tmp", raw, 0600); err != nil {
				return err
			}
			return os.Rename(statePath+".tmp", statePath)
		}
		return nil
	})
	return results, errors.Join(append(failures, err)...)
}

func (l Layout) pauseIdle(o IdleObservation) error {
	var errs []error
	services := make([]Service, 0, len(o.Services))
	for svc := range o.Services {
		services = append(services, svc)
	}
	sort.Slice(services, func(i, j int) bool { return services[i] < services[j] })
	for _, svc := range services {
		if err := stopIdleService(o.Name, svc, o.Services[svc]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
