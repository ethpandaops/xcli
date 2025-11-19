package ui

import (
	"fmt"

	"github.com/pterm/pterm"
)

// ASCII art for xcli logo.
const xcliLogo = `
 __  _______ _     ___
 \ \/ / ____| |   |_ _|
  \  / |    | |    | |
  /  \ |___ | |___ | |
 /_/\_\____|_____|___|
`

// PrintInitBanner prints the full ASCII banner for init commands.
// This should only be used for major first-run experiences like 'xcli init'.
func PrintInitBanner(version string) {
	// Print the ASCII logo in cyan
	fmt.Print(pterm.Cyan(xcliLogo))

	// Print subtitle
	subtitle := fmt.Sprintf(" ethPandaOps - %s", version)
	fmt.Println(pterm.NewStyle(pterm.FgWhite, pterm.Bold).Sprint(subtitle))
	fmt.Println()
}

// PrintCompactBanner prints a minimal one-line banner.
// Use this sparingly - most commands should not print any banner.
func PrintCompactBanner(version string) {
	fmt.Printf("%s %s\n",
		pterm.Cyan("xcli"),
		pterm.Gray(fmt.Sprintf("v%s", version)),
	)
}
