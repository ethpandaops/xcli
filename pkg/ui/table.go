package ui

import (
	"github.com/pterm/pterm"
)

// Service represents a service for display in a ServiceTable.
// It contains the basic information needed to show service status.
type Service struct {
	Name   string
	URL    string
	Status string
}

// Table creates and prints a formatted table with headers and rows.
// The headers are displayed in bold at the top of the table.
// This is a general-purpose table function that can be used for any tabular data.
func Table(headers []string, rows [][]string) {
	data := [][]string{headers}
	data = append(data, rows...)
	_ = pterm.DefaultTable.WithHasHeader().WithData(data).Render()
}

// ServiceTable creates a formatted table for services with color-coded status.
// Services with "running" status are displayed in green, all others in red.
// The table displays three columns: Service name, URL, and Status.
func ServiceTable(services []Service) {
	headers := []string{"Service", "URL", "Status"}
	rows := [][]string{}

	for _, svc := range services {
		var status string
		if svc.Status == "running" {
			status = pterm.Green(svc.Status)
		} else {
			status = pterm.Red(svc.Status)
		}

		rows = append(rows, []string{svc.Name, svc.URL, status})
	}

	Table(headers, rows)
}

// GitStatus represents a git repository status for display in a GitStatusTable.
type GitStatus struct {
	Repository string
	Branch     string
	Status     string
}

// GitStatusTable creates a formatted table for git repository status.
// Displays repository name, current branch, and status (up to date or behind/ahead commits).
func GitStatusTable(statuses []GitStatus) {
	headers := []string{"Repository", "Branch", "Status"}
	rows := [][]string{}

	for _, status := range statuses {
		rows = append(rows, []string{status.Repository, status.Branch, pterm.Yellow(status.Status)})
	}

	Table(headers, rows)
}
