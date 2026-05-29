package liftoff

import (
	"strings"
	"testing"
)

func TestSpecFor_App(t *testing.T) {
	p := PortsForSlot(1)
	spec := SpecFor("voice-agent", "/wt", SvcApp, p)
	if spec.Cwd != "/wt/frontend/app" {
		t.Errorf("Cwd = %q", spec.Cwd)
	}
	want := []string{"yarn", "dev", "--port", "3010", "--strictPort"}
	if len(spec.Argv) != len(want) {
		t.Fatalf("Argv = %v, want %v", spec.Argv, want)
	}
	for i := range want {
		if spec.Argv[i] != want[i] {
			t.Errorf("Argv[%d] = %q, want %q", i, spec.Argv[i], want[i])
		}
	}
	envBlob := strings.Join(spec.Env, " ")
	for _, want := range []string{
		"VITE_APP_API_URL=http://localhost:9010/api",
		"VITE_APP_BASE_URL=http://localhost:3010",
		"VITE_APP_SHORT_BASE_URL=localhost:3010",
	} {
		if !strings.Contains(envBlob, want) {
			t.Errorf("env missing %q\ngot: %s", want, envBlob)
		}
	}
}

func TestSpecFor_Admin(t *testing.T) {
	p := PortsForSlot(2)
	spec := SpecFor("notebook", "/wt", SvcAdmin, p)
	if spec.Cwd != "/wt/frontend/admin" {
		t.Errorf("Cwd = %q", spec.Cwd)
	}
	// --strictPort stops Vite from silently drifting to another port when the
	// assigned one is taken (which broke WaitForPort and hung kit play).
	if !strings.Contains(strings.Join(spec.Argv, " "), "--strictPort") {
		t.Errorf("admin Argv missing --strictPort: %v", spec.Argv)
	}
	envBlob := strings.Join(spec.Env, " ")
	for _, want := range []string{
		"VITE_APP_API_URL=http://localhost:9021/api",
		"VITE_APP_BASE_URL=http://localhost:3021",
		"VITE_APP_LIFTOFF_BASE_URL=http://localhost:3020",
	} {
		if !strings.Contains(envBlob, want) {
			t.Errorf("env missing %q\ngot: %s", want, envBlob)
		}
	}
}

func TestSpecFor_API_ShellWrap(t *testing.T) {
	spec := SpecFor("voice-agent", "/wt", SvcAPI, PortsForSlot(1))
	if spec.Cwd != "/wt/backend" {
		t.Errorf("Cwd = %q", spec.Cwd)
	}
	if len(spec.Argv) != 3 || spec.Argv[0] != "bash" || spec.Argv[1] != "-lc" {
		t.Fatalf("expected bash -lc wrapper, got %v", spec.Argv)
	}
	cmd := spec.Argv[2]
	for _, want := range []string{
		"uvicorn api.app:create_app",
		"--factory",
		"--host 127.0.0.1",
		"--port 9010",
		"--reload",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("cmd missing %q\nfull: %s", want, cmd)
		}
	}
}

func TestSpecFor_Celery(t *testing.T) {
	spec := SpecFor("voice-agent", "/wt", SvcCelery, PortsForSlot(1))
	if !strings.Contains(spec.Argv[2], "celery -A common.celery worker") {
		t.Errorf("celery cmd = %s", spec.Argv[2])
	}
}

func TestSpecFor_Beat_ScheduleOutsideRepo(t *testing.T) {
	spec := SpecFor("voice-agent", "/wt", SvcBeat, PortsForSlot(1))
	cmd := spec.Argv[2]
	if !strings.Contains(cmd, "celery -A common.celery beat") {
		t.Errorf("beat cmd = %s", cmd)
	}
	// The schedule shelve must be redirected into the run dir, never the
	// worktree's backend/ (where it would pollute git status).
	if !strings.Contains(cmd, "--schedule") {
		t.Errorf("beat cmd missing --schedule: %s", cmd)
	}
	if !strings.Contains(cmd, RunDirPath("voice-agent")) {
		t.Errorf("beat schedule not pointed at run dir: %s", cmd)
	}
	if strings.Contains(cmd, "/wt/backend/celerybeat-schedule") {
		t.Errorf("beat schedule still inside the worktree: %s", cmd)
	}
}

func TestServicePort(t *testing.T) {
	p := PortsForSlot(3)
	cases := []struct {
		svc  Service
		want int
	}{
		{SvcApp, 3030},
		{SvcAdmin, 3031},
		{SvcAPI, 9030},
		{SvcAdminBE, 9031},
		{SvcMCP, 9032},
		{SvcCelery, 0},
		{SvcBeat, 0},
	}
	for _, c := range cases {
		if got := ServicePort(c.svc, p); got != c.want {
			t.Errorf("ServicePort(%s) = %d, want %d", c.svc, got, c.want)
		}
	}
}
