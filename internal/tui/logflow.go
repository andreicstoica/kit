package tui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// logKeys are extra keys for the log viewer (on top of DefaultKeymap).
type logKeys struct {
	KeyMap
	Follow key.Binding
	Filter key.Binding
}

func newLogKeys() logKeys {
	return logKeys{
		KeyMap: DefaultKeymap,
		Follow: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "follow"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
	}
}

func (k logKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Follow, k.Filter, k.HelpKey, k.Quit}
}
func (k logKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Follow},
		{k.Filter, k.HelpKey, k.Quit},
	}
}

// logLineMsg is one new line read from a log file.
type logLineMsg struct {
	tag  string
	line string
}

// logEOFMsg fires when a tail goroutine exits.
type logEOFMsg struct{ tag string }

type logModel struct {
	worktree string
	files    []string
	follow   bool

	viewport viewport.Model
	help     help.Model
	keys     logKeys

	lines  []string // raw, full backlog (capped)
	buf    strings.Builder // joined content; rebuilt only on trim
	width  int
	height int

	// channel where tail goroutines push lines, plus a done channel so they exit.
	incoming chan logLineMsg
	done     chan struct{}
}

// rebuildBuf reassembles m.buf from m.lines. Called only when the rolling
// window trims old lines; new-line appends use a cheap WriteString.
func (m *logModel) rebuildBuf() {
	m.buf.Reset()
	m.buf.Grow(len(m.lines) * 80)
	for i, ln := range m.lines {
		if i > 0 {
			m.buf.WriteByte('\n')
		}
		m.buf.WriteString(ln)
	}
}

// pumpLines is the tea.Cmd that reads from the channel.
func pumpLines(ch chan logLineMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return logEOFMsg{}
		}
		return msg
	}
}

func (m *logModel) Init() tea.Cmd {
	for _, p := range m.files {
		go tailFileToChan(p, m.incoming, m.done)
	}
	return pumpLines(m.incoming)
}

// trySend attempts to send msg on ch but bails out if done closes first.
// Returns false if done was signaled (caller should exit).
func trySend(ch chan<- logLineMsg, done <-chan struct{}, msg logLineMsg) bool {
	select {
	case ch <- msg:
		return true
	case <-done:
		return false
	}
}

func tailFileToChan(path string, ch chan<- logLineMsg, done <-chan struct{}) {
	tag := strings.TrimSuffix(filepath.Base(path), ".log")
	f, err := os.Open(path)
	if err != nil {
		trySend(ch, done, logLineMsg{tag: tag, line: fmt.Sprintf("[error] open: %v", err)})
		return
	}
	defer f.Close()
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		trySend(ch, done, logLineMsg{tag: tag, line: fmt.Sprintf("[error] seek: %v", err)})
		return
	}
	r := bufio.NewReader(f)
	for {
		select {
		case <-done:
			return
		default:
		}
		line, err := r.ReadString('\n')
		if line != "" {
			if !trySend(ch, done, logLineMsg{tag: tag, line: strings.TrimRight(line, "\n")}) {
				return
			}
		}
		if err == io.EOF {
			select {
			case <-done:
				return
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}
		if err != nil {
			trySend(ch, done, logLineMsg{tag: tag, line: fmt.Sprintf("[error] %v", err)})
			return
		}
	}
}

func (m *logModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.viewport.Width = msg.Width - 2
		m.viewport.Height = msg.Height - 5
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit), msg.Type == tea.KeyCtrlC:
			close(m.done) // signal tail goroutines to exit
			return m, tea.Quit
		case key.Matches(msg, m.keys.Follow):
			m.follow = !m.follow
			if m.follow {
				m.viewport.GotoBottom()
			}
		case msg.String() == "?":
			m.help.ShowAll = !m.help.ShowAll
		}
	case logLineMsg:
		styled := stylizeLine(msg.tag, msg.line)
		m.lines = append(m.lines, styled)
		if len(m.lines) > 5000 {
			m.lines = m.lines[len(m.lines)-5000:]
			m.rebuildBuf()
		} else {
			if m.buf.Len() > 0 {
				m.buf.WriteByte('\n')
			}
			m.buf.WriteString(styled)
		}
		m.viewport.SetContent(m.buf.String())
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, pumpLines(m.incoming)
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// tagStyles maps log file tag → lipgloss style. Built once at init.
var tagStyles = map[string]lipgloss.Style{
	"app":      lipgloss.NewStyle().Foreground(ColorAccent).Bold(true),
	"admin":    lipgloss.NewStyle().Foreground(ColorWarn).Bold(true),
	"api":      lipgloss.NewStyle().Foreground(ColorAPI).Bold(true),
	"admin_be": lipgloss.NewStyle().Foreground(ColorAdminBE).Bold(true),
	"celery":   lipgloss.NewStyle().Foreground(ColorOK).Bold(true),
	"beat":     lipgloss.NewStyle().Foreground(ColorErr).Bold(true),
	"mcp":      lipgloss.NewStyle().Foreground(ColorMuted).Bold(true),
}

var defaultTagStyle = lipgloss.NewStyle().Foreground(ColorMuted).Bold(true)

// stylizeLine prefixes the service tag with a color and shows the raw line.
func stylizeLine(tag, line string) string {
	style, ok := tagStyles[tag]
	if !ok {
		style = defaultTagStyle
	}
	return style.Render("["+tag+"]") + " " + line
}

func (m *logModel) View() string {
	header := StyleTitle.Render("kit log — " + m.worktree)
	follow := StyleDim.Render("follow: off")
	if m.follow {
		follow = StyleOK.Render("follow: on")
	}
	header += "  " + follow + "  " + StyleDim.Render(fmt.Sprintf("%d lines", len(m.lines)))

	footer := m.help.View(m.keys)
	return header + "\n" + m.viewport.View() + "\n" + footer
}

// RunLogTUI is the cobra entry point for `kit log`.
func RunLogTUI(worktree string) error {
	dir := liftoff.RunDirPath(worktree)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("no log dir for %s — run `kit play` first", worktree)
	}
	var files []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".log" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	if len(files) == 0 {
		return errors.New("no logs in " + dir)
	}

	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colorDim)

	m := &logModel{
		worktree: worktree,
		files:    files,
		follow:   true,
		viewport: vp,
		help:     NewHelp(),
		keys:     newLogKeys(),
		incoming: make(chan logLineMsg, 256),
		done:     make(chan struct{}),
	}
	_, runErr := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return runErr
}
