package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

const launchTestBrief = "Implement the login redirect fix. marker-7f3a"

type launchCapture struct {
	called bool
	name   string
	argv   []string
}

func runAgentLaunch(t *testing.T, stdin string, result liftoff.RexAgentLaunch, launchErr error, args ...string) (*launchCapture, string, string, error) {
	t.Helper()
	capture := &launchCapture{}
	launch := func(_ liftoff.Layout, name string, argv []string) (liftoff.RexAgentLaunch, error) {
		capture.called, capture.name, capture.argv = true, name, argv
		return result, launchErr
	}
	lookPath := func(file string) (string, error) {
		return "/opt/test/bin/" + file, nil
	}
	cmd := newAgentLaunchCommand(launch, lookPath)
	var stdout, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return capture, stdout.String(), stderr.String(), err
}

func writeBrief(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "brief.md")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func launchedResult() liftoff.RexAgentLaunch {
	return liftoff.RexAgentLaunch{Name: "login-fix", Path: "/w/login-fix", SessionID: "session:1", WindowID: "window:2"}
}

func TestAgentLaunchRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"agent", "launch"})
	if err != nil || cmd.CommandPath() != "kit agent launch" {
		t.Fatalf("agent launch not discoverable: %v, %v", cmd, err)
	}
}

func TestAgentLaunchPassesExactArgvWithoutPrintingBrief(t *testing.T) {
	path := writeBrief(t, "\n"+launchTestBrief+"\n\n")
	capture, stdout, stderr, err := runAgentLaunch(t, "", launchedResult(), nil, "login-fix", "--brief", path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/opt/test/bin/claude", "--model", "opus", launchTestBrief}
	if capture.name != "login-fix" || strings.Join(capture.argv, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("launch(%q, %q)", capture.name, capture.argv)
	}
	for _, arg := range capture.argv {
		if strings.Contains(arg, "dangerously") || strings.Contains(arg, "permission") {
			t.Fatalf("argv contains a permission flag: %q", capture.argv)
		}
	}
	if strings.Contains(stdout+stderr, "marker-7f3a") {
		t.Fatalf("output leaks the brief: %q %q", stdout, stderr)
	}
	if !strings.Contains(stdout, "session:1") || !strings.Contains(stdout, "window:2") || stderr != "" {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestAgentLaunchReadsBriefFromStdin(t *testing.T) {
	capture, _, _, err := runAgentLaunch(t, launchTestBrief, launchedResult(), nil, "login-fix", "--brief", "-")
	if err != nil {
		t.Fatal(err)
	}
	if got := capture.argv[len(capture.argv)-1]; got != launchTestBrief {
		t.Fatalf("brief = %q", got)
	}
}

func TestAgentLaunchRejectsUnsafeBriefsBeforeLaunch(t *testing.T) {
	cases := map[string]string{
		"empty":         " \n\t",
		"leading dash":  "--dangerously-skip-permissions do it",
		"nul":           "fix\x00it",
		"escape":        "fix \x1b]0;title\x07 it",
		"c1 control":    "fix \u009b it",
		"invalid utf-8": "fix \xff it",
		"too large":     strings.Repeat("a", agentBriefLimit+1),
	}
	for name, brief := range cases {
		t.Run(name, func(t *testing.T) {
			capture, stdout, stderr, err := runAgentLaunch(t, "", launchedResult(), nil, "login-fix", "--brief", writeBrief(t, brief))
			if err == nil || capture.called {
				t.Fatalf("expected rejection before launch: err=%v called=%v", err, capture.called)
			}
			if name != "empty" && strings.Contains(err.Error()+stdout+stderr, brief) {
				t.Fatalf("error echoes the brief: %v", err)
			}
		})
	}
	t.Run("directory", func(t *testing.T) {
		capture, _, _, err := runAgentLaunch(t, "", launchedResult(), nil, "login-fix", "--brief", t.TempDir())
		if err == nil || capture.called {
			t.Fatalf("expected rejection: %v", err)
		}
	})
}

func TestAgentLaunchRejectsInvalidOptions(t *testing.T) {
	path := writeBrief(t, launchTestBrief)
	for _, args := range [][]string{
		{"--brief", path},
		{"a", "b", "--brief", path},
		{"login-fix", "--harness", "sh"},
		{"login-fix", "--harness", "opencode", "--model", "example"},
		{"login-fix", "--model=--help"},
		{"login-fix", "--model="},
		{"login-fix", "--prompt="},
		{"login-fix", "--prompt", strings.Repeat("a", agentBriefLimit+1)},
		{"login-fix", "--brief", path, "--prompt", launchTestBrief},
		{"login-fix", "--brief", ""},
	} {
		capture, _, _, err := runAgentLaunch(t, "", launchedResult(), nil, args...)
		if err == nil || capture.called {
			t.Fatalf("args %q: expected error before launch, got %v", args, err)
		}
	}
}

func TestAgentLaunchFocusFailureExitsZeroWithWarning(t *testing.T) {
	result := launchedResult()
	result.FocusErr = errors.New("open the Rex app")
	_, stdout, stderr, err := runAgentLaunch(t, "", result, nil, "login-fix", "--brief", writeBrief(t, launchTestBrief))
	if err != nil {
		t.Fatalf("a launched agent must not fail the command: %v", err)
	}
	if !strings.Contains(stdout, "Launched") || !strings.Contains(stderr, "check it before you launch again") {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}

	_, stdout, _, err = runAgentLaunch(t, "", result, nil, "login-fix", "--brief", writeBrief(t, launchTestBrief), "--json")
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("json: %v: %q", err, stdout)
	}
	if payload["launched"] != true || payload["focused"] != false || payload["focus_error"] != "open the Rex app" || payload["window_id"] != "window:2" {
		t.Fatalf("payload = %v", payload)
	}
}

func TestAgentLaunchErrorIsReturnedUnchanged(t *testing.T) {
	_, stdout, _, err := runAgentLaunch(t, "", liftoff.RexAgentLaunch{}, errors.New("feature is parked"), "login-fix", "--brief", writeBrief(t, launchTestBrief))
	if err == nil || err.Error() != "feature is parked" || stdout != "" {
		t.Fatalf("err=%v stdout=%q", err, stdout)
	}
}

func TestAgentLaunchSelectsHarnessModelAndFirstMessage(t *testing.T) {
	for _, tc := range []struct {
		name           string
		flags, argv    []string
		harness, model string
	}{
		{"Claude default", nil, []string{"claude", "--model", "opus"}, "claude", "opus"},
		{"Claude override", []string{"--model", "sonnet", "--prompt", launchTestBrief}, []string{"claude", "--model", "sonnet", launchTestBrief}, "claude", "sonnet"},
		{"fx override", []string{"--harness", "fx", "--model", "openai/gpt-5.4", "--prompt", launchTestBrief}, []string{"fx", "--model", "openai/gpt-5.4", launchTestBrief}, "fx", "openai/gpt-5.4"},
		{"Codex configured", []string{"--harness", "codex"}, []string{"codex"}, "codex", ""},
		{"Codex override", []string{"--harness", "codex", "--model", "gpt-5.4", "--brief", "-"}, []string{"codex", "--model", "gpt-5.4", launchTestBrief}, "codex", "gpt-5.4"},
		{"OpenCode prompt", []string{"--harness", "opencode", "--prompt", launchTestBrief}, []string{"opencode", "--prompt", launchTestBrief}, "opencode", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture, stdout, _, err := runAgentLaunch(t, launchTestBrief, launchedResult(), nil, append([]string{"login-fix", "--json"}, tc.flags...)...)
			if err != nil {
				t.Fatal(err)
			}
			want := append([]string{"/opt/test/bin/" + tc.argv[0]}, tc.argv[1:]...)
			if !slices.Equal(capture.argv, want) {
				t.Fatalf("argv = %q, want %q", capture.argv, want)
			}
			var report map[string]any
			if err := json.Unmarshal([]byte(stdout), &report); err != nil {
				t.Fatal(err)
			}
			if report["harness"] != tc.harness || report["model"] != tc.model || strings.Contains(stdout, launchTestBrief) {
				t.Fatalf("unexpected report: %s", stdout)
			}
		})
	}
}
