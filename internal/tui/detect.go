package tui

import (
	"os"

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

// detectTerminalBg runs a minimal bubbletea program that sends an OSC 11
// query to the terminal and reads back the background color.
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
	return tea.RequestBackgroundColor
}

func (m *detectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		return m, tea.Quit
	}
	return m, nil
}

func (m *detectModel) View() tea.View { return tea.NewView("") }
