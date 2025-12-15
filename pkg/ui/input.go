package ui

import (
	"fmt"

	"github.com/pterm/pterm"
)

// TextInput displays an interactive text input and returns the entered value.
// Returns the entered text, or error if cancelled/failed.
func TextInput(prompt string, defaultValue string) (string, error) {
	input := pterm.DefaultInteractiveTextInput.
		WithDefaultText(prompt)

	if defaultValue != "" {
		input = input.WithDefaultValue(defaultValue)
	}

	result, err := input.Show()
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrSelectionCancelled, err)
	}

	return result, nil
}

// TextInputRequired displays an interactive text input that requires a non-empty value.
// Returns the entered text, or error if cancelled/failed/empty.
func TextInputRequired(prompt string) (string, error) {
	result, err := TextInput(prompt, "")
	if err != nil {
		return "", err
	}

	if result == "" {
		return "", fmt.Errorf("input required")
	}

	return result, nil
}
