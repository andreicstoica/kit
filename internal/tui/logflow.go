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
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// logKeys are extra keys for the log viewer (on top of DefaultKeymap).
type logKeys struct {
	KeyMap
	Follow key.Binding
	Filter key.Binding
	Tags   key.Binding
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
			key.WithHelp("/", "search"),
		),
		Tags: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "services"),
		),
	}
}

func (k logKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Follow, k.Filter, k.Tags, k.HelpKey, k.Quit}
}
func (k logKeys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Follow},
		{k.Filter, k.Tags, k.HelpKey, k.Quit},
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

	// Parallel slices: same length, both rebuild together. tags[i] is the
	// raw service tag for the styled line lines[i].
	lines  []string
	tags   []string
	buf    strings.Builder
	width  int
	height int

	// Search (text substring) filter.
	filterMode  bool
	filterInput textinput.Model
	pattern     string

	// Service tag filter — when a tag is false in tagOn, lines with that
	// tag are hidden. Initial state: all on. The overlay panel toggled
	// via `t` lets the user pick which streams to show.
	tagMode   bool
	tagOn     map[string]bool
	tagOrder  []string
	tagCursor int

	// channel where tail goroutines push lines, plus a done channel so they exit.
	incoming chan logLineMsg
	done     chan struct{}
}

// rebuildBuf reassembles m.buf from m.lines, applying both the active
// substring filter (m.pattern) and the per-tag filter (m.tagOn).
func (m *logModel) rebuildBuf() {
	m.buf.Reset()
	m.buf.Grow(len(m.lines) * 80)
	first := true
	for i, ln := range m.lines {
		if !m.matchVisible(m.tags[i], ln) {
			continue
		}
		if !first {
			m.buf.WriteByte('\n')
		}
		m.buf.WriteString(ln)
		first = false
	}
}

func (m *logModel) matchVisible(tag, ln string) bool {
	if m.tagOn != nil {
		if on, known := m.tagOn[tag]; known && !on {
			return false
		}
	}
	if m.pattern == "" {
		return true
	}
	return strings.Contains(strings.ToLower(ln), strings.ToLower(m.pattern))
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
		m.filterInput.Width = msg.Width - 12
	case tea.KeyMsg:
		// Tag-picker overlay owns input when open.
		if m.tagMode {
			switch msg.String() {
			case "esc", "enter", "t":
				m.tagMode = false
				return m, nil
			case "up", "k":
				if m.tagCursor > 0 {
					m.tagCursor--
				}
			case "down", "j":
				if m.tagCursor < len(m.tagOrder)-1 {
					m.tagCursor++
				}
			case " ", "tab":
				if m.tagCursor < len(m.tagOrder) {
					t := m.tagOrder[m.tagCursor]
					m.tagOn[t] = !m.tagOn[t]
					m.rebuildBuf()
					m.viewport.SetContent(m.buf.String())
					if m.follow {
						m.viewport.GotoBottom()
					}
				}
			case "a":
				for _, t := range m.tagOrder {
					m.tagOn[t] = true
				}
				m.rebuildBuf()
				m.viewport.SetContent(m.buf.String())
			case "n":
				for _, t := range m.tagOrder {
					m.tagOn[t] = false
				}
				m.rebuildBuf()
				m.viewport.SetContent(m.buf.String())
			}
			return m, nil
		}
		// Filter-editing mode owns most key input.
		if m.filterMode {
			switch msg.Type {
			case tea.KeyEsc:
				m.filterMode = false
				m.filterInput.Blur()
				if m.pattern != "" {
					m.pattern = ""
					m.rebuildBuf()
					m.viewport.SetContent(m.buf.String())
				}
				return m, nil
			case tea.KeyEnter:
				m.filterMode = false
				m.filterInput.Blur()
				return m, nil
			}
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			if m.filterInput.Value() != m.pattern {
				m.pattern = m.filterInput.Value()
				m.rebuildBuf()
				m.viewport.SetContent(m.buf.String())
				if m.follow {
					m.viewport.GotoBottom()
				}
			}
			return m, cmd
		}
		switch {
		case key.Matches(msg, m.keys.Quit), msg.Type == tea.KeyCtrlC:
			close(m.done) // signal tail goroutines to exit
			return m, tea.Quit
		case key.Matches(msg, m.keys.Follow):
			m.follow = !m.follow
			if m.follow {
				m.viewport.GotoBottom()
			}
		case key.Matches(msg, m.keys.Filter):
			m.filterMode = true
			m.filterInput.SetValue(m.pattern)
			m.filterInput.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.Tags):
			m.tagMode = true
			return m, nil
		case msg.String() == "?":
			m.help.ShowAll = !m.help.ShowAll
		}
	case logLineMsg:
		styled := stylizeLine(msg.tag, msg.line)
		m.lines = append(m.lines, styled)
		m.tags = append(m.tags, msg.tag)
		// Register newly-seen tags so they appear in the tag panel.
		if _, ok := m.tagOn[msg.tag]; !ok {
			m.tagOn[msg.tag] = true
			m.tagOrder = append(m.tagOrder, msg.tag)
		}
		if len(m.lines) > 5000 {
			m.lines = m.lines[len(m.lines)-5000:]
			m.tags = m.tags[len(m.tags)-5000:]
			m.rebuildBuf()
		} else if m.matchVisible(msg.tag, styled) {
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

// tagStyles maps the on-disk log filename (= internal service id) to a
// lipgloss style. File names stay app.log/admin.log/api.log/admin_be.log
// for compatibility; display labels come from tagDisplay below.
var tagStyles = map[string]lipgloss.Style{
	"app":      lipgloss.NewStyle().Foreground(ColorAccent).Bold(true),
	"admin":    lipgloss.NewStyle().Foreground(ColorWarn).Bold(true),
	"api":      lipgloss.NewStyle().Foreground(ColorAPI).Bold(true),
	"admin_be": lipgloss.NewStyle().Foreground(ColorAdminBE).Bold(true),
	"celery":   lipgloss.NewStyle().Foreground(ColorOK).Bold(true),
	"beat":     lipgloss.NewStyle().Foreground(ColorOK).Bold(true),
	"mcp":      lipgloss.NewStyle().Foreground(ColorMuted).Bold(true),
}

// tagDisplay translates filename stems to the designer-facing service
// names. Keeps logs readable for non-engineers without renaming files.
var tagDisplay = map[string]string{
	"app":      "app_front",
	"admin":    "admin_front",
	"api":      "app_back",
	"admin_be": "admin_back",
	"celery":   "celery",
	"beat":     "celery",
	"mcp":      "mcp",
}

var defaultTagStyle = lipgloss.NewStyle().Foreground(ColorMuted).Bold(true)

// stylizeLine prefixes the service tag with a color and pads to keep
// the message column aligned across log streams.
func stylizeLine(tag, line string) string {
	display := tagDisplay[tag]
	if display == "" {
		display = tag
	}
	style, ok := tagStyles[tag]
	if !ok {
		style = defaultTagStyle
	}
	const tagWidth = 11
	label := "[" + display + "]"
	if len(label) < tagWidth {
		label += strings.Repeat(" ", tagWidth-len(label))
	}
	return style.Render(label) + " " + line
}

func (m *logModel) View() string {
	header := StyleTitle.Render("kit log — " + m.worktree)
	follow := StyleDim.Render("follow: off")
	if m.follow {
		follow = StyleOK.Render("follow: on")
	}
	header += "  " + follow + "  " + StyleDim.Render(fmt.Sprintf("%d lines", len(m.lines)))
	if m.pattern != "" && !m.filterMode {
		header += "  " + StyleWarn.Render("search: "+m.pattern)
	}
	if hidden := m.hiddenTagCount(); hidden > 0 {
		header += "  " + StyleWarn.Render(fmt.Sprintf("%d service(s) hidden", hidden))
	}

	var footer string
	switch {
	case m.tagMode:
		footer = m.viewTagPanel()
	case m.filterMode:
		footer = StyleHi.Render("search: ") + m.filterInput.View() +
			"\n" + StyleDim.Render("enter: apply · esc: clear")
	default:
		footer = m.help.View(m.keys)
	}
	return header + "\n" + m.viewport.View() + "\n" + footer
}

func (m *logModel) hiddenTagCount() int {
	n := 0
	for _, on := range m.tagOn {
		if !on {
			n++
		}
	}
	return n
}

func (m *logModel) viewTagPanel() string {
	var b strings.Builder
	b.WriteString(StyleTitle.Render("services") + "\n")
	for i, t := range m.tagOrder {
		cursor := "  "
		if i == m.tagCursor {
			cursor = "> "
		}
		box := "[ ]"
		if m.tagOn[t] {
			box = StyleOK.Render("[x]")
		}
		display := tagDisplay[t]
		if display == "" {
			display = t
		}
		b.WriteString(cursor + box + " " + display + "\n")
	}
	b.WriteString(StyleDim.Render("space toggle · a all · n none · enter/t/esc close"))
	return b.String()
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

	ti := textinput.New()
	ti.Placeholder = "substring (case-insensitive)…"
	ti.Prompt = ""
	ti.CharLimit = 80

	m := &logModel{
		worktree:    worktree,
		files:       files,
		follow:      true,
		viewport:    vp,
		help:        NewHelp(),
		keys:        newLogKeys(),
		filterInput: ti,
		tagOn:       map[string]bool{},
		incoming:    make(chan logLineMsg, 256),
		done:        make(chan struct{}),
	}
	_, runErr := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return runErr
}
