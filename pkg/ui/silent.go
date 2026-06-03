package ui

// Compile-time guarantees.
var (
	_ Renderer = (*SilentRenderer)(nil)
	_ Task     = silentTask{}
)

// SilentRenderer discards all rendering. It is used when progress is surfaced
// elsewhere — for example the Command Center web UI streams stack progress over
// SSE — and duplicating it as terminal output would just be noise.
type SilentRenderer struct{}

// silentTask is a no-op task handle.
type silentTask struct{}

// NewSilentRenderer creates a renderer that produces no output.
func NewSilentRenderer() *SilentRenderer {
	return &SilentRenderer{}
}

// Banner discards the heading.
func (*SilentRenderer) Banner(string) {}

// Phase discards the section.
func (*SilentRenderer) Phase(string) {}

// Task returns a no-op task handle.
func (*SilentRenderer) Task(string) Task { return silentTask{} }

// Header discards the sub-heading.
func (*SilentRenderer) Header(string) {}

// Success discards the line.
func (*SilentRenderer) Success(string) {}

// Warning discards the line.
func (*SilentRenderer) Warning(string) {}

// Error discards the line.
func (*SilentRenderer) Error(string) {}

// Info discards the line.
func (*SilentRenderer) Info(string) {}

// Blank discards the spacing.
func (*SilentRenderer) Blank() {}

// ServiceTable discards the table.
func (*SilentRenderer) ServiceTable([]Service) {}

// GitStatusTable discards the table.
func (*SilentRenderer) GitStatusTable([]GitStatus) {}

// Close is a no-op.
func (*SilentRenderer) Close() {}

// UpdateText discards the update.
func (silentTask) UpdateText(string) {}

// Success discards the terminal state.
func (silentTask) Success(string) {}

// Fail discards the terminal state.
func (silentTask) Fail(string) {}

// Warning discards the terminal state.
func (silentTask) Warning(string) {}

// Stop is a no-op.
func (silentTask) Stop() error { return nil }
