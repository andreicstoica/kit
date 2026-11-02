package tui

import (
	tea "charm.land/bubbletea/v2"
	"testing"
	"time"
)

func TestLogQuitStopsTailsFromEveryMode(t *testing.T) {
	for _, mode := range []logMode{logModeBrowse, logModeSearch, logModeTags} {
		m := &logModel{mode: mode, done: make(chan struct{})}
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		if cmd == nil || m.mode != logModeClosed {
			t.Fatalf("Ctrl+C did not close mode %d", mode)
		}
		select {
		case <-m.done:
		default:
			t.Fatal("tail shutdown was not signaled")
		}
		m.stop()
		_, cmd = m.Update(logLineMsg{tag: "app", line: "late"})
		if cmd != nil || len(m.lines) != 0 {
			t.Fatal("closed viewer accepted more log lines")
		}
	}
}

func TestLogPumpStopsWithoutAnotherLine(t *testing.T) {
	incoming := make(chan logLineMsg)
	done := make(chan struct{})
	result := make(chan tea.Msg, 1)
	go func() { result <- pumpLines(incoming, done)() }()
	close(done)
	select {
	case msg := <-result:
		if _, ok := msg.(logEOFMsg); !ok {
			t.Fatalf("shutdown returned %T", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("log pump stayed blocked after shutdown")
	}
}
