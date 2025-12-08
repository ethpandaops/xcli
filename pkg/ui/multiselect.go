package ui

import (
	"errors"
	"fmt"

	"github.com/pterm/pterm"
)

// MultiSelectOption represents a single option in the multi-select menu.
type MultiSelectOption struct {
	Label       string // Display text (e.g., "cbt")
	Description string // Additional info (e.g., "current: v0.5.2")
	Value       string // Return value when selected
	Selected    bool   // Pre-selected state
}

// MultiSelect displays an interactive multi-select menu and returns selected values.
// Returns slice of selected option Values, or error if cancelled/failed.
// At least one option must be selected (returns error otherwise).
func MultiSelect(title string, options []MultiSelectOption) ([]string, error) {
	if len(options) == 0 {
		return nil, errors.New("no options provided")
	}

	// Build display strings and track mapping
	displayStrings := make([]string, 0, len(options))
	valueMap := make(map[string]string, len(options))
	defaultSelected := make([]string, 0)

	for _, opt := range options {
		display := opt.Label
		if opt.Description != "" {
			display = fmt.Sprintf("%s  %s", opt.Label, pterm.Gray(opt.Description))
		}

		displayStrings = append(displayStrings, display)
		valueMap[display] = opt.Value

		if opt.Selected {
			defaultSelected = append(defaultSelected, display)
		}
	}

	// Create interactive multiselect
	printer := pterm.DefaultInteractiveMultiselect.
		WithOptions(displayStrings).
		WithDefaultOptions(defaultSelected).
		WithFilter(false).
		WithCheckmark(&pterm.Checkmark{Checked: pterm.Green("✓"), Unchecked: "○"})

	selected, err := printer.Show(title)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSelectionCancelled, err)
	}

	if len(selected) == 0 {
		return nil, errors.New("no projects selected")
	}

	// Map display strings back to values
	result := make([]string, 0, len(selected))
	for _, display := range selected {
		if value, ok := valueMap[display]; ok {
			result = append(result, value)
		}
	}

	return result, nil
}
