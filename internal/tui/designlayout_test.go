package tui

import (
	"testing"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
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
	pb := progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage())
	pb.Width = 30
	return &designModel{
		answers:       &designAnswers{name: "user-prof-intro-preferences"},
		worktree:      "/Users/acs/liftoff/user-prof-intro-preferences",
		spinner:       spinner.New(),
		progress:      pb,
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
		got := lipgloss.Width(body)
		if got > w {
			t.Errorf("width=%d: rendered body width %d exceeds terminal (orb would clip)", w, got)
		} else {
			t.Logf("width=%d: body width %d (ok)", w, got)
		}
	}
}
