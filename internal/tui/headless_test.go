package tui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func feed(updates ...liftoff.PlayUpdate) <-chan liftoff.PlayUpdate {
	ch := make(chan liftoff.PlayUpdate, len(updates))
	for _, u := range updates {
		ch <- u
	}
	close(ch)
	return ch
}

func TestPrintPlayUpdates(t *testing.T) {
	var out bytes.Buffer
	failed := PrintPlayUpdates(&out, feed(
		liftoff.PlayUpdate{Status: liftoff.StepDone, Title: "celery broker: vhost kit-google"},
		liftoff.PlayUpdate{Service: liftoff.SvcAPI, Status: liftoff.StepRunning, Title: "start app backend"},
		liftoff.PlayUpdate{Service: liftoff.SvcAPI, Status: liftoff.StepDone, Title: "start app backend", URL: "http://localhost:9010"},
		liftoff.PlayUpdate{Status: liftoff.StepSkipped, Title: "app backend still on another celery broker"},
	))
	if failed {
		t.Error("failed = true with no failed step")
	}
	got := out.String()
	for _, want := range []string{"vhost kit-google", "✓ start app backend  http://localhost:9010", "! app backend still on another"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "start app backend") != 1 {
		t.Errorf("running steps must not print:\n%s", got)
	}

	out.Reset()
	failed = PrintPlayUpdates(&out, feed(liftoff.PlayUpdate{
		Service: liftoff.SvcCelery, Status: liftoff.StepFailed, Title: "start celery worker", Err: errors.New("exit 1"),
	}))
	if !failed || !strings.Contains(out.String(), "✗ start celery worker: exit 1") {
		t.Errorf("failed=%v output=%q", failed, out.String())
	}
}

// Run-wide notes (no Service) must reach the screen, not vanish into the
// per-service status map under an empty key.
func TestPlayRunKeepsNotes(t *testing.T) {
	m := &playModel{
		runStatuses: map[liftoff.Service]liftoff.StepStatus{},
		runMessages: map[liftoff.Service]string{},
		runURLs:     map[liftoff.Service]string{},
		runPIDs:     map[liftoff.Service]int{},
		runUpdates:  feed(),
	}
	note := liftoff.PlayUpdate{Status: liftoff.StepSkipped, Title: "celery broker: shared default vhost"}
	m.updateRun(playUpdMsg{upd: note, ok: true})
	if len(m.runNotes) != 1 {
		t.Fatalf("runNotes = %v", m.runNotes)
	}
	if _, ok := m.runStatuses[""]; ok {
		t.Error("note stored as a service status")
	}
	if view := m.viewRun(); !strings.Contains(view, "shared default vhost") {
		t.Errorf("viewRun misses the note:\n%s", view)
	}
}
