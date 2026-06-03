package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// statusRunning is the Service.Status value marking a healthy, running service.
const statusRunning = "running"

// StatusPanel renders a group of services as a rounded, titled panel with a
// status dot per row — the same visual language as the 'lab up' summary. A
// green dot marks a running service; a dim dot plus a "(status)" suffix marks
// anything else. In non-interactive output (pipes, CI) it falls back to the
// plain table so scripted/redirected output stays stable and parseable.
func StatusPanel(title string, services []Service) {
	if !isInteractive() {
		Header(title)
		ServiceTable(services)

		return
	}

	if len(services) == 0 {
		return
	}

	nameWidth, urlWidth := 0, 0

	for _, s := range services {
		if w := lipgloss.Width(s.Name); w > nameWidth {
			nameWidth = w
		}

		if w := lipgloss.Width(s.URL); w > urlWidth {
			urlWidth = w
		}
	}

	rows := make([]string, 0, len(services))
	innerWidth := 0

	for _, s := range services {
		row := statusRow(s, nameWidth, urlWidth)
		rows = append(rows, row)

		if w := lipgloss.Width(row); w > innerWidth {
			innerWidth = w
		}
	}

	fmt.Print(renderPanel(title, rows, innerWidth))
}

// statusRow renders a single service line for a panel: dot, padded name, padded
// URL, and a trailing status note for non-running services. A running service
// renders at full brightness so the list reads as a result, not as fine print;
// a non-running one is dimmed to recede.
func statusRow(s Service, nameWidth, urlWidth int) string {
	running := s.Status == statusRunning

	dot := styleTreeGreen.Render("●")
	nameStyle := lipgloss.NewStyle()

	if !running {
		dot = styleTreeDim.Render("●")
		nameStyle = styleTreeDim
	}

	name := nameStyle.Render(s.Name + strings.Repeat(" ", nameWidth-lipgloss.Width(s.Name)))

	url := s.URL + strings.Repeat(" ", urlWidth-lipgloss.Width(s.URL))
	if s.URL == "-" || s.URL == "" {
		url = styleTreeDim.Render(url)
	} else {
		url = styleTreeURL.Render(url)
	}

	row := dot + "  " + name + "   " + url

	if !running {
		row += "  " + styleTreeDim.Render("("+s.Status+")")
	}

	return row
}
