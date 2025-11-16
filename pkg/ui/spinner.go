package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

// Spinner wraps pterm spinner with convenience methods.
type Spinner struct {
	spinner *pterm.SpinnerPrinter
	message string
}

// NewSpinner creates and starts a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	// Disable spinners in test mode to avoid race conditions with pterm's internal goroutines
	if isTestMode() {
		return &Spinner{
			spinner: nil,
			message: message,
		}
	}

	s, _ := pterm.DefaultSpinner.
		WithRemoveWhenDone(false). // Keep spinner result, don't remove
		Start(message)

	return &Spinner{
		spinner: s,
		message: message,
	}
}

// NewSilentSpinner creates a spinner that will be removed when stopped.
// Use this for child operations that should disappear without leaving blank lines.
func NewSilentSpinner(message string) *Spinner {
	// Disable spinners in test mode to avoid race conditions with pterm's internal goroutines
	if isTestMode() {
		return &Spinner{
			spinner: nil,
			message: message,
		}
	}

	s, _ := pterm.DefaultSpinner.
		WithRemoveWhenDone(true). // Remove completely when stopped
		Start(message)

	return &Spinner{
		spinner: s,
		message: message,
	}
}

// UpdateText updates the spinner message.
func (s *Spinner) UpdateText(message string) {
	s.message = message

	if s.spinner != nil {
		s.spinner.UpdateText(message)
	}
}

// Success stops the spinner with a success message.
func (s *Spinner) Success(message string) {
	if message == "" {
		message = s.message
	}

	if s.spinner != nil {
		s.spinner.Success(message)
	}
}

// SuccessWithDuration stops spinner with duration display.
func (s *Spinner) SuccessWithDuration(message string, duration time.Duration) {
	if s.spinner != nil {
		s.spinner.Success(fmt.Sprintf("%s (%.2fs)", message, duration.Seconds()))
	}
}

// Fail stops the spinner with an error message.
func (s *Spinner) Fail(message string) {
	if message == "" {
		message = s.message
	}

	if s.spinner != nil {
		s.spinner.Fail(message)
	}
}

// Warning stops the spinner with a warning message.
func (s *Spinner) Warning(message string) {
	if message == "" {
		message = s.message
	}

	if s.spinner != nil {
		s.spinner.Warning(message)
	}
}

// Stop stops the spinner without a message.
func (s *Spinner) Stop() error {
	if s.spinner != nil {
		return s.spinner.Stop()
	}

	return nil
}

// WithSpinner executes a function with a spinner.
// If the function returns an error, spinner fails; otherwise succeeds.
func WithSpinner(message string, fn func() error) error {
	s := NewSpinner(message)

	err := fn()
	if err != nil {
		s.Fail(message)

		return err
	}

	s.Success(message)

	return nil
}

// WithSpinnerAndUpdate executes a function with a spinner that can be updated.
func WithSpinnerAndUpdate(initialMessage string, fn func(update func(string)) error) error {
	s := NewSpinner(initialMessage)
	updateFn := func(msg string) {
		s.UpdateText(msg)
	}

	err := fn(updateFn)
	if err != nil {
		s.Fail(initialMessage)

		return err
	}

	s.Success(initialMessage)

	return nil
}

// isTestMode checks if we're running in test mode by examining os.Args and environment.
func isTestMode() bool {
	// Check if running under go test
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.") {
			return true
		}
	}

	// Check for XCLI_TEST_MODE environment variable (for integration tests)
	if os.Getenv("XCLI_TEST_MODE") == "true" {
		return true
	}

	return false
}
