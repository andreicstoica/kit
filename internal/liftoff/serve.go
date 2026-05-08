package liftoff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Service identifies one runnable component.
type Service string

const (
	SvcApp     Service = "app"
	SvcAdmin   Service = "admin"
	SvcAPI     Service = "api"
	SvcAdminBE Service = "admin_be"
	SvcMCP     Service = "mcp"
	SvcCelery  Service = "celery"
	SvcBeat    Service = "beat"
)

// AllServices in canonical start order.
var AllServices = []Service{SvcApp, SvcAdmin, SvcAPI, SvcAdminBE, SvcMCP, SvcCelery, SvcBeat}

// DefaultServices are the ones turned on by `kit play` defaults.
var DefaultServices = []Service{SvcApp, SvcAdmin, SvcAPI, SvcAdminBE, SvcCelery, SvcBeat}

// ServiceLabel returns a human-readable label.
func (s Service) Label() string {
	switch s {
	case SvcApp:
		return "frontend/app"
	case SvcAdmin:
		return "frontend/admin"
	case SvcAPI:
		return "backend/api"
	case SvcAdminBE:
		return "backend/admin"
	case SvcMCP:
		return "mcp_server"
	case SvcCelery:
		return "celery worker"
	case SvcBeat:
		return "celery beat"
	}
	return string(s)
}

// ServicePort returns the assigned port (0 if the service has no port).
func ServicePort(svc Service, p Ports) int {
	switch svc {
	case SvcApp:
		return p.App
	case SvcAdmin:
		return p.Admin
	case SvcAPI:
		return p.API
	case SvcAdminBE:
		return p.AdminBE
	case SvcMCP:
		return p.MCP
	}
	return 0
}

// LaunchSpec is everything needed to spawn a service.
type LaunchSpec struct {
	Worktree string
	Service  Service
	Cwd      string
	Argv     []string // command + args, ready for exec.Command
	Env      []string // additional env, merged with os.Environ
}

// pyVenv returns the configured python venv path.
func pyVenv() string {
	if v := os.Getenv("KIT_PY_VENV"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".envs", "py314")
}

// shellWrap wraps a command in `bash -lc 'source <venv>/bin/activate && exec <cmd>'`
// so backend services run inside the configured venv. Frontend commands skip wrapping.
func shellWrap(cmd string) []string {
	venv := pyVenv()
	full := fmt.Sprintf(`source %q/bin/activate >/dev/null 2>&1 || true; exec %s`, venv, cmd)
	return []string{"bash", "-lc", full}
}

// SpecFor returns the LaunchSpec for a (worktree, service, ports) triple.
func SpecFor(worktree, worktreePath string, svc Service, p Ports) LaunchSpec {
	port := ServicePort(svc, p)
	switch svc {
	case SvcApp:
		return LaunchSpec{
			Worktree: worktree,
			Service:  svc,
			Cwd:      filepath.Join(worktreePath, "frontend", "app"),
			Argv:     []string{"yarn", "dev", "--port", fmt.Sprint(port)},
			Env: []string{
				fmt.Sprintf("VITE_APP_API_URL=http://localhost:%d/api", p.API),
				fmt.Sprintf("VITE_APP_BASE_URL=http://localhost:%d", p.App),
				fmt.Sprintf("VITE_APP_SHORT_BASE_URL=localhost:%d", p.App),
			},
		}
	case SvcAdmin:
		return LaunchSpec{
			Worktree: worktree,
			Service:  svc,
			Cwd:      filepath.Join(worktreePath, "frontend", "admin"),
			Argv:     []string{"yarn", "dev", "--port", fmt.Sprint(port)},
			Env: []string{
				fmt.Sprintf("VITE_APP_API_URL=http://localhost:%d/api", p.AdminBE),
				fmt.Sprintf("VITE_APP_BASE_URL=http://localhost:%d", p.Admin),
				fmt.Sprintf("VITE_APP_LIFTOFF_BASE_URL=http://localhost:%d", p.App),
				fmt.Sprintf("VITE_APP_SHORT_BASE_URL=localhost:%d", p.Admin),
			},
		}
	case SvcAPI:
		return LaunchSpec{
			Worktree: worktree,
			Service:  svc,
			Cwd:      filepath.Join(worktreePath, "backend"),
			Argv: shellWrap(fmt.Sprintf(
				"uvicorn api.app:create_app --factory --host 127.0.0.1 --port %d --reload",
				port)),
		}
	case SvcAdminBE:
		return LaunchSpec{
			Worktree: worktree,
			Service:  svc,
			Cwd:      filepath.Join(worktreePath, "backend"),
			Argv: shellWrap(fmt.Sprintf(
				"uvicorn admin.admin_app:create_admin_app --factory --host 127.0.0.1 --port %d --reload",
				port)),
		}
	case SvcMCP:
		return LaunchSpec{
			Worktree: worktree,
			Service:  svc,
			Cwd:      filepath.Join(worktreePath, "backend"),
			Argv: shellWrap(fmt.Sprintf(
				"uvicorn mcp_server.app:create_app --factory --host 127.0.0.1 --port %d --reload",
				port)),
		}
	case SvcCelery:
		return LaunchSpec{
			Worktree: worktree,
			Service:  svc,
			Cwd:      filepath.Join(worktreePath, "backend"),
			Argv:     shellWrap("celery -A common.celery worker --loglevel=INFO"),
		}
	case SvcBeat:
		return LaunchSpec{
			Worktree: worktree,
			Service:  svc,
			Cwd:      filepath.Join(worktreePath, "backend"),
			Argv:     shellWrap("celery -A common.celery beat --loglevel=INFO"),
		}
	}
	return LaunchSpec{}
}

// StartService spawns the service detached, writes pid+log+cmd files,
// and returns the new PID. Does not wait for readiness; use WaitForPort/WaitForPID.
func StartService(spec LaunchSpec) (int, error) {
	logPath, err := LogFile(spec.Worktree, string(spec.Service))
	if err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	// logFile is closed after we hand it to the child.
	defer logFile.Close()

	cmdPath, err := CmdFile(spec.Worktree, string(spec.Service))
	if err == nil {
		body := fmt.Sprintf("cwd: %s\nargv: %s\nenv: %s\nstarted: %s\n",
			spec.Cwd, strings.Join(spec.Argv, " "),
			strings.Join(spec.Env, " "), time.Now().UTC().Format(time.RFC3339))
		_ = os.WriteFile(cmdPath, []byte(body), 0o644)
	}

	cmd := exec.Command(spec.Argv[0], spec.Argv[1:]...)
	cmd.Dir = spec.Cwd
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(), spec.Env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start %s: %w", spec.Service, err)
	}
	pid := cmd.Process.Pid
	if err := WritePID(spec.Worktree, string(spec.Service), pid); err != nil {
		_ = KillGroup(pid)
		return 0, err
	}
	// Detach child from this Go process — we don't want to reap it.
	go func() { _ = cmd.Wait() }()
	return pid, nil
}

// StopService kills the running service and removes its pid file.
// No-op if the pid file is missing or the process is already gone.
func StopService(worktree string, svc Service) error {
	pid := ReadPID(worktree, string(svc))
	if pid == 0 {
		return nil
	}
	if IsAlive(pid) {
		if err := KillGroup(pid); err != nil {
			return err
		}
	}
	return RemovePID(worktree, string(svc))
}

// ServiceStatus describes a service's current state.
type ServiceStatus struct {
	Service Service
	PID     int
	Alive   bool
	Port    int
	Listening bool
}

// StatusOf returns the live status for a service.
func StatusOf(worktree string, svc Service, p Ports) ServiceStatus {
	pid := ReadPID(worktree, string(svc))
	alive := IsAlive(pid)
	port := ServicePort(svc, p)
	listening := false
	if alive && port > 0 {
		listening = PortListening(port)
	}
	return ServiceStatus{
		Service:   svc,
		PID:       pid,
		Alive:     alive,
		Port:      port,
		Listening: listening,
	}
}

// FindCeleryOwner scans every worktree's celery.pid and returns the worktree
// name + pid of the live celery, if any. Returns "" if nothing is running.
func FindCeleryOwner() (owner string, pid int) {
	st, err := LoadState()
	if err != nil {
		return "", 0
	}
	for name := range st.Worktrees {
		p := ReadPID(name, string(SvcCelery))
		if p > 0 && IsAlive(p) {
			return name, p
		}
	}
	// Also check any run dir not in state (orphaned).
	home, _ := os.UserHomeDir()
	runRoot := filepath.Join(home, ".config", "kit", "run")
	if v := os.Getenv("KIT_RUN_DIR"); v != "" {
		runRoot = v
	}
	entries, err := os.ReadDir(runRoot)
	if err != nil {
		return "", 0
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := ReadPID(e.Name(), string(SvcCelery))
		if p > 0 && IsAlive(p) {
			return e.Name(), p
		}
	}
	return "", 0
}
