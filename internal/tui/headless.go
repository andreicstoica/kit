package tui

import (
	"errors"
	"fmt"
	"io"

	"github.com/andreicstoica/kit/internal/liftoff"
)

// PrintPlayUpdates writes one line per finished step and reports whether any
// step failed. Used where no terminal UI runs: restart and non-TTY play/pause.
func PrintPlayUpdates(w io.Writer, ch <-chan liftoff.PlayUpdate) (failed bool) {
	for upd := range ch {
		switch upd.Status {
		case liftoff.StepDone:
			line := "  ✓ " + upd.Title
			if upd.URL != "" {
				line += "  " + upd.URL
			}
			fmt.Fprintln(w, StyleOK.Render(line))
		case liftoff.StepSkipped:
			fmt.Fprintln(w, StyleWarn.Render("  ! "+upd.Title))
		case liftoff.StepFailed:
			failed = true
			msg := upd.Title
			if upd.Err != nil {
				msg += ": " + upd.Err.Error()
			}
			fmt.Fprintln(w, StyleErr.Render("  ✗ "+msg))
		}
	}
	return failed
}

// RunPlayHeadless is `kit play` without a terminal. It takes the TUI's
// defaults, except that it never stops another worktree's celery worker when
// they would share a broker: that needs the confirmation only the TUI asks for.
func RunPlayHeadless(layout liftoff.Layout, cfg PlayConfig, w io.Writer) error {
	name := cfg.Name
	if name == "" {
		return errors.New("kit play needs a workspace name when not run in a terminal")
	}
	if !layout.MasterIsRepo() {
		return fmt.Errorf("master repo not found at %s", layout.Master)
	}
	path, err := layout.ResolveWorktreePath(name)
	if err != nil {
		return err
	}
	on := initialToggles(cfg)
	var selected []liftoff.Service
	for _, s := range liftoff.AllServices {
		if on[s] {
			selected = append(selected, s)
		}
	}
	if len(selected) == 0 {
		return errors.New("nothing left to start")
	}
	res, err := buildPlayPlan(layout, name, path, selected)
	if err != nil {
		return err
	}
	if res.needsAdopt {
		return fmt.Errorf("%s has no kit slot yet; run `kit adopt %s` first", name, name)
	}
	if res.celeryBlocked {
		fmt.Fprintln(w, StyleWarn.Render(fmt.Sprintf(
			"  ! celery skipped: %s runs the worker (pid %d); run `kit pause %s --only celery` first",
			res.celeryVictim, res.celeryPID, res.celeryVictim)))
		filtered := selected[:0]
		for _, s := range selected {
			if s != liftoff.SvcCelery && s != liftoff.SvcBeat {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return errors.New("nothing left to start")
		}
		res, err = buildPlayPlan(layout, name, path, filtered)
		if err != nil {
			return err
		}
	}
	plan := res.plan
	failed := PrintPlayUpdates(w, layout.RunPlay(plan))
	fmt.Fprintln(w, StyleDim.Render("logs: "+liftoff.RunDirPath(name)))
	if failed {
		return fmt.Errorf("kit play %s: a service failed to start", name)
	}
	return nil
}

// RunPauseHeadless is `kit pause <name>` without a terminal: it stops the
// running services (narrowed by cfg.Only) with no confirm step.
func RunPauseHeadless(layout liftoff.Layout, cfg PauseConfig, w io.Writer) error {
	name := cfg.Name
	if name == "" {
		return errors.New("kit pause needs a workspace name when not run in a terminal")
	}
	if _, err := layout.ResolveWorktreePath(name); err != nil {
		return err
	}
	slot, err := slotFor(name)
	if err != nil {
		return err
	}
	svcs, err := runningServices(name, cfg.Only)
	if err != nil {
		return err
	}
	if len(svcs) == 0 {
		fmt.Fprintf(w, "no services running for %s\n", name)
		return nil
	}
	plan := liftoff.PausePlan{
		Worktree: name,
		Services: svcs,
		Ports:    liftoff.PortsForSlot(slot),
	}
	if PrintPlayUpdates(w, layout.RunPause(plan)) {
		return fmt.Errorf("kit pause %s: a service did not stop", name)
	}
	return nil
}
