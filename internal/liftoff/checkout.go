package liftoff

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func validateCheckoutBackendName(backend string) error {
	switch backend {
	case "", "git", "twig":
		return nil
	default:
		return fmt.Errorf("unknown checkout backend %q; use git or twig", backend)
	}
}

// ValidateCheckoutBackend checks opt-in tooling before setup can create resources.
func ValidateCheckoutBackend(backend string) error {
	if err := validateCheckoutBackendName(backend); err != nil {
		return err
	}
	if backend == "twig" {
		out, err := Run("", "git", "twig", "--version")
		if err != nil {
			return fmt.Errorf("Twig >=0.4.0 is required; install it with `brew tap leostera/git-twig https://github.com/leostera/git-twig` then `brew install leostera/git-twig/git-twig`: %w", err)
		}
		parts := strings.Split(strings.TrimPrefix(out, "git-twig "), ".")
		if strings.HasPrefix(out, "git-twig ") && len(parts) == 3 {
			major, majorErr := strconv.Atoi(parts[0])
			minor, minorErr := strconv.Atoi(parts[1])
			patch, patchErr := strconv.Atoi(parts[2])
			if majorErr == nil && minorErr == nil && patchErr == nil &&
				major >= 0 && minor >= 0 && patch >= 0 && (major > 0 || minor >= 4) {
				return nil
			}
		}
		return fmt.Errorf("Twig >=0.4.0 (released version) is required; found %q", out)
	}
	return nil
}

type twigCreateError struct {
	cause   error
	partial bool
}

func (e *twigCreateError) Error() string { return e.cause.Error() }
func (e *twigCreateError) Unwrap() error { return e.cause }

func (l Layout) AddWorktreeWithBackend(name, path, backend string, onLine LineFn) error {
	if backend == "" || backend == "git" {
		return l.AddWorktree(name, path, onLine)
	}
	if err := ValidateCheckoutBackend(backend); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("worktree path already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	// A failed create must never claim a branch that predates this attempt.
	if _, err := Run(l.Master, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+name); err == nil {
		return fmt.Errorf("branch already exists: %s", name)
	} else {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return fmt.Errorf("check new branch: %w", err)
		}
	}
	// Workaround: Twig 0.4.0 mistakes new tracking config for a policy change.
	// Create detached, then let Git apply tracking. Remove when the minimum
	// Twig version includes the upstream tracking-policy fix.
	args := []string{"-C", l.Master, "twig", "new", "--detach", path, "origin/" + l.MainBranch}
	var lines []string
	emit := func(line string) {
		lines = append(lines, line)
		if len(lines) > 8 {
			lines = lines[len(lines)-8:]
		}
		if onLine != nil {
			onLine(line)
		}
	}
	err := RunStreamEnv("", "git", args, append(os.Environ(), "TWIG_TRACE=1"), emit)
	if err == nil {
		err = RunStream(path, "git", []string{"switch", "-c", name, "origin/" + l.MainBranch}, emit)
	}
	if err == nil {
		return nil
	}
	_, pathErr := os.Lstat(path)
	_, branchErr := Run(l.Master, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	partial := !os.IsNotExist(pathErr) || branchErr == nil
	if wts, listErr := l.ListWorktrees(); listErr == nil {
		_, registered := NewWorktreeSet(wts).At(path)
		partial = partial || registered
	}
	staging := filepath.Join(l.Master, ".git", "twigs", "staging")
	if common, commonErr := Run(l.Master, "git", "rev-parse", "--path-format=absolute", "--git-common-dir"); commonErr == nil {
		staging = filepath.Join(common, "twigs", "staging")
	}
	return &twigCreateError{
		cause: fmt.Errorf("Twig creation failed: %w\n%s\nInspect `git worktree list`, %s and retained staging at %s; do not retry or delete partial state automatically",
			err, strings.Join(lines, "\n"), path, staging),
		partial: partial,
	}
}
