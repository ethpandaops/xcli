package tui

import "github.com/charmbracelet/lipgloss"

const (
	statusRunning = "running"
	statusStopped = "stopped"

	panelServices = "services"
	panelActions  = "actions"
)

var (
	// Colors.
	ColorGreen  = lipgloss.Color("10")
	ColorRed    = lipgloss.Color("9")
	ColorYellow = lipgloss.Color("11")
	ColorBlue   = lipgloss.Color("12")
	ColorCyan   = lipgloss.Color("14")
	ColorGray   = lipgloss.Color("8")

	// Styles.
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorCyan).
			MarginBottom(1)

	StyleRunning = lipgloss.NewStyle().
			Foreground(ColorGreen).
			Bold(true)

	StyleStopped = lipgloss.NewStyle().
			Foreground(ColorGray)

	StyleError = lipgloss.NewStyle().
			Foreground(ColorRed).
			Bold(true)

	StyleSelected = lipgloss.NewStyle().
			Background(lipgloss.Color("237")).
			Foreground(lipgloss.Color("15"))

	StylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorCyan).
			Padding(0, 1)

	StyleHelp = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")). // Bright white
			Bold(true).
			Background(lipgloss.Color("#3A3A3A")). // Dark gray background
			MarginTop(1).
			Padding(0, 1)

	StyleStatusBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FFFF")). // Bright cyan
			Bold(true).
			Background(lipgloss.Color("#3A3A3A")). // Dark gray background
			Padding(0, 1)
)
