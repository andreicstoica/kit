package liftoff

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// LineFn receives one stdout/stderr line at a time from a streamed command.
type LineFn func(line string)

// Run executes name with args in dir. Returns combined output on error.
// Use this for short, non-streaming commands.
func Run(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	s := strings.TrimRight(string(out), "\n")
	if err != nil {
		return s, fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, s)
	}
	return s, nil
}

// RunStream executes name with args in dir and pipes each output line to onLine.
// Returns when the process exits. dir may be empty.
func RunStream(dir, name string, args []string, onLine LineFn) error {
	return runStream(dir, name, args, nil, onLine)
}

// RunStreamEnv is RunStream with an explicit process environment.
func RunStreamEnv(dir, name string, args []string, env []string, onLine LineFn) error {
	return runStream(dir, name, args, env, onLine)
}

func runStream(dir, name string, args []string, env []string, onLine LineFn) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var wg sync.WaitGroup
	pump := func(r io.Reader) {
		defer wg.Done()
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			if onLine != nil {
				onLine(s.Text())
			}
		}
	}
	wg.Add(2)
	go pump(stdout)
	go pump(stderr)
	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
