package ui

// Compile-time guarantee that PlainRenderer satisfies Renderer.
var _ Renderer = (*PlainRenderer)(nil)

// PlainRenderer renders progress line-by-line using pterm. It is safe for
// non-interactive output (CI, pipes, logs) and preserves xcli's historical
// output format. Each method delegates to the package-level helpers so output
// is byte-for-byte identical to direct ui.* calls.
type PlainRenderer struct{}

// NewPlainRenderer creates a line-by-line renderer.
func NewPlainRenderer() *PlainRenderer {
	return &PlainRenderer{}
}

// Banner renders the prominent heading that opens a flow.
func (*PlainRenderer) Banner(message string) { Banner(message) }

// Phase starts a new top-level section of work. In plain output a phase is just
// a section header.
func (*PlainRenderer) Phase(title string) { Header(title) }

// Task starts a unit of work, backed by a line spinner.
func (*PlainRenderer) Task(name string) Task { return NewSpinner(name) }

// Header renders a section sub-heading.
func (*PlainRenderer) Header(message string) { Header(message) }

// Success renders a standalone success line.
func (*PlainRenderer) Success(message string) { Success(message) }

// Warning renders a standalone warning line.
func (*PlainRenderer) Warning(message string) { Warning(message) }

// Error renders a standalone error line.
func (*PlainRenderer) Error(message string) { Error(message) }

// Info renders a standalone informational line.
func (*PlainRenderer) Info(message string) { Info(message) }

// Blank renders vertical spacing.
func (*PlainRenderer) Blank() { Blank() }

// ServiceTable renders the final services/URLs table under a section header.
func (*PlainRenderer) ServiceTable(title string, services []Service) {
	Header(title)
	ServiceTable(services)
}

// GitStatusTable renders the table of out-of-date repositories.
func (*PlainRenderer) GitStatusTable(statuses []GitStatus) { GitStatusTable(statuses) }

// Close is a no-op; the plain renderer holds no live state.
func (*PlainRenderer) Close() {}
