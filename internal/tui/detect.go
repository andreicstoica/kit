package tui

import (
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
)

// DetectTerminalBackground queries the terminal for its background color and
// updates the global palette. Returns true if the background is dark. Used by
// static (non-TUI) commands like `kit lineup` and `kit doctor` that never
// receive a tea.BackgroundColorMsg. Falls back to dark on error (the safe
// default for most kit developers' terminals).
func DetectTerminalBackground() bool {
	if !isTerminal(os.Stdin) {
		SetDarkBackground(true)
		return true
	}
	dark := detectTerminalBg()
	SetDarkBackground(dark)
	return dark
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// detectTimeout bounds the background-color probe. Bubble Tea v2 sends no
// fallback BackgroundColorMsg when the terminal never answers (e.g. Herdr
// panes), so without this the probe program blocks forever and static
// commands hang. Stays well under the 5s v1 termenv timeout.
const detectTimeout = 500 * time.Millisecond

type detectTimeoutMsg struct{}

// detectTerminalBg runs a minimal bubbletea program that sends an OSC 11
// query to the terminal and reads back the background color. Falls back to
// the dark default if the terminal stays silent past detectTimeout.
func detectTerminalBg() bool {
	m := &detectModel{dark: true} // default dark
	p := tea.NewProgram(m, tea.WithoutCatchPanics())
	// Run in raw mode to get the terminal response, then quit immediately.
	if _, err := p.Run(); err != nil {
		return true
	}
	return m.dark
}

type detectModel struct {
	dark bool
}

func (m *detectModel) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
		tea.Tick(detectTimeout, func(time.Time) tea.Msg { return detectTimeoutMsg{} }),
	)
}

func (m *detectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		return m, tea.Quit
	case detectTimeoutMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m *detectModel) View() tea.View { return tea.NewView("") }
