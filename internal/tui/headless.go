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
// defaults, except that it never stops another worktree's celery worker:
// that needs the confirmation only the TUI asks for.
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
	conf, err := liftoff.LoadConfig()
	if err != nil {
		return err
	}
	meta, ok := conf.Worktrees[name]
	if name != "master" && (!ok || meta.Slot == 0) {
		return fmt.Errorf("%s has no kit slot yet; run `kit adopt %s` first", name, name)
	}
	_ = liftoff.WithConfigLock(func(c *liftoff.Config) error {
		c.TouchLastUsed(name)
		return nil
	})

	on := initialToggles(cfg)
	if on[liftoff.SvcCelery] {
		if owner, pid := liftoff.FindCeleryOwner(); owner != "" && owner != name {
			fmt.Fprintln(w, StyleWarn.Render(fmt.Sprintf(
				"  ! celery skipped: %s runs the worker (pid %d); run `kit pause %s --only celery` first",
				owner, pid, owner)))
			on[liftoff.SvcCelery] = false
			on[liftoff.SvcBeat] = false
		}
	}
	var selected []liftoff.Service
	for _, s := range liftoff.AllServices {
		if on[s] {
			selected = append(selected, s)
		}
	}
	if len(selected) == 0 {
		return errors.New("nothing left to start")
	}
	plan := liftoff.PlayPlan{
		Worktree:     name,
		WorktreePath: path,
		Slot:         meta.Slot,
		Ports:        liftoff.PortsForSlot(meta.Slot),
		Services:     selected,
	}
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
	svcs := runningServices(name, cfg.Only)
	if len(svcs) == 0 {
		fmt.Fprintf(w, "no services running for %s\n", name)
		return nil
	}
	plan := liftoff.PausePlan{
		Worktree: name,
		Services: svcs,
		Ports:    liftoff.PortsForSlot(slotFor(name)),
	}
	if PrintPlayUpdates(w, layout.RunPause(plan)) {
		return fmt.Errorf("kit pause %s: a service did not stop", name)
	}
	return nil
}
