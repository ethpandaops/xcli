// Package configgen generates service configuration files from templates
// for CBT engines, APIs, and other lab services.
package configgen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/ethpandaops/xcli/pkg/configgen/dashboards"
)

// grafanaDatasourceConfig is the static Grafana datasource provisioning YAML.
const grafanaDatasourceConfig = `apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    uid: prometheus
    access: proxy
    url: http://host.docker.internal:9090
    isDefault: true
    editable: false
`

// grafanaDashboardProvider is the static Grafana dashboard provider YAML.
const grafanaDashboardProvider = `apiVersion: 1
providers:
  - name: 'Lab Dashboards'
    orgId: 1
    folder: ''
    type: file
    disableDeletion: false
    editable: true
    options:
      path: /var/lib/grafana/dashboards
`

// PrometheusTarget represents a scrape target for Prometheus.
type PrometheusTarget struct {
	JobName string
	Address string
	Path    string
}

// PrometheusTemplateData holds data for prometheus.yml template.
type PrometheusTemplateData struct {
	ScrapeInterval string
	Targets        []PrometheusTarget
}

// prometheusConfigTemplate is the template for generating prometheus.yml.
const prometheusConfigTemplate = `global:
  scrape_interval: {{ .ScrapeInterval }}
  evaluation_interval: 15s

scrape_configs:
{{- range .Targets }}
  - job_name: '{{ .JobName }}'
    static_configs:
      - targets: ['{{ .Address }}']
{{- if .Path }}
    metrics_path: '{{ .Path }}'
{{- end }}
{{- end }}
`

// GeneratePrometheusConfig generates prometheus.yml for enabled services.
func (g *Generator) GeneratePrometheusConfig(outputDir string) (string, error) {
	targets := make([]PrometheusTarget, 0, 10)

	// Add CBT engine targets for each enabled network
	for i, net := range g.cfg.EnabledNetworks() {
		metricsPort := cbtMetricsPortBase + i
		targets = append(targets, PrometheusTarget{
			JobName: fmt.Sprintf("cbt-%s", net.Name),
			Address: fmt.Sprintf("host.docker.internal:%d", metricsPort),
		})
	}

	// Add cbt-api targets for each enabled network
	for i, net := range g.cfg.EnabledNetworks() {
		metricsPort := cbtAPIMetricsPortBase + i
		targets = append(targets, PrometheusTarget{
			JobName: fmt.Sprintf("cbt-api-%s", net.Name),
			Address: fmt.Sprintf("host.docker.internal:%d", metricsPort),
		})
	}

	// Add lab-backend target (assuming it exposes metrics on /metrics)
	targets = append(targets, PrometheusTarget{
		JobName: "lab-backend",
		Address: fmt.Sprintf("host.docker.internal:%d", g.cfg.Ports.LabBackend),
		Path:    "/metrics",
	})

	data := PrometheusTemplateData{
		ScrapeInterval: "15s",
		Targets:        targets,
	}

	tmpl, err := template.New("prometheus-config").Parse(prometheusConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse Prometheus config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute Prometheus config template: %w", err)
	}

	// Write to output directory
	configPath := filepath.Join(outputDir, "prometheus.yml")

	//nolint:gosec // Config file permissions are intentionally 0644 for readability
	if err := os.WriteFile(configPath, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("failed to write prometheus.yml: %w", err)
	}

	g.log.WithField("path", configPath).Debug("generated prometheus.yml")

	return buf.String(), nil
}

// GenerateGrafanaProvisioning creates Grafana provisioning files for automatic setup.
// It generates the built-in lab dashboard and copies any custom dashboards from
// .xcli/custom-dashboards/ directory.
func (g *Generator) GenerateGrafanaProvisioning(outputDir, xcliDir string) error {
	// Create directory structure
	dirs := []string{
		filepath.Join(outputDir, "grafana", "provisioning", "datasources"),
		filepath.Join(outputDir, "grafana", "provisioning", "dashboards"),
		filepath.Join(outputDir, "grafana", "dashboards"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Generate datasource provisioning file
	datasourcePath := filepath.Join(outputDir, "grafana", "provisioning", "datasources", "datasource.yml")

	//nolint:gosec // Config file permissions are intentionally 0644 for readability
	if err := os.WriteFile(datasourcePath, []byte(grafanaDatasourceConfig), 0644); err != nil {
		return fmt.Errorf("failed to write datasource.yml: %w", err)
	}

	// Generate dashboard provider file
	dashboardProviderPath := filepath.Join(outputDir, "grafana", "provisioning", "dashboards", "dashboard.yml")

	//nolint:gosec // Config file permissions are intentionally 0644 for readability
	if err := os.WriteFile(dashboardProviderPath, []byte(grafanaDashboardProvider), 0644); err != nil {
		return fmt.Errorf("failed to write dashboard.yml: %w", err)
	}

	// Generate built-in lab dashboard
	dashboardPath := filepath.Join(outputDir, "grafana", "dashboards", "lab-overview.json")
	if err := g.generateLabDashboard(dashboardPath); err != nil {
		return fmt.Errorf("failed to generate lab dashboard: %w", err)
	}

	// Copy embedded default dashboards from the binary
	dashboardsDir := filepath.Join(outputDir, "grafana", "dashboards")
	if err := g.copyEmbeddedDashboards(dashboardsDir); err != nil {
		g.log.WithError(err).Warn("failed to copy embedded dashboards")
	}

	// Copy custom dashboards from .xcli/custom-dashboards/ if they exist
	// These can override embedded dashboards if they have the same name
	customDashboardsDir := filepath.Join(xcliDir, "custom-dashboards")
	if err := g.copyCustomDashboards(customDashboardsDir, dashboardsDir); err != nil {
		// Log warning but don't fail - custom dashboards are optional
		g.log.WithError(err).Debug("no custom dashboards copied")
	}

	g.log.WithField("path", outputDir).Debug("generated Grafana provisioning files")

	return nil
}

// copyCustomDashboards copies all .json files from the source directory to the destination.
func (g *Generator) copyCustomDashboards(srcDir, dstDir string) error {
	// Check if custom dashboards directory exists
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil // No custom dashboards directory, nothing to copy
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read custom dashboards directory: %w", err)
	}

	copiedCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only copy .json files
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		// Read source file
		data, readErr := os.ReadFile(srcPath)
		if readErr != nil {
			g.log.WithError(readErr).WithField("file", entry.Name()).Warn("failed to read custom dashboard")

			continue
		}

		// Write to destination
		//nolint:gosec // Config file permissions are intentionally 0644 for readability
		if writeErr := os.WriteFile(dstPath, data, 0644); writeErr != nil {
			g.log.WithError(writeErr).WithField("file", entry.Name()).Warn("failed to copy custom dashboard")

			continue
		}

		g.log.WithField("dashboard", entry.Name()).Debug("copied custom dashboard")

		copiedCount++
	}

	if copiedCount > 0 {
		g.log.WithField("count", copiedCount).Info("loaded custom Grafana dashboards")
	}

	return nil
}

// copyEmbeddedDashboards copies the default dashboards embedded in the binary to the destination.
func (g *Generator) copyEmbeddedDashboards(dstDir string) error {
	entries, err := dashboards.DefaultDashboards.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to read embedded dashboards: %w", err)
	}

	copiedCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only copy .json files
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		// Read from embedded filesystem
		data, readErr := dashboards.DefaultDashboards.ReadFile(entry.Name())
		if readErr != nil {
			g.log.WithError(readErr).WithField("file", entry.Name()).Warn("failed to read embedded dashboard")

			continue
		}

		dstPath := filepath.Join(dstDir, entry.Name())

		// Write to destination
		//nolint:gosec // Config file permissions are intentionally 0644 for readability
		if writeErr := os.WriteFile(dstPath, data, 0644); writeErr != nil {
			g.log.WithError(writeErr).WithField("file", entry.Name()).Warn("failed to copy embedded dashboard")

			continue
		}

		g.log.WithField("dashboard", entry.Name()).Debug("copied embedded dashboard")

		copiedCount++
	}

	if copiedCount > 0 {
		g.log.WithField("count", copiedCount).Info("loaded default Grafana dashboards")
	}

	return nil
}

// generateLabDashboard creates a pre-built Grafana dashboard for lab services.
func (g *Generator) generateLabDashboard(outputPath string) error {
	// Build panels dynamically based on enabled networks
	panels := g.buildDashboardPanels()

	dashboard := map[string]any{
		"annotations": map[string]any{
			"list": []any{},
		},
		"editable":             true,
		"fiscalYearStartMonth": 0,
		"graphTooltip":         0,
		"id":                   nil,
		"links":                []any{},
		"panels":               panels,
		"schemaVersion":        39,
		"tags":                 []string{"lab", "xcli"},
		"templating": map[string]any{
			"list": []any{},
		},
		"time": map[string]any{
			"from": "now-1h",
			"to":   "now",
		},
		"timepicker": map[string]any{},
		"timezone":   "browser",
		"title":      "Lab Overview",
		"uid":        "xcli-lab-overview",
		"version":    1,
		"weekStart":  "",
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(dashboard, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal dashboard: %w", err)
	}

	//nolint:gosec // Config file permissions are intentionally 0644 for readability
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write dashboard: %w", err)
	}

	return nil
}

// buildDashboardPanels creates Grafana dashboard panels for lab services.
func (g *Generator) buildDashboardPanels() []any {
	panels := make([]any, 0, 10)
	gridY := 0

	// Service health row
	panels = append(panels, map[string]any{
		"collapsed": false,
		"gridPos":   map[string]any{"h": 1, "w": 24, "x": 0, "y": gridY},
		"id":        1,
		"panels":    []any{},
		"title":     "Service Health",
		"type":      "row",
	})
	gridY++

	// Add up metrics panels for each enabled network
	panelID := 2
	gridX := 0

	for _, net := range g.cfg.EnabledNetworks() {
		// CBT engine up metric
		panels = append(panels, g.createStatPanel(
			panelID,
			fmt.Sprintf("CBT %s", net.Name),
			fmt.Sprintf("up{job=\"cbt-%s\"}", net.Name),
			gridX, gridY, 4, 3,
		))
		panelID++
		gridX += 4

		// cbt-api up metric
		panels = append(panels, g.createStatPanel(
			panelID,
			fmt.Sprintf("CBT API %s", net.Name),
			fmt.Sprintf("up{job=\"cbt-api-%s\"}", net.Name),
			gridX, gridY, 4, 3,
		))
		panelID++
		gridX += 4
	}

	// Lab backend up metric
	panels = append(panels, g.createStatPanel(
		panelID,
		"Lab Backend",
		"up{job=\"lab-backend\"}",
		gridX, gridY, 4, 3,
	))
	panelID++
	gridY += 3

	// Go runtime metrics row
	panels = append(panels, map[string]any{
		"collapsed": false,
		"gridPos":   map[string]any{"h": 1, "w": 24, "x": 0, "y": gridY},
		"id":        panelID,
		"panels":    []any{},
		"title":     "Go Runtime",
		"type":      "row",
	})
	panelID++
	gridY++

	// Goroutines panel
	panels = append(panels, g.createTimeSeriesPanel(
		panelID,
		"Goroutines",
		"go_goroutines",
		0, gridY, 12, 8,
	))
	panelID++

	// Memory panel
	panels = append(panels, g.createTimeSeriesPanel(
		panelID,
		"Memory (Alloc)",
		"go_memstats_alloc_bytes",
		12, gridY, 12, 8,
	))

	return panels
}

// createStatPanel creates a Grafana stat panel configuration.
func (g *Generator) createStatPanel(id int, title, query string, x, y, w, h int) map[string]any {
	return map[string]any{
		"datasource": map[string]any{
			"type": "prometheus",
			"uid":  "prometheus",
		},
		"fieldConfig": map[string]any{
			"defaults": map[string]any{
				"color": map[string]any{
					"mode": "thresholds",
				},
				"mappings": []any{
					map[string]any{
						"options": map[string]any{
							"0": map[string]any{
								"color": "red",
								"index": 1,
								"text":  "Down",
							},
							"1": map[string]any{
								"color": "green",
								"index": 0,
								"text":  "Up",
							},
						},
						"type": "value",
					},
				},
				"thresholds": map[string]any{
					"mode": "absolute",
					"steps": []any{
						map[string]any{"color": "red", "value": nil},
						map[string]any{"color": "green", "value": 1},
					},
				},
			},
			"overrides": []any{},
		},
		"gridPos": map[string]any{"h": h, "w": w, "x": x, "y": y},
		"id":      id,
		"options": map[string]any{
			"colorMode":   "value",
			"graphMode":   "none",
			"justifyMode": "auto",
			"orientation": "auto",
			"reduceOptions": map[string]any{
				"calcs":  []string{"lastNotNull"},
				"fields": "",
				"values": false,
			},
			"textMode": "auto",
		},
		"pluginVersion": "11.3.1",
		"targets": []any{
			map[string]any{
				"datasource": map[string]any{
					"type": "prometheus",
					"uid":  "prometheus",
				},
				"expr":         query,
				"instant":      true,
				"legendFormat": "__auto",
				"refId":        "A",
			},
		},
		"title": title,
		"type":  "stat",
	}
}

// createTimeSeriesPanel creates a Grafana time series panel configuration.
func (g *Generator) createTimeSeriesPanel(id int, title, query string, x, y, w, h int) map[string]any {
	return map[string]any{
		"datasource": map[string]any{
			"type": "prometheus",
			"uid":  "prometheus",
		},
		"fieldConfig": map[string]any{
			"defaults": map[string]any{
				"color": map[string]any{
					"mode": "palette-classic",
				},
				"custom": map[string]any{
					"axisBorderShow":   false,
					"axisCenteredZero": false,
					"axisColorMode":    "text",
					"axisLabel":        "",
					"axisPlacement":    "auto",
					"barAlignment":     0,
					"drawStyle":        "line",
					"fillOpacity":      10,
					"gradientMode":     "none",
					"hideFrom": map[string]any{
						"legend":  false,
						"tooltip": false,
						"viz":     false,
					},
					"insertNulls":       false,
					"lineInterpolation": "linear",
					"lineWidth":         1,
					"pointSize":         5,
					"scaleDistribution": map[string]any{
						"type": "linear",
					},
					"showPoints": "auto",
					"spanNulls":  false,
					"stacking": map[string]any{
						"group": "A",
						"mode":  "none",
					},
					"thresholdsStyle": map[string]any{
						"mode": "off",
					},
				},
				"mappings": []any{},
				"thresholds": map[string]any{
					"mode": "absolute",
					"steps": []any{
						map[string]any{"color": "green", "value": nil},
					},
				},
			},
			"overrides": []any{},
		},
		"gridPos": map[string]any{"h": h, "w": w, "x": x, "y": y},
		"id":      id,
		"options": map[string]any{
			"legend": map[string]any{
				"calcs":       []any{},
				"displayMode": "list",
				"placement":   "bottom",
				"showLegend":  true,
			},
			"tooltip": map[string]any{
				"mode": "single",
				"sort": "none",
			},
		},
		"targets": []any{
			map[string]any{
				"datasource": map[string]any{
					"type": "prometheus",
					"uid":  "prometheus",
				},
				"expr":         query,
				"legendFormat": "{{job}}",
				"refId":        "A",
			},
		},
		"title": title,
		"type":  "timeseries",
	}
}
