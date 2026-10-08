package liftoff

import (
	"errors"
	"fmt"
)

func (l Layout) Park(name string) error {
	return withWorkspaceLock("park:"+name, func() error {
		cfg, err := LoadConfig()
		if err != nil {
			return err
		}
		if err := l.validateParkRecord(cfg, name); err != nil {
			return err
		}
		var failures []error
		for _, svc := range AllServices {
			if err := StopService(name, svc); err != nil {
				failures = append(failures, err)
			}
		}
		if err := errors.Join(failures...); err != nil {
			return fmt.Errorf("workspace remains visible because services could not be stopped: %w", err)
		}
		return l.setParked(name, true)
	})
}

// Resume restores visibility only; starting services remains an explicit play action.
func (l Layout) Resume(name string) error {
	return withWorkspaceLock("park:"+name, func() error { return l.setParked(name, false) })
}

func (l Layout) validateParkRecord(cfg *Config, name string) error {
	meta, ok := cfg.Worktrees[name]
	if name == "master" || (meta.Path != "" && cleanPath(meta.Path) == cleanPath(l.Master)) {
		return errors.New("master cannot be parked or resumed")
	}
	if !ok {
		return fmt.Errorf("workspace %q is not registered with Kit; adopt it first", name)
	}
	if meta.CleanupPending {
		return fmt.Errorf("workspace %q has pending cleanup; finish recovery first", name)
	}
	return nil
}

func (l Layout) setParked(name string, parked bool) error {
	return WithConfigLock(func(cfg *Config) error {
		if err := l.validateParkRecord(cfg, name); err != nil {
			return err
		}
		meta := cfg.Worktrees[name]
		meta.Parked = parked
		cfg.Worktrees[name] = meta
		return nil
	})
}
