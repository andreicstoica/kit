package liftoff

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const commandLineToolsDir = "/Library/Developer/CommandLineTools"

// UseCommandLineToolsIfXcodeUnlicensed keeps Kit's subprocesses usable after
// Xcode is installed but before its license is accepted. In that state,
// Apple's /usr/bin/git exits 69 before doing any work. The standalone Command
// Line Tools remain usable when selected through DEVELOPER_DIR, so prefer them
// for this Kit process and every child it launches (git, gh, gt, etc.).
//
// An explicit DEVELOPER_DIR is never overridden. If the fallback is absent or
// unusable, this is a no-op and the original command error is left intact.
func UseCommandLineToolsIfXcodeUnlicensed() bool {
	if runtime.GOOS != "darwin" || os.Getenv("DEVELOPER_DIR") != "" {
		return false
	}
	if _, err := os.Stat(commandLineToolsDir); err != nil {
		return false
	}

	fallback := commandLineToolsFallback(runtime.GOOS, "", gitVersionProbe)
	if fallback == "" {
		return false
	}
	return os.Setenv("DEVELOPER_DIR", fallback) == nil
}

type developerToolsProbe func(developerDir string) (output string, exitCode int)

func commandLineToolsFallback(goos, configuredDir string, probe developerToolsProbe) string {
	if goos != "darwin" || configuredDir != "" {
		return ""
	}
	output, exitCode := probe("")
	if exitCode != 69 || !strings.Contains(strings.ToLower(output), "xcode license") {
		return ""
	}
	if _, fallbackExitCode := probe(commandLineToolsDir); fallbackExitCode != 0 {
		return ""
	}
	return commandLineToolsDir
}

func gitVersionProbe(developerDir string) (string, int) {
	cmd := exec.Command("git", "--version")
	if developerDir != "" {
		cmd.Env = append(os.Environ(), "DEVELOPER_DIR="+developerDir)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(out), exitErr.ExitCode()
	}
	return string(out), -1
}
