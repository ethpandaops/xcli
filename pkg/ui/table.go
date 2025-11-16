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

// KeyValueTable creates a two-column table for key-value pairs.
// If a title is provided, it will be displayed as a header before the table.
// The keys and values from the map are displayed in "Key" and "Value" columns.
// Note: Map iteration order is not guaranteed, so the rows may appear in any order.
func KeyValueTable(title string, data map[string]string) {
	rows := [][]string{}
	for k, v := range data {
		rows = append(rows, []string{k, v})
	}

	if title != "" {
		Header(title)
	}

	Table([]string{"Key", "Value"}, rows)
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
