// Package seeddata provides functionality to generate seed data parquet files
// for xatu-cbt tests by extracting data from external ClickHouse.
package seeddata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
)

const (
	schemeHTTPS = "https"
)

// Generator handles seed data generation from external ClickHouse.
type Generator struct {
	log logrus.FieldLogger
	cfg *config.LabConfig
}

// GenerateOptions contains options for generating seed data.
type GenerateOptions struct {
	Model             string   // Table name (e.g., "beacon_api_eth_v1_events_block")
	Network           string   // Network name (e.g., "mainnet", "sepolia")
	Spec              string   // Fork spec (e.g., "pectra", "fusaka")
	RangeColumn       string   // Column to filter on (e.g., "slot", "epoch")
	From              string   // Range start value
	To                string   // Range end value
	Filters           []Filter // Additional filters
	FilterSQL         string   // Raw SQL fragment for additional WHERE conditions (from AI discovery)
	CorrelationFilter string   // Subquery filter for dimension tables (e.g., "validator_index IN (SELECT ...)")
	Limit             int      // Max rows (0 = unlimited)
	OutputPath        string   // Output file path
	SanitizeIPs       bool     // Enable IP address sanitization
	Salt              string   // Salt for IP sanitization (shared across batch for consistency)

	// sanitizedColumns is an internal field set by Generate() when SanitizeIPs is true.
	// It contains the pre-computed column list with IP sanitization expressions.
	sanitizedColumns string
}

// Filter represents an additional WHERE clause filter.
type Filter struct {
	Column   string // Column name
	Operator string // Operator (=, !=, >, <, >=, <=, LIKE, IN, etc.)
	Value    string // Value to compare against
}

// GenerateResult contains the result of seed data generation.
type GenerateResult struct {
	OutputPath       string   // Path to generated parquet file
	RowCount         int64    // Number of rows extracted (estimated from file size)
	FileSize         int64    // File size in bytes
	SanitizedColumns []string // IP columns that were sanitized (for display to user)
	Query            string   // SQL query used (for debugging)
}

// NewGenerator creates a new seed data generator.
func NewGenerator(log logrus.FieldLogger, cfg *config.LabConfig) *Generator {
	return &Generator{
		log: log.WithField("component", "seeddata"),
		cfg: cfg,
	}
}

// Generate extracts data from external ClickHouse and writes to a parquet file.
func (g *Generator) Generate(ctx context.Context, opts GenerateOptions) (*GenerateResult, error) {
	// Validate options
	if opts.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	if opts.Network == "" {
		return nil, fmt.Errorf("network is required")
	}

	// Build output path if not specified
	if opts.OutputPath == "" {
		opts.OutputPath = fmt.Sprintf("./%s.parquet", opts.Model)
	}

	// Ensure output directory exists
	dir := filepath.Dir(opts.OutputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Build sanitized column list if IP sanitization is enabled
	var sanitizedColumns []string

	if opts.SanitizeIPs && opts.Salt != "" {
		result, err := g.BuildSanitizedColumnList(ctx, opts.Model, opts.Salt)
		if err != nil {
			return nil, fmt.Errorf("failed to build sanitized column list: %w", err)
		}

		opts.sanitizedColumns = result.ColumnExpr
		sanitizedColumns = result.SanitizedColumns
	}

	// Build the SQL query
	query := g.buildQuery(opts)

	g.log.WithFields(logrus.Fields{
		"model":        opts.Model,
		"network":      opts.Network,
		"output":       opts.OutputPath,
		"range_column": opts.RangeColumn,
		"from":         opts.From,
		"to":           opts.To,
		"query":        query,
	}).Info("generating seed data")

	// Execute query and stream to file
	fileSize, err := g.executeQueryToFile(ctx, query, opts.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	return &GenerateResult{
		OutputPath:       opts.OutputPath,
		FileSize:         fileSize,
		SanitizedColumns: sanitizedColumns,
		Query:            query,
	}, nil
}

// ListExternalModels returns a list of available external models from the xatu-cbt repo.
func (g *Generator) ListExternalModels() ([]string, error) {
	modelsDir := filepath.Join(g.cfg.Repos.XatuCBT, "models", "external")

	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read models directory: %w", err)
	}

	models := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".sql") {
			// Remove .sql extension to get model name
			models = append(models, strings.TrimSuffix(name, ".sql"))
		}
	}

	return models, nil
}

// ValidateModel checks if a model name is valid (exists in xatu-cbt external models).
func (g *Generator) ValidateModel(model string) error {
	models, err := g.ListExternalModels()
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	for _, m := range models {
		if m == model {
			return nil
		}
	}

	return fmt.Errorf("model '%s' not found in xatu-cbt external models", model)
}

// buildQuery constructs the SQL query for extracting seed data.
func (g *Generator) buildQuery(opts GenerateOptions) string {
	var sb strings.Builder

	tableRef := g.resolveTableRef(opts.Model)

	// Use sanitized column list if available, otherwise SELECT *
	if opts.sanitizedColumns != "" {
		sb.WriteString("SELECT ")
		sb.WriteString(opts.sanitizedColumns)
		sb.WriteString(" FROM ")
	} else {
		sb.WriteString("SELECT * FROM ")
	}

	sb.WriteString(tableRef)
	sb.WriteString("\nWHERE meta_network_name = '")
	sb.WriteString(opts.Network)
	sb.WriteString("'")

	// Add range filter if specified
	// Use column-name-based detection (same logic as validation query in discovery.go)
	if opts.RangeColumn != "" && opts.From != "" && opts.To != "" {
		colLower := strings.ToLower(opts.RangeColumn)
		isTimeColumn := strings.Contains(colLower, "date") || strings.Contains(colLower, "time")

		sb.WriteString("\n  AND ")
		sb.WriteString(opts.RangeColumn)
		sb.WriteString(" >= ")

		if isTimeColumn {
			fmt.Fprintf(&sb, "toDateTime('%s')", opts.From)
		} else {
			sb.WriteString(opts.From) // Numeric value as-is
		}

		sb.WriteString("\n  AND ")
		sb.WriteString(opts.RangeColumn)
		sb.WriteString(" <= ")

		if isTimeColumn {
			fmt.Fprintf(&sb, "toDateTime('%s')", opts.To)
		} else {
			sb.WriteString(opts.To) // Numeric value as-is
		}
	}

	// Add additional filters (structured)
	for _, filter := range opts.Filters {
		sb.WriteString("\n  AND ")
		sb.WriteString(filter.Column)
		sb.WriteString(" ")
		sb.WriteString(filter.Operator)
		sb.WriteString(" ")
		sb.WriteString(formatSQLValue(filter.Value))
	}

	// Add raw SQL filter if specified (from AI discovery)
	if opts.FilterSQL != "" {
		sb.WriteString("\n  AND ")
		sb.WriteString(opts.FilterSQL)
	}

	// Add correlation filter if specified (subquery for dimension tables)
	if opts.CorrelationFilter != "" {
		sb.WriteString("\n  AND ")
		sb.WriteString(opts.CorrelationFilter)
	}

	// Add limit if specified
	if opts.Limit > 0 {
		fmt.Fprintf(&sb, "\nLIMIT %d", opts.Limit)
	}

	sb.WriteString("\nFORMAT Parquet")

	return sb.String()
}

// executeQueryToFile executes a query and streams the result to a file.
func (g *Generator) executeQueryToFile(ctx context.Context, query, outputPath string) (int64, error) {
	// Parse external ClickHouse URL
	chURL, err := g.buildClickHouseHTTPURL()
	if err != nil {
		return 0, fmt.Errorf("failed to build ClickHouse URL: %w", err)
	}

	g.log.WithField("query", query).Debug("executing query")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chURL, strings.NewReader(query))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Minute, // Allow long queries
	}

	// Execute request
	resp, err := client.Do(req) //nolint:gosec // URL is constructed from trusted config, not user input
	if err != nil {
		return 0, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return 0, fmt.Errorf("ClickHouse returned status %d: %s", resp.StatusCode, string(body))
	}

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Stream response to file
	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to write output file: %w", err)
	}

	return written, nil
}

// buildClickHouseHTTPURL constructs the HTTP URL for ClickHouse queries.
func (g *Generator) buildClickHouseHTTPURL() (string, error) {
	externalURL := g.cfg.Infrastructure.ClickHouse.Xatu.ExternalURL

	// Parse the configured URL
	parsed, err := url.Parse(externalURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse external URL: %w", err)
	}

	// Convert to HTTP URL if needed
	// The external URL might be in native protocol format (port 9000)
	// We need HTTP port (typically 8123 or 8443 for HTTPS)
	host := parsed.Hostname()
	port := parsed.Port()

	// Determine scheme and port for HTTP API
	scheme := "http"

	if parsed.Scheme == schemeHTTPS || parsed.Scheme == "clickhouses" {
		scheme = schemeHTTPS
	}

	// Map native port to HTTP port if needed
	switch port {
	case "9000":
		port = "8123" // Default HTTP port
	case "9440":
		port = "8443" // Default HTTPS port
	case "":
		if scheme == schemeHTTPS {
			port = "8443"
		} else {
			port = "8123"
		}
	}

	// Build HTTP URL with query parameters
	httpURL := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   "/",
	}

	// Add authentication if configured
	query := httpURL.Query()

	if g.cfg.Infrastructure.ClickHouse.Xatu.ExternalUsername != "" {
		query.Set("user", g.cfg.Infrastructure.ClickHouse.Xatu.ExternalUsername)
	} else if parsed.User != nil && parsed.User.Username() != "" {
		query.Set("user", parsed.User.Username())
	} else {
		query.Set("user", "default")
	}

	if g.cfg.Infrastructure.ClickHouse.Xatu.ExternalPassword != "" {
		query.Set("password", g.cfg.Infrastructure.ClickHouse.Xatu.ExternalPassword)
	} else if parsed.User != nil {
		if pass, ok := parsed.User.Password(); ok {
			query.Set("password", pass)
		}
	}

	// Set database
	if g.cfg.Infrastructure.ClickHouse.Xatu.ExternalDatabase != "" {
		query.Set("database", g.cfg.Infrastructure.ClickHouse.Xatu.ExternalDatabase)
	} else {
		query.Set("database", "default")
	}

	httpURL.RawQuery = query.Encode()

	return httpURL.String(), nil
}

// resolveTableRef returns the fully qualified "database.table" reference for an external model.
// If the model's frontmatter specifies a database and table, it uses those.
// Otherwise it falls back to "default.modelName" for backward compatibility.
func (g *Generator) resolveTableRef(model string) string {
	return ResolveExternalTableRef(model, g.cfg.Repos.XatuCBT)
}

// formatSQLValue formats a value for use in SQL.
// Numeric values are returned as-is, datetime values use toDateTime(), other values are quoted.
func formatSQLValue(val string) string {
	// Check if value is purely numeric (integer or decimal)
	if isNumeric(val) {
		return val
	}

	// Check if value looks like a datetime (YYYY-MM-DD HH:MM:SS)
	if isDateTime(val) {
		return fmt.Sprintf("toDateTime('%s')", val)
	}

	// Quote non-numeric values (strings, etc.)
	// Escape single quotes by doubling them
	escaped := strings.ReplaceAll(val, "'", "''")

	return "'" + escaped + "'"
}

// isDateTime checks if a string looks like a datetime (YYYY-MM-DD HH:MM:SS).
func isDateTime(s string) bool {
	// Must be exactly 19 characters: YYYY-MM-DD HH:MM:SS
	if len(s) != 19 {
		return false
	}

	// Check format: YYYY-MM-DD HH:MM:SS
	// Positions: 0123456789012345678
	//            2025-12-10 20:00:00
	if s[4] != '-' || s[7] != '-' || s[10] != ' ' || s[13] != ':' || s[16] != ':' {
		return false
	}

	// Check that other positions are digits
	digitPositions := []int{0, 1, 2, 3, 5, 6, 8, 9, 11, 12, 14, 15, 17, 18}
	for _, pos := range digitPositions {
		if s[pos] < '0' || s[pos] > '9' {
			return false
		}
	}

	return true
}

// isNumeric checks if a string represents a numeric value.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}

	// Allow leading minus sign
	start := 0
	if s[0] == '-' {
		start = 1

		if len(s) == 1 {
			return false
		}
	}

	hasDecimal := false

	for i := start; i < len(s); i++ {
		c := s[i]
		if c == '.' {
			if hasDecimal {
				return false // Multiple decimals
			}

			hasDecimal = true

			continue
		}

		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}
