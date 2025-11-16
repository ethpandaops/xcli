package ui

import (
	"io"
)

// ConditionalWriter writes to the underlying writer only if enabled
type ConditionalWriter struct {
	writer  io.Writer
	enabled bool
}

// NewConditionalWriter creates a writer that only writes when enabled
func NewConditionalWriter(writer io.Writer, enabled bool) *ConditionalWriter {
	return &ConditionalWriter{
		writer:  writer,
		enabled: enabled,
	}
}

// Write implements io.Writer.
func (w *ConditionalWriter) Write(p []byte) (n int, err error) {
	if !w.enabled {
		// Discard the write but return success
		return len(p), nil
	}

	return w.writer.Write(p)
}

// SetEnabled enables or disables writing
func (w *ConditionalWriter) SetEnabled(enabled bool) {
	w.enabled = enabled
}

// IsEnabled returns whether writing is enabled
func (w *ConditionalWriter) IsEnabled() bool {
	return w.enabled
}
