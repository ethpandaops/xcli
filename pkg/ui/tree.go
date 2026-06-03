package ui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pterm/pterm"
)

// spinnerFrames drives the in-progress task glyph; one frame per tick.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Palette shared with the dashboard TUI (256-colour indices).
var (
	treeGreen  = lipgloss.Color("10")
	treeRed    = lipgloss.Color("9")
	treeYellow = lipgloss.Color("11")
	treeCyan   = lipgloss.Color("14")
	treeGray   = lipgloss.Color("8")
)

var (
	styleTreeTitle   = lipgloss.NewStyle().Bold(true)
	styleTreeRule    = lipgloss.NewStyle().Foreground(treeGray)
	styleTreePhase   = lipgloss.NewStyle().Bold(true).Foreground(treeCyan)
	styleTreeAccent  = lipgloss.NewStyle().Foreground(treeCyan)
	styleTreeDim     = lipgloss.NewStyle().Foreground(treeGray)
	styleTreeGreen   = lipgloss.NewStyle().Foreground(treeGreen)
	styleTreeRed     = lipgloss.NewStyle().Foreground(treeRed).Bold(true)
	styleTreeYellow  = lipgloss.NewStyle().Foreground(treeYellow)
	styleTreeURL     = lipgloss.NewStyle().Foreground(treeCyan).Underline(true)
	styleTreeHeader  = lipgloss.NewStyle().Bold(true).Foreground(treeCyan)
	styleTreeSuccess = lipgloss.NewStyle().Foreground(treeGreen).Bold(true)
)

// Compile-time guarantees.
var (
	_ Renderer = (*TTYRenderer)(nil)
	_ Task     = (*ttyTask)(nil)
)

// taskStatus is the lifecycle state of a tree task.
type taskStatus int

const (
	taskRunning taskStatus = iota
	taskOK
	taskFail
	taskWarn
)

// blockKind tags the entries rendered top-to-bottom in the live frame.
type blockKind int

const (
	blockBanner blockKind = iota
	blockPhase
	blockLine
	blockBlank
	blockTable
	blockGitTable
)

// lineKind tags a standalone status line.
type lineKind int

const (
	lineHeader lineKind = iota
	lineSuccess
	lineWarning
	lineError
	lineInfo
)

// TTYRenderer renders the build flow as a live, redrawing task tree using
// Bubble Tea. It is used only for interactive terminals; CI and piped output
// use PlainRenderer instead. Its methods are safe to call from a single
// goroutine and translate into messages for the rendering goroutine.
type TTYRenderer struct {
	prog   *tea.Program
	done   chan struct{}
	nextID atomic.Uint64
	close  sync.Once
}

// ttyTask is a handle to one task node; its methods post updates by id.
type ttyTask struct {
	id   uint64
	prog *tea.Program
}

// treeTask is the rendering goroutine's view of a single unit of work.
type treeTask struct {
	id        uint64
	name      string
	status    taskStatus
	startedAt time.Time
	doneAt    time.Time
}

// treeBlock is one renderable entry in the frame.
type treeBlock struct {
	kind     blockKind
	text     string
	lineKind lineKind
	tasks    []*treeTask
	services []Service
	gitRepos []GitStatus
}

// treeModel is the Bubble Tea model backing the live frame.
type treeModel struct {
	blocks      []treeBlock
	width       int
	frame       int
	now         time.Time
	interrupted bool
	onInterrupt func()
}

// Message types posted by renderer methods to the model.
type (
	bannerMsg  struct{ text string }
	phaseMsg   struct{ title string }
	addTaskMsg struct {
		id   uint64
		name string
	}
	taskUpdateMsg struct {
		id   uint64
		text string
	}
	taskDoneMsg struct {
		id     uint64
		status taskStatus
		text   string
	}
	lineMsg struct {
		kind lineKind
		text string
	}
	blankMsg    struct{}
	tableMsg    struct{ services []Service }
	gitTableMsg struct{ repos []GitStatus }
	tickMsg     time.Time
)

// NewTTYRenderer starts a live task-tree renderer. onInterrupt is invoked when
// the user presses ctrl+c so the caller can cancel its context for a graceful
// shutdown; a second ctrl+c quits the renderer outright. onInterrupt may be nil.
func NewTTYRenderer(onInterrupt func()) *TTYRenderer {
	// The live frame owns stdout. Silence pterm so any spinner, table, or
	// progress bar emitted by nested managers (git, infrastructure, builder)
	// cannot corrupt the frame; Close restores it.
	pterm.DisableOutput()

	m := &treeModel{onInterrupt: onInterrupt}

	prog := tea.NewProgram(m, tea.WithoutSignalHandler())

	r := &TTYRenderer{
		prog: prog,
		done: make(chan struct{}),
	}

	go func() {
		defer close(r.done)

		_, _ = prog.Run()
	}()

	return r
}

// Banner posts the opening heading.
func (r *TTYRenderer) Banner(message string) { r.prog.Send(bannerMsg{text: message}) }

// Phase starts a new top-level section.
func (r *TTYRenderer) Phase(title string) { r.prog.Send(phaseMsg{title: title}) }

// Task starts a unit of work and returns its handle.
func (r *TTYRenderer) Task(name string) Task {
	id := r.nextID.Add(1)
	r.prog.Send(addTaskMsg{id: id, name: name})

	return &ttyTask{id: id, prog: r.prog}
}

// Header posts a section sub-heading.
func (r *TTYRenderer) Header(message string) {
	r.prog.Send(lineMsg{kind: lineHeader, text: message})
}

// Success posts a standalone success line.
func (r *TTYRenderer) Success(message string) {
	r.prog.Send(lineMsg{kind: lineSuccess, text: message})
}

// Warning posts a standalone warning line.
func (r *TTYRenderer) Warning(message string) {
	r.prog.Send(lineMsg{kind: lineWarning, text: message})
}

// Error posts a standalone error line.
func (r *TTYRenderer) Error(message string) {
	r.prog.Send(lineMsg{kind: lineError, text: message})
}

// Info posts a standalone informational line.
func (r *TTYRenderer) Info(message string) {
	r.prog.Send(lineMsg{kind: lineInfo, text: message})
}

// Blank posts vertical spacing.
func (r *TTYRenderer) Blank() { r.prog.Send(blankMsg{}) }

// ServiceTable posts the final services/URLs table.
func (r *TTYRenderer) ServiceTable(services []Service) {
	r.prog.Send(tableMsg{services: services})
}

// GitStatusTable posts the out-of-date repositories table.
func (r *TTYRenderer) GitStatusTable(statuses []GitStatus) {
	r.prog.Send(gitTableMsg{repos: statuses})
}

// Close quits the program and waits for the final frame to flush. Idempotent.
func (r *TTYRenderer) Close() {
	r.close.Do(func() {
		r.prog.Quit()
		<-r.done
		pterm.EnableOutput()
	})
}

// UpdateText changes the task's in-progress label.
func (t *ttyTask) UpdateText(message string) {
	t.prog.Send(taskUpdateMsg{id: t.id, text: message})
}

// Success marks the task complete.
func (t *ttyTask) Success(message string) {
	t.prog.Send(taskDoneMsg{id: t.id, status: taskOK, text: message})
}

// Fail marks the task failed.
func (t *ttyTask) Fail(message string) {
	t.prog.Send(taskDoneMsg{id: t.id, status: taskFail, text: message})
}

// Warning marks the task complete with a caveat.
func (t *ttyTask) Warning(message string) {
	t.prog.Send(taskDoneMsg{id: t.id, status: taskWarn, text: message})
}

// Stop ends the task without a terminal status line.
func (t *ttyTask) Stop() error {
	t.prog.Send(taskDoneMsg{id: t.id, status: taskOK, text: ""})

	return nil
}

// Init starts the spinner/elapsed tick loop.
func (m *treeModel) Init() tea.Cmd {
	return tick()
}

// Update applies incoming messages to the model.
func (m *treeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			if m.interrupted {
				return m, tea.Quit
			}

			m.interrupted = true
			if m.onInterrupt != nil {
				m.onInterrupt()
			}
		}
	case tickMsg:
		m.now = time.Time(msg)
		m.frame++

		return m, tick()
	case bannerMsg:
		m.blocks = append(m.blocks, treeBlock{kind: blockBanner, text: msg.text})
	case phaseMsg:
		m.blocks = append(m.blocks, treeBlock{kind: blockPhase, text: msg.title})
	case addTaskMsg:
		phase := m.currentPhase()
		phase.tasks = append(phase.tasks, &treeTask{
			id:        msg.id,
			name:      msg.name,
			status:    taskRunning,
			startedAt: m.clock(),
		})
	case taskUpdateMsg:
		if t := m.findTask(msg.id); t != nil {
			t.name = msg.text
		}
	case taskDoneMsg:
		if t := m.findTask(msg.id); t != nil {
			t.status = msg.status
			t.doneAt = m.clock()

			if msg.text != "" {
				t.name = msg.text
			}
		}
	case lineMsg:
		m.blocks = append(m.blocks, treeBlock{kind: blockLine, lineKind: msg.kind, text: msg.text})
	case blankMsg:
		m.blocks = append(m.blocks, treeBlock{kind: blockBlank})
	case tableMsg:
		m.blocks = append(m.blocks, treeBlock{kind: blockTable, services: msg.services})
	case gitTableMsg:
		m.blocks = append(m.blocks, treeBlock{kind: blockGitTable, gitRepos: msg.repos})
	}

	return m, nil
}

// View renders the current frame.
func (m *treeModel) View() string {
	var b strings.Builder

	for _, block := range m.blocks {
		switch block.kind {
		case blockBanner:
			b.WriteString(m.renderBanner(block.text))
		case blockPhase:
			b.WriteString(m.renderPhase(block))
		case blockLine:
			b.WriteString(renderLine(block.lineKind, block.text))
			b.WriteByte('\n')
		case blockBlank:
			b.WriteByte('\n')
		case blockTable:
			b.WriteString(m.renderTable(block.services))
		case blockGitTable:
			b.WriteString(m.renderGitTable(block.gitRepos))
		}
	}

	if m.interrupted {
		b.WriteString(styleTreeYellow.Render("⚠ Interrupt received, shutting down gracefully…"))
		b.WriteByte('\n')
	}

	return b.String()
}

// currentPhase returns the phase tasks attach to, creating an implicit
// headerless phase when a task is started before any Phase call.
func (m *treeModel) currentPhase() *treeBlock {
	if n := len(m.blocks); n > 0 && m.blocks[n-1].kind == blockPhase {
		return &m.blocks[n-1]
	}

	m.blocks = append(m.blocks, treeBlock{kind: blockPhase})

	return &m.blocks[len(m.blocks)-1]
}

// findTask locates a task node by id across all phases.
func (m *treeModel) findTask(id uint64) *treeTask {
	for i := range m.blocks {
		for _, t := range m.blocks[i].tasks {
			if t.id == id {
				return t
			}
		}
	}

	return nil
}

// clock returns the model's notion of "now", falling back to the tick clock.
func (m *treeModel) clock() time.Time {
	if m.now.IsZero() {
		return time.Now()
	}

	return m.now
}

// renderBanner renders the opening heading and a dim rule.
func (m *treeModel) renderBanner(text string) string {
	rule := strings.Repeat("─", m.ruleWidth())

	return styleTreeTitle.Render(text) + "\n" + styleTreeRule.Render(rule) + "\n"
}

// renderPhase renders a phase header (if titled) and its task tree.
func (m *treeModel) renderPhase(block treeBlock) string {
	var b strings.Builder

	if block.text != "" {
		b.WriteString(styleTreeAccent.Render("▌ ") + styleTreePhase.Render(block.text))
		b.WriteByte('\n')
	}

	for i, t := range block.tasks {
		connector := "├─"
		if i == len(block.tasks)-1 {
			connector = "└─"
		}

		b.WriteString(m.renderTask(t, connector))
		b.WriteByte('\n')
	}

	return b.String()
}

// renderTask renders one task line: connector, status glyph, label, duration.
func (m *treeModel) renderTask(t *treeTask, connector string) string {
	glyph := styleTreeAccent.Render(spinnerFrames[m.frame%len(spinnerFrames)])

	switch t.status {
	case taskOK:
		glyph = styleTreeGreen.Render("✓")
	case taskFail:
		glyph = styleTreeRed.Render("✗")
	case taskWarn:
		glyph = styleTreeYellow.Render("⚠")
	case taskRunning:
	}

	// A task message may carry detail lines (e.g. a failed check listing repos);
	// only the first line shares the row with the glyph and duration, the rest
	// are indented beneath it.
	headline, detail, hasDetail := strings.Cut(t.name, "\n")

	left := fmt.Sprintf("  %s %s %s", styleTreeDim.Render(connector), glyph, headline)
	dur := styleTreeDim.Render(m.taskDuration(t))

	row := left + "  " + dur
	if m.width > 0 {
		pad := max(m.width-lipgloss.Width(left)-lipgloss.Width(dur), 2)
		row = left + strings.Repeat(" ", pad) + dur
	}

	if !hasDetail {
		return row
	}

	var b strings.Builder

	b.WriteString(row)

	for line := range strings.SplitSeq(detail, "\n") {
		b.WriteString("\n       " + styleTreeDim.Render(strings.TrimLeft(line, " ")))
	}

	return b.String()
}

// renderTable renders the final services list with clickable URLs.
func (m *treeModel) renderTable(services []Service) string {
	width := 0
	for _, s := range services {
		if w := lipgloss.Width(s.Name); w > width {
			width = w
		}
	}

	var b strings.Builder

	for _, s := range services {
		name := s.Name + strings.Repeat(" ", width-lipgloss.Width(s.Name))
		b.WriteString("  " + styleTreeDim.Render(name) + "  " + styleTreeURL.Render(s.URL))
		b.WriteByte('\n')
	}

	return b.String()
}

// renderGitTable renders the out-of-date repositories as an aligned table.
func (m *treeModel) renderGitTable(repos []GitStatus) string {
	repoWidth, branchWidth := len("Repository"), len("Branch")

	for _, r := range repos {
		if w := lipgloss.Width(r.Repository); w > repoWidth {
			repoWidth = w
		}

		if w := lipgloss.Width(r.Branch); w > branchWidth {
			branchWidth = w
		}
	}

	pad := func(s string, w int) string {
		return s + strings.Repeat(" ", w-lipgloss.Width(s))
	}

	var b strings.Builder

	header := "  " + pad("Repository", repoWidth) + "  " + pad("Branch", branchWidth) + "  Status"
	b.WriteString(styleTreeDim.Render(header))
	b.WriteByte('\n')

	for _, r := range repos {
		row := "  " + pad(r.Repository, repoWidth) + "  " + pad(r.Branch, branchWidth) + "  "
		b.WriteString(row + styleTreeYellow.Render(r.Status))
		b.WriteByte('\n')
	}

	return b.String()
}

// taskDuration formats a task's elapsed or total time.
func (m *treeModel) taskDuration(t *treeTask) string {
	end := t.doneAt
	if t.status == taskRunning {
		end = m.clock()
	}

	d := max(end.Sub(t.startedAt), 0)

	return formatTreeDuration(d)
}

// ruleWidth returns the width of the banner rule, capped for tidiness.
func (m *treeModel) ruleWidth() int {
	const maxRule = 48
	if m.width > 0 && m.width < maxRule {
		return m.width
	}

	return maxRule
}

// renderLine renders a standalone status line with its glyph.
func renderLine(kind lineKind, text string) string {
	switch kind {
	case lineSuccess:
		return styleTreeSuccess.Render("✓ " + text)
	case lineWarning:
		return styleTreeYellow.Render("⚠ " + text)
	case lineError:
		return styleTreeRed.Render("✗ " + text)
	case lineInfo:
		return styleTreeAccent.Render("→ ") + text
	case lineHeader:
		return styleTreeHeader.Render(text)
	}

	return text
}

// formatTreeDuration renders a compact duration like "6.2s" or "1m04s".
func formatTreeDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// tick schedules the next spinner/elapsed refresh.
func tick() tea.Cmd {
	return tea.Tick(time.Second/10, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
