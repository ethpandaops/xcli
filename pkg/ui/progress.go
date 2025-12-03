package ui

import (
	"sync"

	"github.com/pterm/pterm"
)

// ProgressBar wraps pterm progress bar.
type ProgressBar struct {
	bar     *pterm.ProgressbarPrinter
	mu      sync.Mutex
	active  bool
	current int
	total   int
}

// NewProgressBar creates a progress bar with title and total steps.
func NewProgressBar(title string, total int) *ProgressBar {
	// Disable pterm's auto-remove feature which causes a race condition when multiple
	// goroutines call Increment() concurrently and one triggers the auto-stop.
	bar, _ := pterm.DefaultProgressbar.
		WithTitle(title).
		WithTotal(total).
		WithRemoveWhenDone(false).
		Start()

	return &ProgressBar{
		bar:     bar,
		active:  true,
		current: 0,
		total:   total,
	}
}

// Increment increments the progress bar by 1.
func (p *ProgressBar) Increment() {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active || p.bar == nil {
		return
	}

	p.current++

	// Don't increment past total - pterm's internal state becomes invalid
	if p.current > p.total {
		return
	}

	p.bar.Increment()
}

// Add adds n to the progress bar.
func (p *ProgressBar) Add(n int) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active || p.bar == nil {
		return
	}

	p.current += n

	// Don't increment past total - pterm's internal state becomes invalid
	if p.current > p.total {
		return
	}

	p.bar.Add(n)
}

// UpdateTitle updates the progress bar title.
func (p *ProgressBar) UpdateTitle(title string) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active || p.bar == nil {
		return
	}

	p.bar.UpdateTitle(title)
}

// Stop stops the progress bar.
func (p *ProgressBar) Stop() error {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active || p.bar == nil {
		return nil
	}

	p.active = false

	_, err := p.bar.Stop()

	return err
}
