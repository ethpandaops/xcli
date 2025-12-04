package ui

import (
	"errors"
	"fmt"

	"github.com/pterm/pterm"
)

// ErrSelectionCancelled is returned when user cancels a selection.
var ErrSelectionCancelled = errors.New("selection cancelled")

// SelectOption represents a single option in the selection menu.
type SelectOption struct {
	Label       string // Display text (e.g., "cbt")
	Description string // Additional info (e.g., "current: v0.5.2")
	Value       string // Return value when selected
}

// Select displays an interactive selection menu and returns the selected value.
// Returns the selected option's Value, or error if cancelled/failed.
func Select(title string, options []SelectOption) (string, error) {
	return SelectWithDefault(title, options, "")
}

// SelectWithDefault displays a selection menu with a default selected option.
func SelectWithDefault(title string, options []SelectOption, defaultValue string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no options provided")
	}

	// Build display strings for pterm
	displayOptions := make([]string, 0, len(options))
	valueMap := make(map[string]string, len(options))

	for _, opt := range options {
		displayStr := opt.Label
		if opt.Description != "" {
			displayStr = fmt.Sprintf("%s  %s", opt.Label, pterm.Gray(opt.Description))
		}

		displayOptions = append(displayOptions, displayStr)
		valueMap[displayStr] = opt.Value
	}

	// Find default index
	defaultIndex := 0

	if defaultValue != "" {
		for i, opt := range options {
			if opt.Value == defaultValue {
				defaultIndex = i

				break
			}
		}
	}

	// Create and run the interactive selector
	selector := pterm.DefaultInteractiveSelect.
		WithOptions(displayOptions).
		WithDefaultOption(displayOptions[defaultIndex])

	if title != "" {
		selector = selector.WithDefaultText(title)
	}

	selected, err := selector.Show()
	if err != nil {
		// pterm returns error on Ctrl+C or escape
		return "", fmt.Errorf("%w: %w", ErrSelectionCancelled, err)
	}

	return valueMap[selected], nil
}

// Confirm displays a yes/no confirmation prompt.
// Returns true if user confirms, false otherwise.
func Confirm(message string) (bool, error) {
	return ConfirmWithDefault(message, false)
}

// ConfirmWithDefault displays a confirmation with a default value.
func ConfirmWithDefault(message string, defaultYes bool) (bool, error) {
	result, err := pterm.DefaultInteractiveConfirm.
		WithDefaultText(message).
		WithDefaultValue(defaultYes).
		Show()
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrSelectionCancelled, err)
	}

	return result, nil
}
