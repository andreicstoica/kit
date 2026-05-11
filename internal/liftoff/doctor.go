package liftoff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// CheckStatus is the outcome of a single diagnostic.
type CheckStatus int

const (
	CheckOK   CheckStatus = iota // tool present and healthy
	CheckWarn                    // installed but degraded (eg gh not authed)
	CheckFail                    // missing or unusable
	CheckSkip                    // pre-req failed; nothing to check
)

// CheckResult is the rendered+remediable output of a single check.
type CheckResult struct {
	ID      string
	Name    string
	Status  CheckStatus
	Detail  string
	FixHint string   // human one-liner shown in report
	FixCmd  []string // command setup runs on confirm; nil = manual fix only
	FixCask bool     // brew install --cask vs plain brew install
}

// Check is one registered diagnostic.
type Check struct {
	ID  string
	Run func() CheckResult
}

// DefaultChecks returns the canonical doctor check list.
// Caller should pass a Layout (typically DefaultLayout()).
func DefaultChecks(layout Layout) []Check {
	return []Check{
		{ID: "brew", Run: checkBrew},
		{ID: "brew-path", Run: checkBrewPath},
		{ID: "git", Run: checkGit},
		{ID: "gh", Run: checkGh},
		{ID: "node-yarn", Run: checkNodeYarn},
		{ID: "python", Run: checkPython},
		{ID: "redis", Run: checkRedis},
		{ID: "rabbitmq", Run: checkRabbitMQ},
		{ID: "postgres", Run: checkPostgres},
		{ID: "ghostty", Run: checkGhostty},
		{ID: "editor", Run: checkEditor},
		{ID: "liftoff-master", Run: func() CheckResult { return checkLiftoffMaster(layout) }},
		{ID: "kit-config", Run: checkKitConfig},
	}
}

// RunChecks executes every check in parallel (capped concurrency) and
// returns results in registered order.
func RunChecks(checks []Check) []CheckResult {
	results := make([]CheckResult, len(checks))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, c := range checks {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, c Check) {
			defer wg.Done()
			defer func() { <-sem }()
			r := c.Run()
			if r.ID == "" {
				r.ID = c.ID
			}
			results[i] = r
		}(i, c)
	}
	wg.Wait()
	return results
}

// AnyFailed reports whether any result has Status == CheckFail.
func AnyFailed(results []CheckResult) bool {
	for _, r := range results {
		if r.Status == CheckFail {
			return true
		}
	}
	return false
}

// AnyWarned reports whether any result has Status == CheckWarn.
func AnyWarned(results []CheckResult) bool {
	for _, r := range results {
		if r.Status == CheckWarn {
			return true
		}
	}
	return false
}

// Summary counts OK/Warn/Fail/Skip.
type Summary struct {
	OK, Warn, Fail, Skip int
}

func Summarize(results []CheckResult) Summary {
	var s Summary
	for _, r := range results {
		switch r.Status {
		case CheckOK:
			s.OK++
		case CheckWarn:
			s.Warn++
		case CheckFail:
			s.Fail++
		case CheckSkip:
			s.Skip++
		}
	}
	return s
}

// ---- individual checks ----

func checkBrew() CheckResult {
	r := CheckResult{Name: "brew"}
	st := DetectBrew()
	if st.OnPath {
		ver, _ := ToolVersion("brew", "--version")
		ver = firstLine(ver)
		prefix := st.PrefixDir
		r.Status = CheckOK
		r.Detail = fmt.Sprintf("%s  (%s)", ver, prefix)
		return r
	}
	// Not on PATH; brew-path check handles "binary exists, not on PATH".
	if st.BinaryAt != "" {
		r.Status = CheckSkip
		r.Detail = "installed but not on PATH — see brew-path"
		return r
	}
	r.Status = CheckFail
	r.Detail = "not installed"
	r.FixHint = `install Homebrew: /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`
	return r
}

func checkBrewPath() CheckResult {
	r := CheckResult{Name: "brew-path"}
	st := DetectBrew()
	if st.OnPath {
		// Don't clutter the report when everything's fine.
		r.Status = CheckSkip
		return r
	}
	if st.BinaryAt == "" {
		r.Status = CheckSkip
		return r
	}
	r.Status = CheckFail
	r.Detail = "brew at " + st.BinaryAt + " but not on PATH"
	r.FixHint = `add this line to ~/.zshrc: ` + BrewShellenvLine(st.BinaryAt)
	return r
}

func checkGit() CheckResult {
	r := CheckResult{Name: "git"}
	if _, err := exec.LookPath("git"); err != nil {
		r.Status = CheckFail
		r.Detail = "not installed"
		r.FixHint = "brew install git"
		r.FixCmd = []string{"git"}
		return r
	}
	ver, _ := ToolVersion("git", "--version")
	r.Status = CheckOK
	r.Detail = ver
	return r
}

func checkGh() CheckResult {
	r := CheckResult{Name: "gh"}
	if _, err := exec.LookPath("gh"); err != nil {
		r.Status = CheckFail
		r.Detail = "not installed"
		r.FixHint = "brew install gh"
		r.FixCmd = []string{"gh"}
		return r
	}
	ver, _ := ToolVersion("gh", "--version")
	ver = firstLine(ver)
	if err := exec.Command("gh", "auth", "status").Run(); err != nil {
		r.Status = CheckWarn
		r.Detail = ver + "  not authenticated"
		r.FixHint = "gh auth login"
		return r
	}
	r.Status = CheckOK
	r.Detail = ver + "  authenticated"
	return r
}

func checkNodeYarn() CheckResult {
	r := CheckResult{Name: "node/yarn"}
	if _, err := exec.LookPath("node"); err != nil {
		r.Status = CheckFail
		r.Detail = "node not installed"
		r.FixHint = "brew install node yarn"
		r.FixCmd = []string{"node", "yarn"}
		return r
	}
	if _, err := exec.LookPath("yarn"); err != nil {
		r.Status = CheckFail
		r.Detail = "yarn not installed"
		r.FixHint = "brew install yarn"
		r.FixCmd = []string{"yarn"}
		return r
	}
	nodeVer, _ := ToolVersion("node", "--version")
	yarnVer, _ := ToolVersion("yarn", "--version")
	r.Status = CheckOK
	r.Detail = fmt.Sprintf("node %s  yarn %s", nodeVer, yarnVer)
	return r
}

func checkPython() CheckResult {
	r := CheckResult{Name: "python"}
	if _, err := exec.LookPath("python3"); err != nil {
		r.Status = CheckFail
		r.Detail = "python3 not installed"
		r.FixHint = "brew install python@3.14"
		r.FixCmd = []string{"python@3.14"}
		return r
	}
	venv := os.Getenv("KIT_PY_VENV")
	if venv == "" {
		home, _ := os.UserHomeDir()
		venv = filepath.Join(home, ".envs", "py314")
	}
	venvPy := filepath.Join(venv, "bin", "python")
	if _, err := os.Stat(filepath.Join(venv, "bin", "activate")); err != nil {
		sysVer, _ := ToolVersion("python3", "--version")
		r.Status = CheckWarn
		r.Detail = sysVer + "  venv missing at " + venv
		r.FixHint = "brew install python@3.14 && python3.14 -m venv " + venv
		return r
	}
	venvVer, _ := ToolVersion(venvPy, "--version")
	if !strings.Contains(venvVer, "3.14") {
		r.Status = CheckWarn
		r.Detail = venvVer + " in " + venv + "  (Liftoff wants 3.14)"
		r.FixHint = "brew install python@3.14 && rm -rf " + venv + " && python3.14 -m venv " + venv
		return r
	}
	r.Status = CheckOK
	r.Detail = venvVer + "  venv at " + venv
	return r
}

func checkRedis() CheckResult {
	r := CheckResult{Name: "redis"}
	if _, err := exec.LookPath("redis-cli"); err != nil {
		r.Status = CheckFail
		r.Detail = "redis not installed"
		r.FixHint = "brew install redis"
		r.FixCmd = []string{"redis"}
		return r
	}
	out, err := exec.Command("redis-cli", "ping").Output()
	if err != nil || strings.TrimSpace(string(out)) != "PONG" {
		r.Status = CheckWarn
		r.Detail = "installed but not reachable on :6379"
		r.FixHint = "brew services start redis"
		return r
	}
	ver, _ := ToolVersion("redis-cli", "--version")
	ver = firstWord(ver, "redis-cli ")
	r.Status = CheckOK
	r.Detail = "redis " + ver + "  reachable on :6379"
	return r
}

func checkPostgres() CheckResult {
	r := CheckResult{Name: "postgres"}
	if _, err := exec.LookPath("pg_isready"); err != nil {
		r.Status = CheckFail
		r.Detail = "postgres not installed"
		r.FixHint = "brew install postgresql@17 pgvector postgis"
		r.FixCmd = []string{"postgresql@17", "pgvector", "postgis"}
		return r
	}
	if err := exec.Command("pg_isready", "-h", "127.0.0.1", "-p", "5432").Run(); err != nil {
		r.Status = CheckWarn
		r.Detail = "installed but not reachable on :5432"
		r.FixHint = "brew services start postgresql@17"
		return r
	}
	ver, _ := ToolVersion("psql", "--version")
	r.Status = CheckOK
	r.Detail = ver + "  reachable on :5432"
	return r
}

func checkRabbitMQ() CheckResult {
	r := CheckResult{Name: "rabbitmq"}
	// rabbitmqctl ships in brew's sbin (e.g. /opt/homebrew/sbin), which is
	// commonly not on PATH. Fall back to known locations before declaring
	// it missing.
	bin := findBinary("rabbitmqctl",
		"/opt/homebrew/sbin/rabbitmqctl",
		"/usr/local/sbin/rabbitmqctl")
	if bin == "" {
		r.Status = CheckFail
		r.Detail = "rabbitmq not installed"
		r.FixHint = "brew install rabbitmq"
		r.FixCmd = []string{"rabbitmq"}
		return r
	}
	if err := exec.Command(bin, "status").Run(); err != nil {
		r.Status = CheckWarn
		r.Detail = "not running — only needed if you run celery tasks"
		r.FixHint = "brew services start rabbitmq"
		return r
	}
	r.Status = CheckOK
	r.Detail = "running"
	return r
}

// findBinary returns the first path that resolves: PATH first, then explicit
// fallback paths. Returns "" if none match.
func findBinary(name string, fallbacks ...string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, p := range fallbacks {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func checkGhostty() CheckResult {
	r := CheckResult{Name: "ghostty"}
	if appExists("Ghostty.app") {
		r.Status = CheckOK
		r.Detail = "/Applications/Ghostty.app"
		return r
	}
	r.Status = CheckWarn
	r.Detail = "not installed"
	r.FixHint = "brew install --cask ghostty"
	r.FixCmd = []string{"ghostty"}
	r.FixCask = true
	return r
}

func checkEditor() CheckResult {
	r := CheckResult{Name: "editors"}
	known := []struct{ App, Label string }{
		{"Zed.app", "Zed"},
		{"Cursor.app", "Cursor"},
		{"Visual Studio Code.app", "VS Code"},
	}
	var found []string
	for _, k := range known {
		if appExists(k.App) {
			found = append(found, k.Label)
		}
	}
	if len(found) > 0 {
		r.Status = CheckOK
		r.Detail = strings.Join(found, ", ")
		return r
	}
	r.Status = CheckWarn
	r.Detail = "no supported editor in /Applications"
	r.FixHint = "brew install --cask zed"
	r.FixCmd = []string{"zed"}
	r.FixCask = true
	return r
}

func checkLiftoffMaster(layout Layout) CheckResult {
	r := CheckResult{Name: "liftoff repo"}
	if _, err := os.Stat(layout.Master); err != nil {
		r.Status = CheckFail
		r.Detail = "master repo not found at " + layout.Master
		r.FixHint = "run `kit setup` to clone"
		return r
	}
	if _, err := os.Stat(filepath.Join(layout.Master, ".git")); err != nil {
		r.Status = CheckFail
		r.Detail = layout.Master + " exists but is not a git repo"
		r.FixHint = "remove the directory and re-run `kit setup`"
		return r
	}
	if _, err := os.Stat(filepath.Join(layout.Master, "frontend", "app", "node_modules")); err != nil {
		r.Status = CheckWarn
		r.Detail = "master clone present but node_modules missing"
		r.FixHint = "run `kit setup` to finish bootstrapping (yarn install in master)"
		return r
	}
	r.Status = CheckOK
	r.Detail = "ready at " + layout.Master
	return r
}

func checkKitConfig() CheckResult {
	r := CheckResult{Name: "kit-config"}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "kit")
	if _, err := os.Stat(dir); err != nil {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			r.Status = CheckFail
			r.Detail = "cannot create " + dir + ": " + mkErr.Error()
			return r
		}
	}
	r.Status = CheckOK
	r.Detail = dir
	return r
}

// ---- helpers ----

func appExists(app string) bool {
	for _, root := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
		if _, err := os.Stat(filepath.Join(root, app)); err == nil {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func firstWord(s, prefix string) string {
	s = strings.TrimPrefix(s, prefix)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// SortResultsByID is a deterministic ordering used in tests.
func SortResultsByID(results []CheckResult) {
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
}
