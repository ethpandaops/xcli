package ui

import "github.com/pterm/pterm"

var (
	// Color styles.
	SuccessStyle = pterm.NewStyle(pterm.FgGreen)
	ErrorStyle   = pterm.NewStyle(pterm.FgRed)
	WarningStyle = pterm.NewStyle(pterm.FgYellow)
	InfoStyle    = pterm.NewStyle(pterm.FgCyan)

	// Symbol styles.
	SuccessSymbol = pterm.Green("✓")
	ErrorSymbol   = pterm.Red("✗")
	WarningSymbol = pterm.Yellow("⚠")
	InfoSymbol    = pterm.Cyan("→")

	// Section header style.
	HeaderStyle = pterm.NewStyle(pterm.FgCyan, pterm.Bold)
)
