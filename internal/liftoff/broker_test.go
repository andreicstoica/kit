package liftoff

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

// stubRabbitmqctl records rabbitmqctl calls and answers each with out/err.
func stubRabbitmqctl(t *testing.T, out string, err error) *[][]string {
	t.Helper()
	var calls [][]string
	orig := rabbitmqctl
	rabbitmqctl = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		return []byte(out), err
	}
	t.Cleanup(func() { rabbitmqctl = orig })
	return &calls
}

// writeBackendEnv writes <dir>/backend/.env and returns dir.
func writeBackendEnv(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "backend", ".env"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBrokerVHost(t *testing.T) {
	cases := map[string]string{
		"google":            "kit-google",
		"voice-agent":       "kit-voice-agent",
		"feat/x y":          "kit-feat-x-y",
		"Mixed_Case.v2":     "kit-Mixed_Case.v2",
		"emoji-🚀":           "kit-emoji--",
		"slash/../traverse": "kit-slash-..-traverse",
	}
	for in, want := range cases {
		if got := BrokerVHost(in); got != want {
			t.Errorf("BrokerVHost(%q) = %q, want %q", in, got, want)
		}
	}
	if got := BrokerURL("kit-google"); got != "pyamqp://guest@localhost:5672/kit-google" {
		t.Errorf("BrokerURL = %q", got)
	}
}

// The worker's default queue must match Liftoff's
// Path(__file__).resolve().parents[2].name, which follows symlinks.
func TestCeleryDefaultQueue(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "google")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if got := CeleryDefaultQueue(real + "/"); got != "dev.google" {
		t.Errorf("CeleryDefaultQueue(real) = %q", got)
	}
	if got := CeleryDefaultQueue(link); got != "dev.google" {
		t.Errorf("CeleryDefaultQueue(symlink) = %q, want dev.google", got)
	}
	if got := CeleryDefaultQueue("/nonexistent/liftoff-app-master"); got != "dev.liftoff-app-master" {
		t.Errorf("CeleryDefaultQueue(missing) = %q", got)
	}
	want := "dev.google,celery,linkedin_import_only,interactive,signals_llm,signals_regen"
	if got := CeleryQueues(real); got != want {
		t.Errorf("CeleryQueues = %q, want %q", got, want)
	}
}

func TestCeleryBrokerEnv(t *testing.T) {
	if env := (CeleryBroker{}).Env(); env != nil {
		t.Errorf("zero broker env = %v, want none", env)
	}
	if url := (CeleryBroker{}).URL(); url != "" {
		t.Errorf("zero broker URL = %q, want empty", url)
	}
	got := (CeleryBroker{VHost: "kit-google"}).Env()
	want := []string{
		"CELERY_BROKER_URL=pyamqp://guest@localhost:5672/kit-google",
		"CELERY_TASK_ALWAYS_EAGER=false",
	}
	if !slices.Equal(got, want) {
		t.Errorf("env = %v, want %v", got, want)
	}
}

func TestKitMayReplaceBroker(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"  ", true},
		{"memory://", true},
		{"pyamqp://guest@localhost:5672", true},
		{"pyamqp://guest@localhost:5672/", true},
		{"amqp://guest:guest@127.0.0.1:5672//", true},
		{"amqp://guest@[::1]:5672", true},
		{"pyamqp://guest@localhost:5672/kit-google", false},
		{"amqps://user:pw@mq.example.com:5671", false},
		{"redis://localhost:6379/0", false},
		{"not a url", false},
	}
	for _, c := range cases {
		if got := kitMayReplaceBroker(c.in); got != c.want {
			t.Errorf("kitMayReplaceBroker(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestDotenvValue(t *testing.T) {
	cases := []struct {
		line   string
		want   string
		wantOK bool
	}{
		{"CELERY_BROKER_URL=amqp://h", "amqp://h", true},
		{"  export CELERY_BROKER_URL = amqp://h  ", "amqp://h", true},
		{`CELERY_BROKER_URL="amqp://h#x" # note`, "amqp://h#x", true},
		{"celery_broker_url='amqp://h'", "amqp://h", true},
		{"CELERY_BROKER_URL=amqp://h # note", "amqp://h", true},
		{"# CELERY_BROKER_URL=amqp://h", "", false},
		{"CELERY_BROKER_URL_OLD=amqp://h", "", false},
		{"OTHER=1", "", false},
	}
	for _, c := range cases {
		got, ok := dotenvValue(c.line, "CELERY_BROKER_URL")
		if ok != c.wantOK || got != c.want {
			t.Errorf("dotenvValue(%q) = (%q,%v), want (%q,%v)", c.line, got, ok, c.want, c.wantOK)
		}
	}
}

func TestBrokerOverriddenInEnvFile(t *testing.T) {
	cases := []struct {
		name, body string
		want       bool
	}{
		{"no key", "SECRET=x\nCELERY_TASK_ALWAYS_EAGER=true\n", false},
		{"pull-env placeholder", "CELERY_BROKER_URL=memory://\nCELERY_TASK_ALWAYS_EAGER=true\n", false},
		{"default local", "CELERY_BROKER_URL=pyamqp://guest@localhost:5672\n", false},
		{"remote", "CELERY_BROKER_URL=amqps://u:p@mq.example.com:5671\n", true},
		{"local custom vhost", "CELERY_BROKER_URL=amqp://guest@localhost:5672/mine\n", true},
		{"last wins", "CELERY_BROKER_URL=amqps://u:p@mq.example.com\nCELERY_BROKER_URL=amqp://localhost\n", false},
		{"empty", "CELERY_BROKER_URL=\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := writeBackendEnv(t, c.body)
			if got := brokerOverriddenInEnvFile(filepath.Join(dir, "backend", ".env")); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
	if brokerOverriddenInEnvFile(filepath.Join(t.TempDir(), "missing")) {
		t.Error("missing .env must not count as an override")
	}
}

func TestPrepareCeleryBroker_CreatesVHost(t *testing.T) {
	t.Setenv("CELERY_BROKER_URL", "")
	calls := stubRabbitmqctl(t, "", nil)
	dir := writeBackendEnv(t, "CELERY_BROKER_URL=pyamqp://guest@localhost:5672\n")

	b, note, warn := PrepareCeleryBroker("google", dir)
	if warn || b.VHost != "kit-google" {
		t.Fatalf("broker = %+v warn=%v", b, warn)
	}
	if !strings.Contains(note, "kit-google") {
		t.Errorf("note = %q", note)
	}
	want := [][]string{
		{"add_vhost", "kit-google"},
		{"set_permissions", "-p", "kit-google", "guest", ".*", ".*", ".*"},
	}
	if len(*calls) != len(want) {
		t.Fatalf("calls = %v", *calls)
	}
	for i := range want {
		if !slices.Equal((*calls)[i], want[i]) {
			t.Errorf("call %d = %v, want %v", i, (*calls)[i], want[i])
		}
	}
}

func TestEnsureVHost_AlreadyExistsIsFine(t *testing.T) {
	orig := rabbitmqctl
	t.Cleanup(func() { rabbitmqctl = orig })
	var ops []string
	rabbitmqctl = func(args ...string) ([]byte, error) {
		ops = append(ops, args[0])
		if args[0] == "add_vhost" {
			return []byte("Error:\nvhost_already_exists: kit-google"), errors.New("exit status 70")
		}
		return nil, nil
	}
	if err := ensureVHost("kit-google"); err != nil {
		t.Fatalf("ensureVHost = %v, want nil for an existing vhost", err)
	}
	if !slices.Equal(ops, []string{"add_vhost", "set_permissions"}) {
		t.Errorf("ops = %v", ops)
	}

	rabbitmqctl = func(args ...string) ([]byte, error) {
		return []byte("Error: unable to perform an operation on node 'rabbit@a5'"), errors.New("exit status 69")
	}
	err := ensureVHost("kit-google")
	if err == nil || !strings.Contains(err.Error(), "unable to perform an operation") {
		t.Errorf("node down: err = %v", err)
	}
}

func TestPrepareCeleryBroker_FallsBackWithoutRabbitmqctl(t *testing.T) {
	t.Setenv("CELERY_BROKER_URL", "")
	stubRabbitmqctl(t, "", errNoRabbitmqctl)
	b, note, warn := PrepareCeleryBroker("google", t.TempDir())
	if b.Isolated() || !warn {
		t.Fatalf("want fallback, got %+v warn=%v", b, warn)
	}
	if b.Env() != nil {
		t.Errorf("fallback must inject nothing, got %v", b.Env())
	}
	if !strings.Contains(note, "rabbitmqctl not found") {
		t.Errorf("note = %q", note)
	}
}

func TestPrepareCeleryBroker_RespectsOverrides(t *testing.T) {
	calls := stubRabbitmqctl(t, "", nil)

	t.Setenv("CELERY_BROKER_URL", "")
	dir := writeBackendEnv(t, "CELERY_BROKER_URL=amqps://u:p@mq.example.com:5671\n")
	b, note, warn := PrepareCeleryBroker("google", dir)
	if b.Isolated() || warn || !strings.Contains(note, "backend/.env") {
		t.Errorf(".env override: broker=%+v note=%q warn=%v", b, note, warn)
	}
	if strings.Contains(note, "mq.example.com") || strings.Contains(note, "u:p") {
		t.Errorf("note leaks the .env value: %q", note)
	}

	t.Setenv("CELERY_BROKER_URL", "amqp://guest@localhost:5672/mine")
	b, note, _ = PrepareCeleryBroker("google", t.TempDir())
	if b.Isolated() || !strings.Contains(note, "shell") {
		t.Errorf("shell override: broker=%+v note=%q", b, note)
	}
	if len(*calls) != 0 {
		t.Errorf("overrides must not touch rabbitmqctl, got %v", *calls)
	}
}

func TestSpecFor_Broker(t *testing.T) {
	b := CeleryBroker{VHost: "kit-google"}
	p := PortsForSlot(1)

	worker := SpecFor("google", "/wt/google", SvcCelery, p, b)
	if !strings.HasSuffix(worker.Argv[2],
		"celery -A common.celery worker --loglevel=INFO -Q dev.google,celery,linkedin_import_only,interactive,signals_llm,signals_regen") {
		t.Errorf("worker argv = %q", worker.Argv[2])
	}
	for _, svc := range []Service{SvcAPI, SvcAdminBE, SvcMCP, SvcCelery, SvcBeat} {
		env := strings.Join(SpecFor("google", "/wt/google", svc, p, b).Env, " ")
		if !strings.Contains(env, "CELERY_BROKER_URL=pyamqp://guest@localhost:5672/kit-google") ||
			!strings.Contains(env, "CELERY_TASK_ALWAYS_EAGER=false") {
			t.Errorf("%s env = %q", svc, env)
		}
	}
	for _, svc := range []Service{SvcApp, SvcAdmin} {
		if env := strings.Join(SpecFor("google", "/wt/google", svc, p, b).Env, " "); strings.Contains(env, "CELERY") {
			t.Errorf("frontend %s got celery env: %q", svc, env)
		}
	}

	// Without a kit vhost the worker keeps its default queue only: the pinned
	// queues on the shared vhost hold every worktree's tasks.
	plain := SpecFor("google", "/wt/google", SvcCelery, p, CeleryBroker{})
	if strings.Contains(plain.Argv[2], "-Q") || len(plain.Env) != 0 {
		t.Errorf("non-isolated worker = %q env=%v", plain.Argv[2], plain.Env)
	}
}

func TestRecordedBroker(t *testing.T) {
	setRunDir(t)
	if got := RecordedBroker("kit-test-recorded", SvcCelery); got != "" {
		t.Errorf("no record: got %q", got)
	}
	spec := LaunchSpec{
		Worktree: "kit-test-recorded", Service: SvcCelery, Cwd: t.TempDir(),
		Argv: []string{"true"},
		Env:  CeleryBroker{VHost: "kit-google"}.Env(),
	}
	if _, err := StartService(spec); err != nil {
		t.Fatal(err)
	}
	if got := RecordedBroker("kit-test-recorded", SvcCelery); got != "pyamqp://guest@localhost:5672/kit-google" {
		t.Errorf("RecordedBroker = %q", got)
	}
}

// The repo's pull-env.py output (memory:// + eager) is exactly what kit must
// replace; backing off would leave the worker idle and every task skipped.
func TestPrepareCeleryBroker_ReplacesPullEnvPlaceholder(t *testing.T) {
	t.Setenv("CELERY_BROKER_URL", "")
	stubRabbitmqctl(t, "", nil)
	dir := writeBackendEnv(t, "CELERY_BROKER_URL=memory://\nCELERY_TASK_ALWAYS_EAGER=true\n")
	b, _, warn := PrepareCeleryBroker("google", dir)
	if warn || b.VHost != "kit-google" {
		t.Fatalf("broker = %+v warn=%v, want vhost kit-google", b, warn)
	}
	if env := strings.Join(b.Env(), " "); !strings.Contains(env, "CELERY_TASK_ALWAYS_EAGER=false") {
		t.Errorf("env = %q, want eager off", env)
	}
}

func TestServicesOnOtherBroker(t *testing.T) {
	setRunDir(t)
	wt := uniqueWorktree(t)
	// Zero ports: every service falls back to pid liveness, so the test never
	// sees a real service listening on a slot port.
	var ports Ports
	start := func(svc Service, b CeleryBroker) {
		t.Helper()
		pid, err := StartService(LaunchSpec{
			Worktree: wt, Service: svc, Cwd: t.TempDir(),
			Argv: []string{"sleep", "30"}, Env: b.Env(),
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	}
	isolated := CeleryBroker{VHost: "kit-google"}
	start(SvcCelery, CeleryBroker{}) // launched before kit isolated the broker
	start(SvcBeat, isolated)
	start(SvcApp, CeleryBroker{}) // frontend: no broker to compare

	if got := ServicesOnOtherBroker(wt, ports, isolated); !slices.Equal(got, []Service{SvcCelery}) {
		t.Errorf("isolated: got %v, want [celery]", got)
	}
	if got := ServicesOnOtherBroker(wt, ports, CeleryBroker{}); !slices.Equal(got, []Service{SvcBeat}) {
		t.Errorf("shared: got %v, want [beat]", got)
	}
}

func TestWorkerSharesBroker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CELERY_BROKER_URL", "")
	os.Unsetenv("CELERY_BROKER_URL")
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "rabbitmqctl"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if WorkerSharesBroker(dir) {
		t.Error("isolatable worktree should not conflict")
	}
	t.Setenv("CELERY_BROKER_URL", "pyamqp://u@remote.example/prod")
	if !WorkerSharesBroker(dir) {
		t.Error("user-set remote broker should conflict")
	}
}
