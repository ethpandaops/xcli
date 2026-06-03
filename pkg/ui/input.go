package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"golang.org/x/term"
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

// PasswordInput prints prompt and reads a line from stdin without echoing it,
// so the typed value stays hidden. The returned value may be empty. When stdin
// is not an interactive terminal, it falls back to a normal (echoed) read so
// piped/non-tty input still works.
func PasswordInput(prompt string) (string, error) {
	fmt.Print(prompt)

	//nolint:gosec // stdin's file descriptor is always a small, in-range int.
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)

		fmt.Println()

		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}

		return strings.TrimSpace(string(b)), nil
	}

	var result string

	_, _ = fmt.Scanln(&result)

	return strings.TrimSpace(result), nil
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
