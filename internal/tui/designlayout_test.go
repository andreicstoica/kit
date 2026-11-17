package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
	"github.com/andreicstoica/kit/internal/liftoff"
)

func mkDesignModel(width int) *designModel {
	titles := []string{
		"fetch origin/master",
		"worktree add /Users/acs/liftoff/user-prof-intro-preferences -b user-prof-intro-preferences master",
		"copy env files (root, backend, frontend/app, frontend/admin)",
		"allocate port slot",
	}
	st := make([]liftoff.StepStatus, len(titles))
	for i := range st {
		st[i] = liftoff.StepDone
	}
	return &designModel{
		answers:       &designAnswers{name: "user-prof-intro-preferences"},
		worktree:      "/Users/acs/liftoff/user-prof-intro-preferences",
		spinner:       spinner.New(),
		anim:          NewAnimation(),
		keys:          DefaultKeymap,
		help:          NewHelp(),
		stepTitles:    titles,
		stepStatuses:  st,
		stepElapsed:   make([]time.Duration, len(titles)),
		currentLines:  map[int][]string{},
		done:          true,
		allocatedSlot: 3,
		width:         width,
	}
}

func TestOrbNotClipped(t *testing.T) {
	for _, w := range []int{120, 140, 100, 70} {
		m := mkDesignModel(w)
		body := m.View()
		got := lipgloss.Width(body.Content)
		if got > w {
			t.Errorf("width=%d: rendered body width %d exceeds terminal (orb would clip)", w, got)
		} else {
			t.Logf("width=%d: body width %d (ok)", w, got)
		}
	}
}

// orbColumn returns the screen column where the orb's top border starts.
func orbColumn(t *testing.T, m *designModel) int {
	t.Helper()
	for _, ln := range strings.Split(m.View().Content, "\n") {
		if i := strings.Index(ln, "╭"); i >= 0 {
			return lipgloss.Width(ln[:i])
		}
	}
	t.Fatal("orb border not rendered")
	return -1
}

// The orb must sit flush against the right edge, not trail the widest step
// line — otherwise it slides sideways whenever a note or elapsed time lands.
func TestOrbPinnedToRightEdge(t *testing.T) {
	for _, w := range []int{200, 160, 140, 120, 100} {
		m := mkDesignModel(w)
		orbW := lipgloss.Width(m.anim.View())
		want := w - orbW
		if got := orbColumn(t, m); got != want {
			t.Errorf("width=%d: orb starts at column %d, want %d (flush right)", w, got, want)
		}

		// Same terminal, wider step text: the orb must not move.
		short := mkDesignModel(w)
		long := mkDesignModel(w)
		long.stepNotes = map[int]string{
			0: "Git fallback: requested revision differs from source HEAD",
		}
		long.stepStatuses = append([]liftoff.StepStatus(nil), long.stepStatuses...)
		long.stepStatuses[0] = liftoff.StepDone
		if a, b := orbColumn(t, short), orbColumn(t, long); a != b {
			t.Errorf("width=%d: orb column moved from %d to %d when a note appeared", w, a, b)
		}
	}
}

func TestFriendlyDesignStepTitle(t *testing.T) {
	cases := map[string]string{
		"fetch origin/master":                                          "Get the latest main code",
		"worktree add /tmp/voice -b voice master":                      "Create the workspace folder",
		"copy env files (root, backend, frontend/app, frontend/admin)": "Copy app settings",
		"create database liftoff_voice":                                "Create a private database",
		"clone database liftoff -> liftoff_voice":                      "Copy local data",
		"update backend/.env SQLALCHEMY_DATABASE_NAME=liftoff_voice":   "Point the app at the private database",
		"uv install backend":                                           "Install backend dependencies",
		"symlink frontend node_modules from master":                    "Reuse frontend dependencies",
		"gt track --parent master":                                     "Add the branch to Graphite",
		"write gtab workspace":                                         "Create workspace tabs",
		"allocate port slot":                                           "Reserve local ports",
	}
	for in, want := range cases {
		if got := friendlyDesignStepTitle(in); got != want {
			t.Errorf("friendlyDesignStepTitle(%q) = %q, want %q", in, got, want)
		}
	}
	if got := friendlyDesignStepTitle("custom step"); got != "custom step" {
		t.Errorf("unknown step = %q, want passthrough", got)
	}
}

func TestDesignStepNote(t *testing.T) {
	cases := map[string]string{
		"git-twig: APFS CoW: verified tracked files":                       "copy-on-write",
		"git-twig: stock Git: requested revision differs from source HEAD": "Git fallback: requested revision differs from source HEAD",
		"copied liftoff by template":                                       "fast copy",
		"template copy unavailable, using pg_dump: busy":                   "full copy, source database in use",
		"already copied by template":                                       "copied in the previous step",
		"CREATE TABLE":                                                     "",
	}
	for line, want := range cases {
		if got := designStepNote(line); got != want {
			t.Errorf("designStepNote(%q) = %q, want %q", line, got, want)
		}
	}
}
