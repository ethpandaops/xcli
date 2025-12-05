// Package ui provides terminal user interface components including
// spinners, status messages, tables, and formatted output.
package ui

import (
	"fmt"

	"github.com/pterm/pterm"
)

// Success prints a success message with green checkmark.
func Success(message string) {
	fmt.Printf("%s %s\n", SuccessSymbol, SuccessStyle.Sprint(message))
}

// Error prints an error message with red X.
func Error(message string) {
	fmt.Printf("%s %s\n", ErrorSymbol, ErrorStyle.Sprint(message))
}

// Warning prints a warning message with yellow symbol.
func Warning(message string) {
	fmt.Printf("%s %s\n", WarningSymbol, WarningStyle.Sprint(message))
}

// Info prints an info message with cyan arrow.
func Info(message string) {
	fmt.Printf("%s %s\n", InfoSymbol, InfoStyle.Sprint(message))
}

// Header prints a styled section header.
func Header(message string) {
	fmt.Printf("%s\n", HeaderStyle.Sprint(message))
}

// Section prints a prominent section header with separator line.
func Section(message string) {
	separator := pterm.Gray("─────────────────────────────────────────────────")
	fmt.Printf("\n%s\n%s\n", separator, HeaderStyle.Sprint(message))
}

// Banner prints a prominent banner.
func Banner(message string) {
	fmt.Printf("\n%s\n", pterm.DefaultHeader.WithFullWidth().Sprint(message))
}

// Blank prints a blank line for spacing.
func Blank() {
	fmt.Println()
}
