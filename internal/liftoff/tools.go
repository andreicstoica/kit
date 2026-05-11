package liftoff

import (
	"os"
	"os/exec"
	"strings"
)

// Homebrew install locations on macOS. Apple Silicon → /opt/homebrew,
// Intel → /usr/local. We probe both so doctor can distinguish
// "missing" from "installed but not on PATH".
var brewBinaryPaths = []string{
	"/opt/homebrew/bin/brew",
	"/usr/local/bin/brew",
}

// BrewState describes how brew is (or isn't) reachable.
type BrewState struct {
	OnPath    bool   // `brew` resolves on PATH
	BinaryAt  string // first known location where brew exists on disk; "" if none
	PrefixDir string // output of `brew --prefix`, eg /opt/homebrew. Only set when OnPath.
}

// DetectBrew checks both PATH and the canonical install locations.
func DetectBrew() BrewState {
	st := BrewState{}
	if p, err := exec.LookPath("brew"); err == nil {
		st.OnPath = true
		st.BinaryAt = p
		if out, err := exec.Command("brew", "--prefix").Output(); err == nil {
			st.PrefixDir = strings.TrimSpace(string(out))
		}
		return st
	}
	for _, p := range brewBinaryPaths {
		if _, err := os.Stat(p); err == nil {
			st.BinaryAt = p
			return st
		}
	}
	return st
}

// BrewShellenvLine returns the canonical shellenv eval line for a brew
// binary path. Used to fix "installed but not on PATH" cold-start cases.
func BrewShellenvLine(brewBin string) string {
	return `eval "$(` + brewBin + ` shellenv)"`
}

// BrewInstall runs `brew install [--cask] pkg` and streams output via onLine.
func BrewInstall(pkg string, cask bool, onLine LineFn) error {
	args := []string{"install"}
	if cask {
		args = append(args, "--cask")
	}
	args = append(args, pkg)
	return RunStream("", "brew", args, onLine)
}

// ToolVersion runs `name <args...>` and returns trimmed stdout.
// Returns "" + error on failure.
func ToolVersion(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
