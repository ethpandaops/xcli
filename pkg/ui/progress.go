package ui

import (
	"github.com/pterm/pterm"
)

// ProgressBar wraps pterm progress bar.
type ProgressBar struct {
	bar *pterm.ProgressbarPrinter
}

// NewProgressBar creates a progress bar with title and total steps.
func NewProgressBar(title string, total int) *ProgressBar {
	bar, _ := pterm.DefaultProgressbar.
		WithTitle(title).
		WithTotal(total).
		Start()

	return &ProgressBar{bar: bar}
}

// Increment increments the progress bar by 1.
func (p *ProgressBar) Increment() {
	p.bar.Increment()
}

// Add adds n to the progress bar.
func (p *ProgressBar) Add(n int) {
	p.bar.Add(n)
}

// UpdateTitle updates the progress bar title.
func (p *ProgressBar) UpdateTitle(title string) {
	p.bar.UpdateTitle(title)
}

// Stop stops the progress bar.
func (p *ProgressBar) Stop() error {
	_, err := p.bar.Stop()

	return err
}

// WithProgress executes a function with a progress bar.
func WithProgress(title string, total int, fn func(bar *ProgressBar) error) error {
	bar := NewProgressBar(title, total)

	defer func() {
		_ = bar.Stop()
	}()

	return fn(bar)
}
