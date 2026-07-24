package liftoff

import "testing"

func TestCommandLineToolsFallbackForPendingXcodeLicense(t *testing.T) {
	var probes []string
	probe := func(developerDir string) (string, int) {
		probes = append(probes, developerDir)
		if developerDir == "" {
			return "You have not agreed to the Xcode license agreements.", 69
		}
		return "git version 2.50.1 (Apple Git-155)", 0
	}

	got := commandLineToolsFallback("darwin", "", probe)
	if got != commandLineToolsDir {
		t.Fatalf("fallback = %q, want %q", got, commandLineToolsDir)
	}
	if len(probes) != 2 || probes[0] != "" || probes[1] != commandLineToolsDir {
		t.Fatalf("probes = %#v, want current selection then Command Line Tools", probes)
	}
}

func TestCommandLineToolsFallbackDoesNotMaskOtherGitFailures(t *testing.T) {
	tests := []struct {
		name          string
		goos          string
		configuredDir string
		output        string
		exitCode      int
	}{
		{name: "working git", goos: "darwin", output: "git version 2.50.1", exitCode: 0},
		{name: "unrelated exit 69", goos: "darwin", output: "service unavailable", exitCode: 69},
		{name: "license message with another status", goos: "darwin", output: "Xcode license", exitCode: 1},
		{name: "non darwin", goos: "linux", output: "Xcode license", exitCode: 69},
		{name: "explicit developer dir", goos: "darwin", configuredDir: "/custom/Xcode", output: "Xcode license", exitCode: 69},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probes := 0
			got := commandLineToolsFallback(tt.goos, tt.configuredDir, func(string) (string, int) {
				probes++
				return tt.output, tt.exitCode
			})
			if got != "" {
				t.Fatalf("fallback = %q, want none", got)
			}
			if (tt.goos != "darwin" || tt.configuredDir != "") && probes != 0 {
				t.Fatalf("probe called %d times despite platform/config guard", probes)
			}
		})
	}
}

func TestCommandLineToolsFallbackRequiresWorkingFallback(t *testing.T) {
	probe := func(developerDir string) (string, int) {
		if developerDir == "" {
			return "Xcode license agreements have not been accepted", 69
		}
		return "xcrun: error: invalid active developer path", 1
	}
	if got := commandLineToolsFallback("darwin", "", probe); got != "" {
		t.Fatalf("fallback = %q, want none", got)
	}
}
