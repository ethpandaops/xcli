package seeddata

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// saltLength is the number of random bytes used for salt generation.
	saltLength = 32
)

// ColumnInfo represents a column's name and type from ClickHouse schema.
type ColumnInfo struct {
	Name string
	Type string
}

// SanitizedColumnResult contains the result of building a sanitized column list.
type SanitizedColumnResult struct {
	ColumnExpr       string   // Comma-separated column expressions for SELECT
	SanitizedColumns []string // Names of columns that were sanitized (for display)
}

// describeTableResponse represents the ClickHouse DESCRIBE TABLE JSON response.
type describeTableResponse struct {
	Data []describeTableRow `json:"data"`
}

// describeTableRow represents a single row from DESCRIBE TABLE.
type describeTableRow struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// GenerateSalt creates a cryptographically random salt for IP sanitization.
// The salt should be generated once per seed data generation run and shared
// across all models to ensure consistent IP anonymization.
func GenerateSalt() (string, error) {
	bytes := make([]byte, saltLength)

	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random salt: %w", err)
	}

	return hex.EncodeToString(bytes), nil
}

// DescribeTable queries ClickHouse to get the schema for a table.
func (g *Generator) DescribeTable(ctx context.Context, model string) ([]ColumnInfo, error) {
	tableRef := g.resolveTableRef(model)
	query := fmt.Sprintf("DESCRIBE TABLE %s FORMAT JSON", tableRef)

	chURL, err := g.buildClickHouseHTTPURL()
	if err != nil {
		return nil, fmt.Errorf("failed to build ClickHouse URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chURL, strings.NewReader(query))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req) //nolint:gosec // URL is from trusted config
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("ClickHouse returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var descResp describeTableResponse
	if unmarshalErr := json.Unmarshal(body, &descResp); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", unmarshalErr)
	}

	columns := make([]ColumnInfo, 0, len(descResp.Data))

	for _, row := range descResp.Data {
		columns = append(columns, ColumnInfo(row))
	}

	return columns, nil
}

// BuildSanitizedColumnList builds a complete SELECT column list with IP sanitization.
// Returns the column expressions and a list of which columns were sanitized.
func (g *Generator) BuildSanitizedColumnList(ctx context.Context, model, salt string) (*SanitizedColumnResult, error) {
	columns, err := g.DescribeTable(ctx, model)
	if err != nil {
		return nil, fmt.Errorf("failed to describe table %s: %w", model, err)
	}

	if len(columns) == 0 {
		return nil, fmt.Errorf("table %s has no columns", model)
	}

	// Find IP columns for reporting
	sanitizedCols := make([]string, 0, len(columns))

	for _, col := range columns {
		if IsIPColumn(col.Type) {
			sanitizedCols = append(sanitizedCols, fmt.Sprintf("%s (%s)", col.Name, col.Type))
		}
	}

	// Build column expressions
	exprs := make([]string, 0, len(columns))

	for _, col := range columns {
		expr := BuildSanitizedColumnExpr(col, salt)
		exprs = append(exprs, expr)
	}

	return &SanitizedColumnResult{
		ColumnExpr:       strings.Join(exprs, ", "),
		SanitizedColumns: sanitizedCols,
	}, nil
}

// IsIPColumn checks if a column type is an IP address type (IPv4 or IPv6).
// Handles both direct types and Nullable wrappers.
func IsIPColumn(colType string) bool {
	// Normalize the type string for comparison
	normalized := strings.TrimSpace(colType)

	// Direct IP types
	if normalized == "IPv4" || normalized == "IPv6" {
		return true
	}

	// Nullable IP types
	if normalized == "Nullable(IPv4)" || normalized == "Nullable(IPv6)" {
		return true
	}

	return false
}

// IsIPv4Column checks if a column is specifically IPv4 type.
func IsIPv4Column(colType string) bool {
	normalized := strings.TrimSpace(colType)

	return normalized == "IPv4" || normalized == "Nullable(IPv4)"
}

// IsNullableIPColumn checks if an IP column is nullable.
func IsNullableIPColumn(colType string) bool {
	return strings.HasPrefix(strings.TrimSpace(colType), "Nullable(")
}

// BuildSanitizedColumnExpr builds a SQL expression for a column with IP sanitization if needed.
// Preserves IP types: IPv4 → IPv4, IPv6 → IPv6, IPv4-mapped-IPv6 → IPv4-mapped-IPv6.
func BuildSanitizedColumnExpr(col ColumnInfo, salt string) string {
	if !IsIPColumn(col.Type) {
		// Non-IP column: select as-is
		return col.Name
	}

	// Escape the salt for SQL (single quotes doubled)
	escapedSalt := strings.ReplaceAll(salt, "'", "''")

	// IPv4 columns: straightforward hash to IPv4
	if IsIPv4Column(col.Type) {
		if IsNullableIPColumn(col.Type) {
			// Nullable(IPv4): preserve NULL, hash non-NULL to IPv4
			return fmt.Sprintf(
				"if(%s IS NOT NULL, toIPv4(reinterpret(substring(sipHash128(%s, '%s'), 1, 4), 'UInt32')), NULL) AS %s",
				col.Name, col.Name, escapedSalt, col.Name,
			)
		}

		// Non-nullable IPv4: hash to IPv4
		return fmt.Sprintf(
			"toIPv4(reinterpret(substring(sipHash128(%s, '%s'), 1, 4), 'UInt32')) AS %s",
			col.Name, escapedSalt, col.Name,
		)
	}

	// IPv6 columns: detect IPv4-mapped addresses and preserve their format
	// IPv4-mapped addresses start with '::ffff:' when converted to string
	if IsNullableIPColumn(col.Type) {
		// Nullable(IPv6): preserve NULL, detect IPv4-mapped vs native IPv6
		return fmt.Sprintf(
			"if(%s IS NOT NULL, "+
				"if(startsWith(IPv6NumToString(%s), '::ffff:'), "+
				"IPv4ToIPv6(toIPv4(reinterpret(substring(sipHash128(%s, '%s'), 1, 4), 'UInt32'))), "+
				"CAST(reinterpret(sipHash128(%s, '%s'), 'FixedString(16)') AS IPv6)), "+
				"NULL) AS %s",
			col.Name, col.Name, col.Name, escapedSalt, col.Name, escapedSalt, col.Name,
		)
	}

	// Non-nullable IPv6: detect IPv4-mapped vs native IPv6
	return fmt.Sprintf(
		"if(startsWith(IPv6NumToString(%s), '::ffff:'), "+
			"IPv4ToIPv6(toIPv4(reinterpret(substring(sipHash128(%s, '%s'), 1, 4), 'UInt32'))), "+
			"CAST(reinterpret(sipHash128(%s, '%s'), 'FixedString(16)') AS IPv6)) AS %s",
		col.Name, col.Name, escapedSalt, col.Name, escapedSalt, col.Name,
	)
}

// CountIPColumns counts the number of IP columns in a table schema.
// Useful for logging/debugging.
func CountIPColumns(columns []ColumnInfo) int {
	count := 0

	for _, col := range columns {
		if IsIPColumn(col.Type) {
			count++
		}
	}

	return count
}

// GetIPColumnNames returns the names of all IP columns in a schema.
// Useful for logging/debugging.
func GetIPColumnNames(columns []ColumnInfo) []string {
	names := make([]string, 0, len(columns))

	for _, col := range columns {
		if IsIPColumn(col.Type) {
			names = append(names, col.Name)
		}
	}

	return names
}
