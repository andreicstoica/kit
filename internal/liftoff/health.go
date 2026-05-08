package liftoff

import (
	"context"
	"errors"
	"time"
)

// WaitForPort polls 127.0.0.1:port until it accepts a connection or timeout.
// Returns nil if the port came up; an error otherwise.
func WaitForPort(port int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		if PortListening(port) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("port did not come up in time")
		case <-tick.C:
		}
	}
}

// WaitForPID returns nil if the PID is alive after `wait` time has passed.
// Used for non-port services like celery worker/beat.
func WaitForPID(pid int, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if !IsAlive(pid) {
			return errors.New("process died during startup")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !IsAlive(pid) {
		return errors.New("process not alive after wait")
	}
	return nil
}
