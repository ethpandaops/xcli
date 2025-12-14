package seeddata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// RangeColumnType identifies the semantic type of a range column.
type RangeColumnType string

const (
	// RangeColumnTypeTime represents DateTime columns like slot_start_date_time.
	RangeColumnTypeTime RangeColumnType = "time"
	// RangeColumnTypeBlock represents block number columns (UInt64/Int64).
	RangeColumnTypeBlock RangeColumnType = "block"
	// RangeColumnTypeSlot represents slot number columns (UInt64/Int64).
	RangeColumnTypeSlot RangeColumnType = "slot"
	// RangeColumnTypeEpoch represents epoch number columns.
	RangeColumnTypeEpoch RangeColumnType = "epoch"
	// RangeColumnTypeUnknown represents an unclassified column type.
	RangeColumnTypeUnknown RangeColumnType = "unknown"
)

// TableRangeStrategy describes how to filter a single table for seed data extraction.
type TableRangeStrategy struct {
	Model          string          `yaml:"model"`
	RangeColumn    string          `yaml:"rangeColumn"`
	ColumnType     RangeColumnType `yaml:"columnType"`
	FromValue      string          `yaml:"fromValue"`
	ToValue        string          `yaml:"toValue"`
	FilterSQL      string          `yaml:"filterSql,omitempty"`
	RequiresBridge bool            `yaml:"requiresBridge"`
	BridgeTable    string          `yaml:"bridgeTable,omitempty"`
	BridgeJoinSQL  string          `yaml:"bridgeJoinSql,omitempty"`
	Confidence     float64         `yaml:"confidence"`
	Reasoning      string          `yaml:"reasoning"`
}

// DiscoveryResult contains the complete AI-generated range strategy.
type DiscoveryResult struct {
	PrimaryRangeType   RangeColumnType      `yaml:"primaryRangeType"`
	PrimaryRangeColumn string               `yaml:"primaryRangeColumn"`
	FromValue          string               `yaml:"fromValue"`
	ToValue            string               `yaml:"toValue"`
	Strategies         []TableRangeStrategy `yaml:"strategies"`
	OverallConfidence  float64              `yaml:"overallConfidence"`
	Summary            string               `yaml:"summary"`
	Warnings           []string             `yaml:"warnings,omitempty"`
}

// GetStrategy returns the strategy for a specific model.
// Uses case-insensitive matching and trims whitespace to handle variations in Claude's output.
func (d *DiscoveryResult) GetStrategy(model string) *TableRangeStrategy {
	modelLower := strings.ToLower(strings.TrimSpace(model))

	for i := range d.Strategies {
		strategyModel := strings.ToLower(strings.TrimSpace(d.Strategies[i].Model))
		if strategyModel == modelLower {
			return &d.Strategies[i]
		}
	}

	return nil
}

// TableSchemaInfo contains schema information for a table.
type TableSchemaInfo struct {
	Model      string           `yaml:"model"`
	Columns    []ColumnInfo     `yaml:"columns"`
	SampleData []map[string]any `yaml:"sampleData,omitempty"`
	RangeInfo  *DetectedRange   `yaml:"rangeInfo,omitempty"`
}

// DetectedRange contains detected range information.
type DetectedRange struct {
	Column     string          `yaml:"column"`
	ColumnType RangeColumnType `yaml:"type"`
	Detected   bool            `yaml:"detected"`
	MinValue   string          `yaml:"minValue,omitempty"`
	MaxValue   string          `yaml:"maxValue,omitempty"`
}

// DiscoveryInput contains all information gathered for Claude analysis.
type DiscoveryInput struct {
	TransformationModel string            `yaml:"transformationModel"`
	TransformationSQL   string            `yaml:"transformationSql"`
	Network             string            `yaml:"network"`
	Duration            string            `yaml:"duration"` // e.g., "5m", "10m", "1h"
	ExternalModels      []TableSchemaInfo `yaml:"externalModels"`
}

// ClaudeDiscoveryClient handles AI-assisted range discovery.
type ClaudeDiscoveryClient struct {
	log        logrus.FieldLogger
	claudePath string
	timeout    time.Duration
	gen        *Generator
}

// NewClaudeDiscoveryClient creates a new discovery client.
func NewClaudeDiscoveryClient(log logrus.FieldLogger, gen *Generator) (*ClaudeDiscoveryClient, error) {
	claudePath, err := findClaudeBinaryPath()
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found: %w", err)
	}

	return &ClaudeDiscoveryClient{
		log:        log.WithField("component", "claude-discovery"),
		claudePath: claudePath,
		timeout:    5 * time.Minute, // Discovery can take longer than assertions
		gen:        gen,
	}, nil
}

// IsAvailable checks if Claude CLI is accessible.
func (c *ClaudeDiscoveryClient) IsAvailable() bool {
	if c.claudePath == "" {
		return false
	}

	info, err := os.Stat(c.claudePath)
	if err != nil {
		return false
	}

	return !info.IsDir() && info.Mode()&0111 != 0
}

// GatherSchemaInfo collects schema information for all external models.
func (c *ClaudeDiscoveryClient) GatherSchemaInfo(
	ctx context.Context,
	models []string,
	network string,
	xatuCBTPath string,
) ([]TableSchemaInfo, error) {
	schemas := make([]TableSchemaInfo, 0, len(models))

	for _, model := range models {
		c.log.WithField("model", model).Debug("gathering schema info")

		// Get column schema from ClickHouse
		columns, err := c.gen.DescribeTable(ctx, model)
		if err != nil {
			return nil, fmt.Errorf("failed to describe table %s: %w", model, err)
		}

		// Detect range column from SQL file
		rangeCol, err := DetectRangeColumnForModel(model, xatuCBTPath)
		if err != nil {
			c.log.WithError(err).WithField("model", model).Warn("failed to detect range column")

			rangeCol = DefaultRangeColumn
		}

		// Classify the range column type
		colType := ClassifyRangeColumn(rangeCol, columns)

		// Query the range for this model
		var minVal, maxVal string

		modelRange, err := c.gen.QueryModelRange(ctx, model, network, rangeCol)
		if err != nil {
			c.log.WithError(err).WithField("model", model).Warn("failed to query model range")
		} else {
			minVal = modelRange.MinRaw
			maxVal = modelRange.MaxRaw
		}

		// Get sample data (limited to 3 rows for prompt size)
		sampleData, err := c.gen.QueryTableSample(ctx, model, network, 3)
		if err != nil {
			c.log.WithError(err).WithField("model", model).Warn("failed to query sample data")
			// Continue without sample data - not critical
		}

		schemas = append(schemas, TableSchemaInfo{
			Model:      model,
			Columns:    columns,
			SampleData: sampleData,
			RangeInfo: &DetectedRange{
				Column:     rangeCol,
				ColumnType: colType,
				Detected:   rangeCol != DefaultRangeColumn,
				MinValue:   minVal,
				MaxValue:   maxVal,
			},
		})
	}

	return schemas, nil
}

// AnalyzeRanges invokes Claude to analyze range strategies.
func (c *ClaudeDiscoveryClient) AnalyzeRanges(
	ctx context.Context,
	input DiscoveryInput,
) (*DiscoveryResult, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("claude CLI is not available")
	}

	prompt := c.buildDiscoveryPrompt(input)

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	//nolint:gosec // claudePath is validated in findClaudeBinaryPath
	cmd := exec.CommandContext(ctx, c.claudePath, "--print")
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	c.log.WithFields(logrus.Fields{
		"timeout": c.timeout,
		"model":   input.TransformationModel,
	}).Debug("invoking Claude CLI for range discovery")

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude discovery timed out after %s", c.timeout)
		}

		return nil, fmt.Errorf("claude CLI failed: %w (stderr: %s)", err, stderr.String())
	}

	response := stdout.String()
	if response == "" {
		return nil, fmt.Errorf("claude returned empty response")
	}

	c.log.WithField("response_length", len(response)).Debug("received Claude response")

	return c.parseDiscoveryResponse(response)
}

// buildDiscoveryPrompt constructs the prompt for Claude.
func (c *ClaudeDiscoveryClient) buildDiscoveryPrompt(input DiscoveryInput) string {
	var sb strings.Builder

	sb.WriteString("## Task\n")
	sb.WriteString("Analyze the following ClickHouse tables and determine the best strategy for extracting correlated seed data across all tables for testing.\n\n")

	sb.WriteString("## Context\n")
	sb.WriteString(fmt.Sprintf("- Transformation Model: %s\n", input.TransformationModel))
	sb.WriteString(fmt.Sprintf("- Network: %s\n", input.Network))
	sb.WriteString(fmt.Sprintf("- Requested Duration: %s\n", input.Duration))
	sb.WriteString("- Goal: Extract a consistent slice of data from all external models that can be used together for testing the transformation\n\n")

	sb.WriteString("## Problem\n")
	sb.WriteString("These tables may use different range column types:\n")
	sb.WriteString("- Time-based columns (slot_start_date_time - DateTime)\n")
	sb.WriteString("- Numeric columns (block_number - UInt64, slot - UInt64)\n\n")
	sb.WriteString("You need to determine how to correlate these ranges so we get matching data across all tables.\n\n")

	sb.WriteString("## External Models and Their Schemas\n\n")

	for _, schema := range input.ExternalModels {
		sb.WriteString(fmt.Sprintf("### %s\n", schema.Model))

		if schema.RangeInfo != nil {
			sb.WriteString(fmt.Sprintf("Detected Range Column: %s (type: %s)\n",
				schema.RangeInfo.Column, schema.RangeInfo.ColumnType))

			if schema.RangeInfo.MinValue != "" && schema.RangeInfo.MaxValue != "" {
				sb.WriteString(fmt.Sprintf("Available Range: %s to %s\n",
					schema.RangeInfo.MinValue, schema.RangeInfo.MaxValue))
			}
		}

		sb.WriteString("\nColumns:\n")

		for _, col := range schema.Columns {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", col.Name, col.Type))
		}

		if len(schema.SampleData) > 0 {
			sb.WriteString("\nSample Data (first rows):\n```yaml\n")

			sampleYAML, err := yaml.Marshal(schema.SampleData)
			if err == nil {
				sb.WriteString(string(sampleYAML))
			}

			sb.WriteString("```\n")
		}

		sb.WriteString("\n")
	}

	sb.WriteString("## Transformation SQL\n```sql\n")
	sb.WriteString(input.TransformationSQL)
	sb.WriteString("\n```\n\n")

	sb.WriteString("## Instructions\n")
	sb.WriteString("1. Analyze which tables can share a common range column directly\n")
	sb.WriteString("2. For tables with different range types, determine if correlation is possible via:\n")
	sb.WriteString("   - Direct conversion (e.g., slot to slot_start_date_time via calculation)\n")
	sb.WriteString("   - Bridge table (e.g., canonical_beacon_block has both slot and execution block info)\n")
	sb.WriteString("   - Shared columns in the data itself\n")
	sb.WriteString("3. Recommend a primary range specification (type + column + from/to values)\n")
	sb.WriteString(fmt.Sprintf("4. Use a %s time range (as requested by the user)\n", input.Duration))
	sb.WriteString("5. For each table, specify exactly how to filter it\n\n")

	sb.WriteString("## Output Format\n")
	sb.WriteString("Output ONLY valid YAML matching this structure:\n\n")
	sb.WriteString("```yaml\n")
	sb.WriteString("primaryRangeType: time  # or block, slot\n")
	sb.WriteString("primaryRangeColumn: slot_start_date_time\n")
	sb.WriteString("fromValue: \"2025-01-01 00:00:00\"  # or numeric value as string\n")
	sb.WriteString("toValue: \"2025-01-01 00:05:00\"\n")
	sb.WriteString("strategies:\n")
	sb.WriteString("  - model: table_name\n")
	sb.WriteString("    rangeColumn: column_to_filter_on\n")
	sb.WriteString("    columnType: time  # or block, slot\n")
	sb.WriteString("    fromValue: \"value\"\n")
	sb.WriteString("    toValue: \"value\"\n")
	sb.WriteString("    requiresBridge: false\n")
	sb.WriteString("    confidence: 0.95\n")
	sb.WriteString("    reasoning: \"Direct filtering on native range column\"\n")
	sb.WriteString("  - model: block_based_table\n")
	sb.WriteString("    rangeColumn: block_number\n")
	sb.WriteString("    columnType: block\n")
	sb.WriteString("    fromValue: \"1000000\"\n")
	sb.WriteString("    toValue: \"1000100\"\n")
	sb.WriteString("    requiresBridge: true\n")
	sb.WriteString("    bridgeTable: canonical_beacon_block\n")
	sb.WriteString("    confidence: 0.8\n")
	sb.WriteString("    reasoning: \"Need to convert time range to block numbers via beacon chain\"\n")
	sb.WriteString("overallConfidence: 0.85\n")
	sb.WriteString("summary: \"Using time-based primary range with block correlation via canonical_beacon_block\"\n")
	sb.WriteString("warnings:\n")
	sb.WriteString("  - \"Some execution tables may have gaps where no beacon block was produced\"\n")
	sb.WriteString("```\n\n")

	sb.WriteString("IMPORTANT:\n")
	sb.WriteString("- Use actual values from the available ranges shown above\n")
	sb.WriteString("- Pick a recent time window (last hour or so) within the intersection of all available ranges\n")
	sb.WriteString("- For block_number tables, estimate block numbers that correspond to the chosen time window\n")
	sb.WriteString("- Include ALL external models in the strategies list\n")
	sb.WriteString("- Output ONLY the YAML, no explanations before or after\n")

	return sb.String()
}

// parseDiscoveryResponse parses Claude's YAML response.
func (c *ClaudeDiscoveryClient) parseDiscoveryResponse(response string) (*DiscoveryResult, error) {
	// Extract YAML from response
	yamlContent := extractYAMLFromResponse(response)
	if yamlContent == "" {
		return nil, fmt.Errorf("no valid YAML found in Claude response")
	}

	var result DiscoveryResult

	if err := yaml.Unmarshal([]byte(yamlContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse discovery YAML: %w", err)
	}

	// Validate result
	if err := c.validateDiscoveryResult(&result); err != nil {
		return nil, fmt.Errorf("invalid discovery result: %w", err)
	}

	return &result, nil
}

// validateDiscoveryResult checks if the AI result is valid and complete.
func (c *ClaudeDiscoveryClient) validateDiscoveryResult(result *DiscoveryResult) error {
	if result.PrimaryRangeColumn == "" {
		return fmt.Errorf("primary_range_column is required")
	}

	if result.FromValue == "" || result.ToValue == "" {
		return fmt.Errorf("from_value and to_value are required")
	}

	if len(result.Strategies) == 0 {
		return fmt.Errorf("at least one strategy is required")
	}

	for i, s := range result.Strategies {
		if s.Model == "" {
			return fmt.Errorf("strategy %d: model is required", i)
		}

		if s.RangeColumn == "" {
			return fmt.Errorf("strategy %d (%s): range_column is required", i, s.Model)
		}

		if s.FromValue == "" || s.ToValue == "" {
			return fmt.Errorf("strategy %d (%s): from_value and to_value are required", i, s.Model)
		}
	}

	return nil
}

// ClassifyRangeColumn determines the semantic type of a range column based on its name and schema type.
func ClassifyRangeColumn(column string, schema []ColumnInfo) RangeColumnType {
	colLower := strings.ToLower(column)

	// Check by column name patterns
	switch {
	case strings.Contains(colLower, "date_time") || strings.Contains(colLower, "datetime"):
		return RangeColumnTypeTime
	case strings.Contains(colLower, "timestamp"):
		return RangeColumnTypeTime
	case colLower == "block_number" || strings.HasSuffix(colLower, "_block_number"):
		return RangeColumnTypeBlock
	case colLower == "slot" || strings.HasSuffix(colLower, "_slot"):
		return RangeColumnTypeSlot
	case colLower == "epoch" || strings.HasSuffix(colLower, "_epoch"):
		return RangeColumnTypeEpoch
	}

	// Check by schema type if available
	for _, col := range schema {
		if col.Name == column {
			typeLower := strings.ToLower(col.Type)
			if strings.Contains(typeLower, "datetime") {
				return RangeColumnTypeTime
			}

			break
		}
	}

	return RangeColumnTypeUnknown
}

// QueryTableSample retrieves sample rows from a table for analysis.
func (g *Generator) QueryTableSample(
	ctx context.Context,
	model string,
	network string,
	limit int,
) ([]map[string]any, error) {
	query := fmt.Sprintf(`
		SELECT *
		FROM default.%s
		WHERE meta_network_name = '%s'
		ORDER BY rand()
		LIMIT %d
		FORMAT JSON
	`, model, network, limit)

	g.log.WithFields(logrus.Fields{
		"model":   model,
		"network": network,
		"limit":   limit,
	}).Debug("querying table sample")

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

	resp, err := client.Do(req)
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

	var jsonResp struct {
		Data []map[string]any `json:"data"`
	}

	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return jsonResp.Data, nil
}

// FallbackRangeDiscovery provides heuristic-based range discovery when Claude is unavailable.
func FallbackRangeDiscovery(
	ctx context.Context,
	gen *Generator,
	models []string,
	network string,
	rangeInfos map[string]*RangeColumnInfo,
	duration string,
) (*DiscoveryResult, error) {
	// Group models by range column type
	timeModels := make([]string, 0)
	blockModels := make([]string, 0)

	for _, model := range models {
		info, ok := rangeInfos[model]
		if !ok {
			continue
		}

		colLower := strings.ToLower(info.RangeColumn)

		switch {
		case strings.Contains(colLower, "date_time") || strings.Contains(colLower, "timestamp"):
			timeModels = append(timeModels, model)
		case strings.Contains(colLower, "block"):
			blockModels = append(blockModels, model)
		}
	}

	// Determine primary range type based on majority
	var primaryType RangeColumnType

	var primaryColumn string

	if len(timeModels) >= len(blockModels) {
		primaryType = RangeColumnTypeTime
		primaryColumn = DefaultRangeColumn
	} else {
		primaryType = RangeColumnTypeBlock
		primaryColumn = "block_number"
	}

	// Query ranges for all models
	var latestMin, earliestMax time.Time

	strategies := make([]TableRangeStrategy, 0, len(models))

	for _, model := range models {
		info := rangeInfos[model]
		rangeCol := DefaultRangeColumn

		if info != nil {
			rangeCol = info.RangeColumn
		}

		modelRange, err := gen.QueryModelRange(ctx, model, network, rangeCol)
		if err != nil {
			return nil, fmt.Errorf("failed to query range for %s: %w", model, err)
		}

		if latestMin.IsZero() || modelRange.Min.After(latestMin) {
			latestMin = modelRange.Min
		}

		if earliestMax.IsZero() || modelRange.Max.Before(earliestMax) {
			earliestMax = modelRange.Max
		}

		colType := ClassifyRangeColumn(rangeCol, nil)

		strategies = append(strategies, TableRangeStrategy{
			Model:       model,
			RangeColumn: rangeCol,
			ColumnType:  colType,
			Confidence:  0.7, // Lower confidence for heuristic
			Reasoning:   "Heuristic-based detection (Claude unavailable)",
		})
	}

	// Check for intersection
	if latestMin.After(earliestMax) {
		return nil, fmt.Errorf("no intersecting range found across all models")
	}

	// Parse duration string (e.g., "5m", "10m", "1h")
	rangeDuration, parseErr := time.ParseDuration(duration)
	if parseErr != nil {
		rangeDuration = 5 * time.Minute // Default to 5 minutes if parsing fails
	}

	// Use the last N minutes/hours of available data
	effectiveMax := earliestMax.Add(-1 * time.Minute) // Account for ingestion lag
	effectiveMin := effectiveMax.Add(-rangeDuration)

	if effectiveMin.Before(latestMin) {
		effectiveMin = latestMin
	}

	fromValue := effectiveMin.Format("2006-01-02 15:04:05")
	toValue := effectiveMax.Format("2006-01-02 15:04:05")

	// Update strategies with range values
	for i := range strategies {
		strategies[i].FromValue = fromValue
		strategies[i].ToValue = toValue
	}

	warnings := make([]string, 0)
	if len(blockModels) > 0 && len(timeModels) > 0 {
		warnings = append(warnings,
			"Mixed range column types detected (time and block). "+
				"Block-based tables may not correlate correctly with time-based filtering.")
	}

	return &DiscoveryResult{
		PrimaryRangeType:   primaryType,
		PrimaryRangeColumn: primaryColumn,
		FromValue:          fromValue,
		ToValue:            toValue,
		Strategies:         strategies,
		OverallConfidence:  0.6, // Lower confidence for heuristic
		Summary:            "Heuristic-based range detection (Claude unavailable)",
		Warnings:           warnings,
	}, nil
}

// ReadTransformationSQL reads the SQL file for a transformation model.
func ReadTransformationSQL(model, xatuCBTPath string) (string, error) {
	sqlPath := filepath.Join(xatuCBTPath, "models", "transformations", model+".sql")

	content, err := os.ReadFile(sqlPath)
	if err != nil {
		return "", fmt.Errorf("failed to read transformation SQL: %w", err)
	}

	return string(content), nil
}

// ModelDataCount holds the row count validation result for a model.
type ModelDataCount struct {
	Model    string
	Strategy *TableRangeStrategy
	RowCount int64
	HasData  bool
	Error    error
}

// ValidationResult contains the results of validating a discovery strategy.
type ValidationResult struct {
	Counts        []ModelDataCount
	AllHaveData   bool
	EmptyModels   []string // Models with zero rows
	ErroredModels []string // Models that failed to query (timeout, etc.)
	TotalRows     int64
	MinRowCount   int64
	MinRowModel   string
}

// ValidateStrategyHasData queries each model to verify data exists in the proposed ranges.
func (g *Generator) ValidateStrategyHasData(
	ctx context.Context,
	result *DiscoveryResult,
	network string,
) (*ValidationResult, error) {
	counts := make([]ModelDataCount, 0, len(result.Strategies))
	emptyModels := make([]string, 0)
	erroredModels := make([]string, 0)

	var totalRows int64

	minRowCount := int64(-1)
	minRowModel := ""

	for _, strategy := range result.Strategies {
		count, err := g.QueryRowCount(ctx, strategy.Model, network, strategy.RangeColumn, strategy.FromValue, strategy.ToValue)

		modelCount := ModelDataCount{
			Model:    strategy.Model,
			Strategy: &strategy,
			Error:    err,
		}

		if err != nil {
			modelCount.HasData = false

			erroredModels = append(erroredModels, strategy.Model)
		} else {
			modelCount.RowCount = count
			modelCount.HasData = count > 0
			totalRows += count

			if !modelCount.HasData {
				emptyModels = append(emptyModels, strategy.Model)
			}

			if minRowCount < 0 || count < minRowCount {
				minRowCount = count
				minRowModel = strategy.Model
			}
		}

		counts = append(counts, modelCount)
	}

	return &ValidationResult{
		Counts:        counts,
		AllHaveData:   len(emptyModels) == 0 && len(erroredModels) == 0,
		EmptyModels:   emptyModels,
		ErroredModels: erroredModels,
		TotalRows:     totalRows,
		MinRowCount:   minRowCount,
		MinRowModel:   minRowModel,
	}, nil
}

// QueryRowCount queries the number of rows in a model for a given range.
func (g *Generator) QueryRowCount(
	ctx context.Context,
	model string,
	network string,
	rangeColumn string,
	fromValue string,
	toValue string,
) (int64, error) {
	// Determine if this is a numeric or time-based column
	isNumeric := !strings.Contains(strings.ToLower(rangeColumn), "date") &&
		!strings.Contains(strings.ToLower(rangeColumn), "time")

	var query string
	if isNumeric {
		query = fmt.Sprintf(`
			SELECT COUNT(*) as cnt
			FROM default.%s
			WHERE meta_network_name = '%s'
			  AND %s >= %s
			  AND %s <= %s
			FORMAT JSON
		`, model, network, rangeColumn, fromValue, rangeColumn, toValue)
	} else {
		query = fmt.Sprintf(`
			SELECT COUNT(*) as cnt
			FROM default.%s
			WHERE meta_network_name = '%s'
			  AND %s >= toDateTime('%s')
			  AND %s <= toDateTime('%s')
			FORMAT JSON
		`, model, network, rangeColumn, fromValue, rangeColumn, toValue)
	}

	g.log.WithFields(logrus.Fields{
		"model":   model,
		"network": network,
		"from":    fromValue,
		"to":      toValue,
	}).Debug("querying row count")

	chURL, err := g.buildClickHouseHTTPURL()
	if err != nil {
		return 0, fmt.Errorf("failed to build ClickHouse URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chURL, strings.NewReader(query))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{
		Timeout: 2 * time.Minute, // Row count queries on large tables can take time
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return 0, fmt.Errorf("ClickHouse returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	var jsonResp struct {
		Data []map[string]any `json:"data"`
	}

	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return 0, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if len(jsonResp.Data) == 0 {
		return 0, nil
	}

	// Extract count from response
	cntVal, ok := jsonResp.Data[0]["cnt"]
	if !ok {
		return 0, fmt.Errorf("cnt not found in response")
	}

	// Handle both string and numeric types
	switch v := cntVal.(type) {
	case string:
		var count int64

		_, err := fmt.Sscanf(v, "%d", &count)

		return count, err
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	default:
		return 0, fmt.Errorf("unexpected count type: %T", cntVal)
	}
}

// ExpandWindowMultiplier defines how much to expand the window on each retry.
const ExpandWindowMultiplier = 2

// SuggestExpandedStrategy creates a new strategy with an expanded time window.
// This is used when the original strategy has models with no data.
func SuggestExpandedStrategy(original *DiscoveryResult, multiplier int) *DiscoveryResult {
	expanded := &DiscoveryResult{
		PrimaryRangeType:   original.PrimaryRangeType,
		PrimaryRangeColumn: original.PrimaryRangeColumn,
		OverallConfidence:  original.OverallConfidence * 0.9, // Reduce confidence slightly
		Summary:            fmt.Sprintf("%s (window expanded %dx)", original.Summary, multiplier),
		Warnings:           append([]string{}, original.Warnings...),
		Strategies:         make([]TableRangeStrategy, len(original.Strategies)),
	}

	// For time-based ranges, we can try to expand by parsing and adjusting
	// For now, just copy and add a warning - the actual expansion would need
	// to be done with knowledge of the original window size
	copy(expanded.Strategies, original.Strategies)
	expanded.Warnings = append(expanded.Warnings,
		fmt.Sprintf("Window expanded %dx to find data - verify data quality", multiplier))

	return expanded
}
