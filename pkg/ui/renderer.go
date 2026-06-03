package ui

import (
	"os"

	"golang.org/x/term"
)

// Compile-time guarantee that the existing line spinner satisfies Task, so the
// plain renderer can hand one back directly.
var _ Task = (*Spinner)(nil)

// logsVisible tracks whether verbose logs are being written to stdout. When
// true, live (redrawing) renderers must not be used because interleaved log
// lines would corrupt the frame. Set once at startup from the verbose flag.
var logsVisible bool

// SetLogsVisible records whether verbose log output is streaming to stdout.
// The CLI entry point calls this from the verbose flag so every command's
// renderer selection avoids a live frame when logs would corrupt it.
func SetLogsVisible(visible bool) {
	logsVisible = visible
}

// Task is a handle to a single unit of work shown within a phase. The plain
// renderer backs it with a line spinner; a live renderer backs it with a node
// in a redrawing task tree. Methods are safe to call until the task reaches a
// terminal state (Success/Fail/Warning) or the renderer is closed.
type Task interface {
	// UpdateText changes the in-progress label.
	UpdateText(message string)
	// Success marks the task complete. An empty message keeps the start label.
	Success(message string)
	// Fail marks the task failed. An empty message keeps the start label.
	Fail(message string)
	// Warning marks the task complete with a caveat.
	Warning(message string)
	// Stop ends the task without a terminal status line.
	Stop() error
}

// Renderer renders structured progress for long-running flows such as
// 'lab up'. Implementations either print line-by-line (plain, CI-safe) or
// maintain a live redrawing view (TTY). All methods must be called from a
// single goroutine; tasks returned by Task are owned by that same goroutine.
type Renderer interface {
	// Banner renders the prominent heading that opens a flow.
	Banner(message string)
	// Phase starts a new top-level section of work.
	Phase(title string)
	// Task starts a unit of work within the current phase.
	Task(name string) Task
	// Header renders a section sub-heading.
	Header(message string)
	// Success renders a standalone success line.
	Success(message string)
	// Warning renders a standalone warning line.
	Warning(message string)
	// Error renders a standalone error line.
	Error(message string)
	// Info renders a standalone informational line.
	Info(message string)
	// Blank renders vertical spacing.
	Blank()
	// ServiceTable renders the final services/URLs table.
	ServiceTable(services []Service)
	// GitStatusTable renders the table of out-of-date repositories.
	GitStatusTable(statuses []GitStatus)
	// Close flushes and tears down any live rendering. Idempotent.
	Close()
}

// NewRenderer returns the most capable renderer the current environment
// supports. It selects the plain, CI-safe renderer whenever stdout is not an
// interactive terminal, when running under tests, or when CI is set; otherwise
// it returns the live task-tree renderer.
//
// onInterrupt is forwarded to the live renderer and invoked when the user
// presses ctrl+c, so the caller can cancel its context for a graceful
// shutdown. It is ignored by the plain renderer (which relies on the process
// signal handler) and may be nil.
func NewRenderer(onInterrupt func()) Renderer {
	if !isInteractive() {
		return NewPlainRenderer()
	}

	return NewTTYRenderer(onInterrupt)
}

// isInteractive reports whether stdout is a live terminal we may redraw in
// place. It is deliberately conservative: anything that looks like CI, a pipe,
// or a test run is treated as non-interactive.
func isInteractive() bool {
	if isTestMode() {
		return false
	}

	if logsVisible {
		return false
	}

	if os.Getenv("CI") != "" {
		return false
	}

	//nolint:gosec // stdout's file descriptor is always a small, in-range int.
	return term.IsTerminal(int(os.Stdout.Fd()))
}
