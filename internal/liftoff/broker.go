package liftoff

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CeleryPinnedQueues mirrors the queues Liftoff's prod ECS workers consume
// (aws/ecs/*-worker.json) and every task pinned with queue=... in the Liftoff
// repo. A worker consumes only the queues it is told to, so when Liftoff adds
// a pinned queue, add it here too or its tasks pile up unconsumed locally.
const CeleryPinnedQueues = "celery,linkedin_import_only,interactive,signals_llm,signals_regen"

// localBroker is Liftoff's default CELERY_BROKER_URL host and the RabbitMQ
// every worktree shares.
const localBroker = "pyamqp://guest@localhost:5672"

// CeleryDefaultQueue is the queue Liftoff's common/celery.py routes unpinned
// tasks to in dev: "dev." + the repo root's directory name, after resolving
// symlinks like Python's Path.resolve().
func CeleryDefaultQueue(worktreePath string) string {
	p := worktreePath
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return "dev." + filepath.Base(filepath.Clean(p))
}

// CeleryQueues is the -Q list for a worktree's worker: its default queue plus
// every pinned queue.
func CeleryQueues(worktreePath string) string {
	return CeleryDefaultQueue(worktreePath) + "," + CeleryPinnedQueues
}

// BrokerVHost is the RabbitMQ vhost that isolates one worktree's tasks:
// "kit-" + the kit name, restricted to characters that need no URL escaping.
func BrokerVHost(worktree string) string {
	var b strings.Builder
	for _, r := range worktree {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return "kit-" + b.String()
}

// BrokerURL is the CELERY_BROKER_URL for a vhost on the shared local RabbitMQ.
func BrokerURL(vhost string) string {
	return localBroker + "/" + vhost
}

// CeleryBroker is the broker setup kit applies to a worktree's backend
// services. A zero value means kit leaves the broker alone (the user's own
// CELERY_BROKER_URL, or the shared default vhost when kit cannot isolate).
type CeleryBroker struct {
	VHost string
}

// Isolated reports whether kit routes this worktree to its own vhost.
func (b CeleryBroker) Isolated() bool { return b.VHost != "" }

// URL is the CELERY_BROKER_URL kit sets, or "" when it sets none.
func (b CeleryBroker) URL() string {
	if !b.Isolated() {
		return ""
	}
	return BrokerURL(b.VHost)
}

// Env is what kit adds to a backend service's environment. Liftoff's pydantic
// settings give env vars priority over backend/.env.
func (b CeleryBroker) Env() []string {
	if !b.Isolated() {
		return nil
	}
	// scripts/dev/pull-env.py writes CELERY_TASK_ALWAYS_EAGER=true, and eager
	// tasks enqueued from async request handlers are skipped, not run. Off
	// even with no worker up: queued tasks run once the worker starts.
	return []string{"CELERY_BROKER_URL=" + b.URL(), "CELERY_TASK_ALWAYS_EAGER=false"}
}

// IsBackend reports whether the service runs Liftoff's Python code and so
// talks to the Celery broker.
func (s Service) IsBackend() bool {
	switch s {
	case SvcAPI, SvcAdminBE, SvcMCP, SvcCelery, SvcBeat:
		return true
	}
	return false
}

// PrepareCeleryBroker decides the broker for a worktree and, when kit
// isolates it, makes sure the vhost exists. note is a one-line summary for
// the user; warn is true when kit could not isolate the worktree.
func PrepareCeleryBroker(worktree, worktreePath string) (b CeleryBroker, note string, warn bool) {
	b, note, managed := PlanCeleryBroker(worktree, worktreePath)
	if !managed {
		return b, note, false
	}
	if err := ensureVHost(b.VHost); err != nil {
		return CeleryBroker{}, "celery broker: not isolated by kit, tasks may leak across worktrees (" + err.Error() + ")", true
	}
	return b, note, false
}

// PlanCeleryBroker is PrepareCeleryBroker without side effects. managed is
// true when kit would run the worktree on its own vhost, which still has to be
// created with ensureVHost. A user-set broker returns the zero CeleryBroker.
func PlanCeleryBroker(worktree, worktreePath string) (b CeleryBroker, note string, managed bool) {
	if raw, ok := os.LookupEnv("CELERY_BROKER_URL"); ok && !kitMayReplaceBroker(raw) {
		return CeleryBroker{}, "celery broker: CELERY_BROKER_URL from your shell (not isolated by kit)", false
	}
	if brokerOverriddenInEnvFile(filepath.Join(worktreePath, "backend", ".env")) {
		return CeleryBroker{}, "celery broker: CELERY_BROKER_URL from backend/.env (not isolated by kit)", false
	}
	vhost := BrokerVHost(worktree)
	return CeleryBroker{VHost: vhost}, "celery broker: vhost " + vhost, true
}

// WorkerSharesBroker reports whether a worker started for this worktree would
// share a broker with other worktrees' workers. It is true only when kit will
// not give the worktree its own vhost: a broker set by the user, or no
// rabbitmqctl to create the vhost. Isolated workers cannot consume each
// other's tasks, so they may run side by side.
func WorkerSharesBroker(worktreePath string) bool {
	if raw, ok := os.LookupEnv("CELERY_BROKER_URL"); ok && !kitMayReplaceBroker(raw) {
		return true
	}
	if brokerOverriddenInEnvFile(filepath.Join(worktreePath, "backend", ".env")) {
		return true
	}
	return findBinary("rabbitmqctl", "/opt/homebrew/sbin/rabbitmqctl", "/usr/local/sbin/rabbitmqctl") == ""
}

// kitMayReplaceBroker reports whether raw is a broker kit may swap for a
// per-worktree vhost: unset, Liftoff's default local RabbitMQ vhost, or the
// memory:// placeholder scripts/dev/pull-env.py writes (no worker can reach
// it). Anything else (remote host, explicit vhost, unparseable) is the user's
// choice and is left alone.
func kitMayReplaceBroker(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "memory":
		return true
	case "amqp", "pyamqp":
	default:
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1", "":
	default:
		return false
	}
	vhost := strings.TrimPrefix(u.Path, "/")
	return vhost == "" || vhost == "/"
}

// brokerOverriddenInEnvFile reports whether the dotenv file sets
// CELERY_BROKER_URL to a broker kit must not replace. It reads only that key
// and never returns its value, so no secret in the file is surfaced.
func brokerOverriddenInEnvFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	raw, found := "", false
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	for s.Scan() {
		if v, ok := dotenvValue(s.Text(), "CELERY_BROKER_URL"); ok {
			raw, found = v, true // last assignment wins, as in python-dotenv
		}
	}
	return found && !kitMayReplaceBroker(raw)
}

// dotenvValue returns the value of key if line assigns it. Matches
// pydantic-settings' case-insensitive keys and python-dotenv's optional
// "export " prefix, quotes and trailing " #" comments.
func dotenvValue(line, key string) (string, bool) {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "export ")
	k, v, ok := strings.Cut(line, "=")
	if !ok || !strings.EqualFold(strings.TrimSpace(k), key) {
		return "", false
	}
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
			return v[1 : end+1], true
		}
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v, true
}

var errNoRabbitmqctl = errors.New("rabbitmqctl not found")

// rabbitmqctl runs one rabbitmqctl command. Swapped in tests.
var rabbitmqctl = func(args ...string) ([]byte, error) {
	bin := findBinary("rabbitmqctl", "/opt/homebrew/sbin/rabbitmqctl", "/usr/local/sbin/rabbitmqctl")
	if bin == "" {
		return nil, errNoRabbitmqctl
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, bin, args...).CombinedOutput()
}

// ensureVHost creates the vhost and grants guest full access. Both steps are
// idempotent; older RabbitMQ versions report an existing vhost as an error.
func ensureVHost(vhost string) error {
	out, err := rabbitmqctl("add_vhost", vhost)
	if err != nil && !strings.Contains(strings.ToLower(string(out)), "already") {
		return rabbitmqctlErr("add_vhost", out, err)
	}
	if out, err := rabbitmqctl("set_permissions", "-p", vhost, "guest", ".*", ".*", ".*"); err != nil {
		return rabbitmqctlErr("set_permissions", out, err)
	}
	return nil
}

func rabbitmqctlErr(op string, out []byte, err error) error {
	if errors.Is(err, errNoRabbitmqctl) {
		return err
	}
	msg := strings.TrimSpace(string(out))
	if i := strings.LastIndexByte(msg, '\n'); i >= 0 {
		msg = msg[i+1:]
	}
	if msg == "" {
		return fmt.Errorf("rabbitmqctl %s: %w", op, err)
	}
	return fmt.Errorf("rabbitmqctl %s: %s", op, msg)
}

// RecordedBroker returns the CELERY_BROKER_URL kit passed to the service's
// last launch, from its .cmd record, or "" if kit did not set one.
func RecordedBroker(worktree string, svc Service) string {
	path, err := CmdFile(worktree, string(svc))
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		env, ok := strings.CutPrefix(line, "env: ")
		if !ok {
			continue
		}
		for _, kv := range strings.Fields(env) {
			if v, ok := strings.CutPrefix(kv, "CELERY_BROKER_URL="); ok {
				return v
			}
		}
	}
	return ""
}

// ServicesOnOtherBroker returns the running backend services whose last launch
// used a different broker than b. Until they restart they publish to or
// consume from the old broker, so tasks cross between the two and stall.
func ServicesOnOtherBroker(worktree string, ports Ports, b CeleryBroker) []Service {
	var out []Service
	for _, svc := range AllServices {
		if svc.IsBackend() && serviceUp(worktree, svc, ports) && RecordedBroker(worktree, svc) != b.URL() {
			out = append(out, svc)
		}
	}
	return out
}
