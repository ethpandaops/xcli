//nolint:staticcheck // QF1012: WriteString(Sprintf) pattern is used consistently for prompt building readability
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
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// ExpandWindowMultiplier defines how much to expand the window on each retry.
const ExpandWindowMultiplier = 2

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
	// RangeColumnTypeNone represents dimension/reference tables with no time-based range.
	RangeColumnTypeNone RangeColumnType = "none"
	// RangeColumnTypeUnknown represents an unclassified column type.
	RangeColumnTypeUnknown RangeColumnType = "unknown"

	// BlockNumberColumn is the standard column name for block-based tables.
	BlockNumberColumn = "block_number"
)

// TableRangeStrategy describes how to filter a single table for seed data extraction.
type TableRangeStrategy struct {
	Model             string          `yaml:"model"`
	RangeColumn       string          `yaml:"rangeColumn"`
	ColumnType        RangeColumnType `yaml:"columnType"`
	FromValue         string          `yaml:"fromValue"`
	ToValue           string          `yaml:"toValue"`
	FilterSQL         string          `yaml:"filterSql,omitempty"`
	CorrelationFilter string          `yaml:"correlationFilter,omitempty"` // Subquery filter for dimension tables
	Optional          bool            `yaml:"optional,omitempty"`          // True if table is optional (LEFT JOIN)
	RequiresBridge    bool            `yaml:"requiresBridge"`
	BridgeTable       string          `yaml:"bridgeTable,omitempty"`
	BridgeJoinSQL     string          `yaml:"bridgeJoinSql,omitempty"`
	Confidence        float64         `yaml:"confidence"`
	Reasoning         string          `yaml:"reasoning"`
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

// TableSchemaInfo contains schema information for a table.
type TableSchemaInfo struct {
	Model        string           `yaml:"model"`
	IntervalType IntervalType     `yaml:"intervalType,omitempty"` // From model frontmatter (slot, block, entity)
	Columns      []ColumnInfo     `yaml:"columns"`
	SampleData   []map[string]any `yaml:"sampleData,omitempty"`
	RangeInfo    *DetectedRange   `yaml:"rangeInfo,omitempty"`
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
	IntermediateModels  []IntermediateSQL `yaml:"intermediateModels,omitempty"` // SQL for intermediate deps
	Network             string            `yaml:"network"`
	Duration            string            `yaml:"duration"` // e.g., "5m", "10m", "1h"
	ExternalModels      []TableSchemaInfo `yaml:"externalModels"`
}

// IntermediateSQL contains SQL for an intermediate transformation model.
type IntermediateSQL struct {
	Model string `yaml:"model"`
	SQL   string `yaml:"sql"`
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
// All tables are treated equally - Claude will analyze the schema to determine
// the best filtering strategy for each table.
func (c *ClaudeDiscoveryClient) GatherSchemaInfo(
	ctx context.Context,
	models []string,
	network string,
	xatuCBTPath string,
) ([]TableSchemaInfo, error) {
	schemas := make([]TableSchemaInfo, 0, len(models))

	for _, model := range models {
		c.log.WithField("model", model).Debug("gathering schema info")

		// Get interval type from model frontmatter (informational context for Claude)
		intervalType, err := GetExternalModelIntervalType(model, xatuCBTPath)
		if err != nil {
			c.log.WithError(err).WithField("model", model).Warn("failed to get interval type")

			intervalType = IntervalTypeSlot // Default to slot
		}

		// Get column schema from ClickHouse
		columns, err := c.gen.DescribeTable(ctx, model)
		if err != nil {
			return nil, fmt.Errorf("failed to describe table %s: %w", model, err)
		}

		schemaInfo := TableSchemaInfo{
			Model:        model,
			IntervalType: intervalType,
			Columns:      columns,
		}

		// Try to detect range column from SQL file
		rangeCol, detectErr := DetectRangeColumnForModel(model, xatuCBTPath)
		if detectErr != nil {
			c.log.WithError(detectErr).WithField("model", model).Debug("failed to detect range column from SQL")

			// For tables without a detected range column, find any time column in schema
			// This handles entity tables and other tables without explicit range definitions
			rangeCol = findTimeColumnInSchema(columns)
			if rangeCol == "" {
				rangeCol = DefaultRangeColumn // Last resort fallback
			}
		}

		// Classify the range column type
		colType := ClassifyRangeColumn(rangeCol, columns)

		// Query the range for this model
		var minVal, maxVal string

		modelRange, rangeErr := c.gen.QueryModelRange(ctx, model, network, rangeCol)
		if rangeErr != nil {
			c.log.WithError(rangeErr).WithField("model", model).Warn("failed to query model range")
		} else {
			minVal = modelRange.MinRaw
			maxVal = modelRange.MaxRaw
		}

		// Get sample data (limited to 3 rows for prompt size)
		sampleData, sampleErr := c.gen.QueryTableSample(ctx, model, network, 3)
		if sampleErr != nil {
			c.log.WithError(sampleErr).WithField("model", model).Warn("failed to query sample data")
			// Continue without sample data - not critical
		}

		schemaInfo.SampleData = sampleData
		schemaInfo.RangeInfo = &DetectedRange{
			Column:     rangeCol,
			ColumnType: colType,
			Detected:   detectErr == nil, // True if detected from SQL, false if fallback
			MinValue:   minVal,
			MaxValue:   maxVal,
		}

		schemas = append(schemas, schemaInfo)
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
		count, err := g.QueryRowCount(ctx, strategy.Model, network, strategy.RangeColumn, strategy.FromValue, strategy.ToValue, strategy.FilterSQL, strategy.CorrelationFilter)

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
// For dimension tables (empty rangeColumn), it counts all rows for the network.
func (g *Generator) QueryRowCount(
	ctx context.Context,
	model string,
	network string,
	rangeColumn string,
	fromValue string,
	toValue string,
	filterSQL string,
	correlationFilter string,
) (int64, error) {
	// Build additional filter clause
	filterClause := ""
	if filterSQL != "" {
		filterClause = fmt.Sprintf("\n			  AND %s", filterSQL)
	}

	if correlationFilter != "" {
		filterClause += fmt.Sprintf("\n			  AND %s", correlationFilter)
	}

	tableRef := g.resolveTableRef(model)

	var query string

	// Handle dimension tables (no range column)
	if rangeColumn == "" {
		query = fmt.Sprintf(`
			SELECT COUNT(*) as cnt
			FROM %s
			WHERE meta_network_name = '%s'%s
			FORMAT JSON
		`, tableRef, network, filterClause)
	} else {
		// Determine if this is a numeric or time-based column
		isNumeric := !strings.Contains(strings.ToLower(rangeColumn), "date") &&
			!strings.Contains(strings.ToLower(rangeColumn), "time")

		if isNumeric {
			query = fmt.Sprintf(`
			SELECT COUNT(*) as cnt
			FROM %s
			WHERE meta_network_name = '%s'
			  AND %s >= %s
			  AND %s <= %s%s
			FORMAT JSON
		`, tableRef, network, rangeColumn, fromValue, rangeColumn, toValue, filterClause)
		} else {
			query = fmt.Sprintf(`
			SELECT COUNT(*) as cnt
			FROM %s
			WHERE meta_network_name = '%s'
			  AND %s >= toDateTime('%s')
			  AND %s <= toDateTime('%s')%s
			FORMAT JSON
		`, tableRef, network, rangeColumn, fromValue, rangeColumn, toValue, filterClause)
		}
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

	resp, err := client.Do(req) //nolint:gosec // URL is from trusted config
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

// QueryTableSample retrieves sample rows from a table for analysis.
func (g *Generator) QueryTableSample(
	ctx context.Context,
	model string,
	network string,
	limit int,
) ([]map[string]any, error) {
	tableRef := g.resolveTableRef(model)
	query := fmt.Sprintf(`
		SELECT *
		FROM %s
		WHERE meta_network_name = '%s'
		ORDER BY rand()
		LIMIT %d
		FORMAT JSON
	`, tableRef, network, limit)

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

	var jsonResp struct {
		Data []map[string]any `json:"data"`
	}

	if err := json.Unmarshal(body, &jsonResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return jsonResp.Data, nil
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
	case colLower == BlockNumberColumn || strings.HasSuffix(colLower, "_block_number"):
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

// ReadTransformationSQL reads the SQL file for a transformation model.
func ReadTransformationSQL(model, xatuCBTPath string) (string, error) {
	sqlPath := filepath.Join(xatuCBTPath, "models", "transformations", model+".sql")

	content, err := os.ReadFile(sqlPath)
	if err != nil {
		return "", fmt.Errorf("failed to read transformation SQL: %w", err)
	}

	return string(content), nil
}

// FallbackRangeDiscovery provides heuristic-based range discovery when Claude is unavailable.
//
//nolint:funlen,gocognit,cyclop,gocyclo // Complex heuristic logic requires length
func FallbackRangeDiscovery(
	ctx context.Context,
	gen *Generator,
	models []string,
	network string,
	rangeInfos map[string]*RangeColumnInfo,
	duration string,
	xatuCBTPath string,
) (*DiscoveryResult, error) {
	// Get interval types from model frontmatter for accurate categorization
	intervalTypes, err := GetExternalModelIntervalTypes(models, xatuCBTPath)
	if err != nil {
		// Log warning but continue with column-based detection as fallback
		gen.log.WithError(err).Warn("failed to get interval types from frontmatter, using column-based detection")

		intervalTypes = nil
	}

	// Group models by interval type
	_, blockModels, entityModels, unknownModels := categorizeModelsByType(models, intervalTypes, rangeInfos)

	// Track time ranges and block ranges separately
	var latestTimeMin, earliestTimeMax time.Time

	var latestBlockMin, earliestBlockMax int64

	hasTimeRanges := false
	hasBlockRanges := false

	// Store per-model range info for later assignment
	type modelRangeInfo struct {
		rangeCol   string
		colType    RangeColumnType
		reasoning  string
		confidence float64
	}

	modelRanges := make(map[string]*modelRangeInfo, len(models))
	strategies := make([]TableRangeStrategy, 0, len(models))

	for _, model := range models {
		info := rangeInfos[model]

		// Check model category
		isEntity := contains(entityModels, model)
		isBlock := contains(blockModels, model)
		isUnknown := contains(unknownModels, model)

		var rangeCol string

		var colType RangeColumnType

		// For entity models, they typically don't have time-based ranges
		// Mark them as "none" type - they'll get all data or use correlation
		if isEntity {
			gen.log.WithField("model", model).Info("entity/dimension table - will query without range filter")

			strategies = append(strategies, TableRangeStrategy{
				Model:      model,
				ColumnType: RangeColumnTypeNone,
				Confidence: 0.7,
				Reasoning:  "Entity/dimension table - no range filtering (all data)",
			})

			continue
		} else if isBlock {
			// Block-based models use block_number
			rangeCol = BlockNumberColumn
			if info != nil && info.RangeColumn != "" {
				rangeCol = info.RangeColumn
			}

			colType = RangeColumnTypeBlock
		} else if isUnknown {
			// Unknown models - try default range column
			rangeCol = DefaultRangeColumn
			colType = RangeColumnTypeTime
		} else {
			// Time models - use detected range column
			rangeCol = DefaultRangeColumn
			if info != nil {
				rangeCol = info.RangeColumn
			}

			colType = ClassifyRangeColumn(rangeCol, nil)
		}

		modelRange, queryErr := gen.QueryModelRange(ctx, model, network, rangeCol)
		if queryErr != nil {
			gen.log.WithError(queryErr).WithField("model", model).Warn("range query failed")

			strategies = append(strategies, TableRangeStrategy{
				Model:       model,
				RangeColumn: rangeCol,
				ColumnType:  colType,
				Confidence:  0.3,
				Reasoning:   fmt.Sprintf("Range query failed: %v", queryErr),
			})

			continue
		}

		// Track ranges based on column type
		if colType == RangeColumnTypeBlock {
			// Parse raw values as block numbers
			var minBlock, maxBlock int64

			if _, scanErr := fmt.Sscanf(modelRange.MinRaw, "%d", &minBlock); scanErr != nil {
				gen.log.WithError(scanErr).WithField("model", model).Warn("failed to parse min block number")

				continue
			}

			if _, scanErr := fmt.Sscanf(modelRange.MaxRaw, "%d", &maxBlock); scanErr != nil {
				gen.log.WithError(scanErr).WithField("model", model).Warn("failed to parse max block number")

				continue
			}

			if !hasBlockRanges || minBlock > latestBlockMin {
				latestBlockMin = minBlock
			}

			if !hasBlockRanges || maxBlock < earliestBlockMax {
				earliestBlockMax = maxBlock
			}

			hasBlockRanges = true

			modelRanges[model] = &modelRangeInfo{
				rangeCol:   rangeCol,
				colType:    colType,
				reasoning:  "Block-based model",
				confidence: 0.7,
			}
		} else {
			// Time-based range
			if latestTimeMin.IsZero() || modelRange.Min.After(latestTimeMin) {
				latestTimeMin = modelRange.Min
			}

			if earliestTimeMax.IsZero() || modelRange.Max.Before(earliestTimeMax) {
				earliestTimeMax = modelRange.Max
			}

			hasTimeRanges = true

			modelRanges[model] = &modelRangeInfo{
				rangeCol:   rangeCol,
				colType:    colType,
				reasoning:  "Time-based model",
				confidence: 0.7,
			}
		}
	}

	// Determine primary type and compute range values
	var fromValue, toValue string

	var primaryType RangeColumnType

	var primaryColumn string

	// Calculate time-based range values if we have time models
	var timeFromValue, timeToValue string

	if hasTimeRanges {
		if latestTimeMin.After(earliestTimeMax) {
			gen.log.Warn("no intersecting time range found, using latest available data")

			earliestTimeMax = latestTimeMin.Add(5 * time.Minute)
		}

		rangeDuration, parseErr := time.ParseDuration(duration)
		if parseErr != nil {
			rangeDuration = 5 * time.Minute
		}

		effectiveMax := earliestTimeMax.Add(-1 * time.Minute)
		effectiveMin := effectiveMax.Add(-rangeDuration)

		if effectiveMin.Before(latestTimeMin) {
			effectiveMin = latestTimeMin
		}

		timeFromValue = effectiveMin.Format("2006-01-02 15:04:05")
		timeToValue = effectiveMax.Format("2006-01-02 15:04:05")
	}

	// Calculate block-based range values if we have block models
	var blockFromValue, blockToValue string

	if hasBlockRanges {
		if latestBlockMin > earliestBlockMax {
			gen.log.Warn("no intersecting block range found, using latest available data")

			earliestBlockMax = latestBlockMin + 1000
		}

		// For blocks, use a reasonable range (e.g., last 1000 blocks or based on duration)
		// Approximate: 1 block every 12 seconds, so 5 minutes = ~25 blocks
		rangeDuration, parseErr := time.ParseDuration(duration)
		if parseErr != nil {
			rangeDuration = 5 * time.Minute
		}

		blocksPerDuration := int64(rangeDuration.Seconds() / 12) // ~12 second block time
		if blocksPerDuration < 100 {
			blocksPerDuration = 100 // Minimum 100 blocks
		}

		effectiveMax := earliestBlockMax - 10 // Account for reorgs
		effectiveMin := effectiveMax - blocksPerDuration

		if effectiveMin < latestBlockMin {
			effectiveMin = latestBlockMin
		}

		blockFromValue = fmt.Sprintf("%d", effectiveMin)
		blockToValue = fmt.Sprintf("%d", effectiveMax)
	}

	// Set primary type based on what we have
	if hasTimeRanges && !hasBlockRanges {
		primaryType = RangeColumnTypeTime
		primaryColumn = DefaultRangeColumn
		fromValue = timeFromValue
		toValue = timeToValue
	} else if hasBlockRanges && !hasTimeRanges {
		primaryType = RangeColumnTypeBlock
		primaryColumn = BlockNumberColumn
		fromValue = blockFromValue
		toValue = blockToValue
	} else if hasTimeRanges && hasBlockRanges {
		// Mixed - prefer time as primary
		primaryType = RangeColumnTypeTime
		primaryColumn = DefaultRangeColumn
		fromValue = timeFromValue
		toValue = timeToValue
	} else {
		// No valid ranges found - check if we have entity-only models
		if len(strategies) > 0 {
			// All models are entity tables - proceed without primary range
			primaryType = RangeColumnTypeNone
			primaryColumn = ""
			fromValue = ""
			toValue = ""
		} else {
			return nil, fmt.Errorf("no valid range columns found for any model")
		}
	}

	// Create strategies for models with ranges
	for model, rangeInfo := range modelRanges {
		strategy := TableRangeStrategy{
			Model:       model,
			RangeColumn: rangeInfo.rangeCol,
			ColumnType:  rangeInfo.colType,
			Confidence:  rangeInfo.confidence,
			Reasoning:   rangeInfo.reasoning,
		}

		// Assign appropriate range values based on column type
		if rangeInfo.colType == RangeColumnTypeBlock {
			strategy.FromValue = blockFromValue
			strategy.ToValue = blockToValue
		} else {
			strategy.FromValue = timeFromValue
			strategy.ToValue = timeToValue
		}

		strategies = append(strategies, strategy)
	}

	warnings := make([]string, 0)

	if hasBlockRanges && hasTimeRanges {
		warnings = append(warnings,
			"Mixed range column types detected (time and block). "+
				"Each table type will use its appropriate range values.")
	}

	if len(entityModels) > 0 {
		warnings = append(warnings,
			fmt.Sprintf("Entity/dimension tables detected (%v). These will query all data without range filtering.",
				entityModels))
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
	sb.WriteString("- Numeric columns (block_number - UInt64, slot - UInt64)\n")
	sb.WriteString("- **Dimension/reference tables** (no time range - static lookup data like validator entities)\n\n")
	sb.WriteString("You need to determine how to correlate these ranges so we get matching data across all tables.\n\n")

	sb.WriteString("**CRITICAL**: The transformation and its intermediate dependencies may have WHERE clauses that filter data.\n")
	sb.WriteString("If you extract seed data that doesn't match these filters, the transformation will produce ZERO output rows!\n")
	sb.WriteString("You MUST analyze the SQL and include any necessary filters in `filterSql` for each external model.\n\n")

	sb.WriteString("**IMPORTANT**: ALL tables must be filtered to limit data volume. Look at each table's schema to find appropriate filter columns.\n\n")

	sb.WriteString("**DIMENSION/ENTITY TABLES**: For tables that are JOINed as lookups (like validator entities):\n")
	sb.WriteString("1. Analyze the JOIN condition in the transformation SQL to find the join key (e.g., validator_index)\n")
	sb.WriteString("2. Use `correlationFilter` to filter by values that exist in the primary data tables\n")
	sb.WriteString("3. **IMPORTANT**: Use `GLOBAL IN` (not just `IN`) for subqueries - ClickHouse requires this for distributed tables\n")
	sb.WriteString("4. Example: If attestations JOIN on validator_index, filter entities to only those validators\n")
	sb.WriteString("5. Mark tables as `optional: true` if the transformation can produce output without them (LEFT JOINs)\n")
	sb.WriteString("6. If correlation isn't possible, use a reasonable time-based filter on any available DateTime column\n\n")

	sb.WriteString("## External Models and Their Schemas\n\n")

	for _, schema := range input.ExternalModels {
		sb.WriteString(fmt.Sprintf("### %s\n", schema.Model))

		// Show interval type from frontmatter (informational context for Claude)
		if schema.IntervalType != "" {
			sb.WriteString(fmt.Sprintf("Interval Type: %s\n", schema.IntervalType))
		}

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

	// Include intermediate dependency SQL if available
	if len(input.IntermediateModels) > 0 {
		sb.WriteString("## Intermediate Dependency SQL\n")
		sb.WriteString("The transformation depends on these intermediate models. Their WHERE clauses affect which seed data is usable:\n\n")

		for _, intermediate := range input.IntermediateModels {
			sb.WriteString(fmt.Sprintf("### %s\n```sql\n%s\n```\n\n", intermediate.Model, intermediate.SQL))
		}
	}

	sb.WriteString("## Instructions\n")
	sb.WriteString("1. Analyze which tables can share a common range column directly\n")
	sb.WriteString("2. For tables with different range types, determine if correlation is possible via:\n")
	sb.WriteString("   - Direct conversion (e.g., slot to slot_start_date_time via calculation)\n")
	sb.WriteString("   - Bridge table (e.g., canonical_beacon_block has both slot and execution block info)\n")
	sb.WriteString("   - Shared columns in the data itself\n")
	sb.WriteString("3. Recommend a primary range specification (type + column + from/to values)\n")
	sb.WriteString(fmt.Sprintf("4. Use a %s time range (as requested by the user)\n", input.Duration))
	sb.WriteString("5. For each table, specify exactly how to filter it\n")
	sb.WriteString("6. **CRITICAL**: Analyze ALL WHERE clauses in the transformation and intermediate SQL.\n")
	sb.WriteString("   For each external model, identify any column filters that must be applied to get usable data.\n")
	sb.WriteString("   Include these as `filterSql` - a SQL fragment like \"aggregation_bits = ''\" or \"attesting_validator_index IS NOT NULL\"\n\n")

	sb.WriteString("## Output Format\n")
	sb.WriteString("Output ONLY valid YAML matching this EXACT structure.\n\n")
	sb.WriteString("**CRITICAL FORMATTING RULES**:\n")
	sb.WriteString("1. ALL field names MUST use proper camelCase (e.g., `primaryRangeColumn`, `fromValue`, `filterSql`)\n")
	sb.WriteString("2. All datetime values MUST be quoted: `fromValue: \"2025-01-01 00:00:00\"`\n")
	sb.WriteString("3. Output ONLY the YAML - no markdown code blocks, no explanations\n\n")
	sb.WriteString("```yaml\n")
	sb.WriteString("primaryRangeType: time\n")
	sb.WriteString("primaryRangeColumn: slot_start_date_time\n")
	sb.WriteString("fromValue: \"2025-01-01 00:00:00\"\n")
	sb.WriteString("toValue: \"2025-01-01 00:05:00\"\n")
	sb.WriteString("strategies:\n")
	sb.WriteString("  - model: beacon_api_eth_v1_events_attestation\n")
	sb.WriteString("    rangeColumn: slot_start_date_time\n")
	sb.WriteString("    columnType: time\n")
	sb.WriteString("    fromValue: \"2025-01-01 00:00:00\"\n")
	sb.WriteString("    toValue: \"2025-01-01 00:05:00\"\n")
	sb.WriteString("    filterSql: \"aggregation_bits = '' AND attesting_validator_index IS NOT NULL\"\n")
	sb.WriteString("    requiresBridge: false\n")
	sb.WriteString("    confidence: 0.9\n")
	sb.WriteString("    reasoning: \"Filters from intermediate SQL\"\n")
	sb.WriteString("  - model: ethseer_validator_entity\n")
	sb.WriteString("    rangeColumn: \"\"\n")
	sb.WriteString("    columnType: none\n")
	sb.WriteString("    fromValue: \"\"\n")
	sb.WriteString("    toValue: \"\"\n")
	sb.WriteString("    filterSql: \"\"\n")
	sb.WriteString("    correlationFilter: \"validator_index GLOBAL IN (SELECT DISTINCT attesting_validator_index FROM default.beacon_api_eth_v1_events_attestation WHERE slot_start_date_time >= toDateTime('2025-01-01 00:00:00') AND slot_start_date_time <= toDateTime('2025-01-01 00:05:00') AND meta_network_name = 'mainnet')\"\n")
	sb.WriteString("    optional: true\n")
	sb.WriteString("    requiresBridge: false\n")
	sb.WriteString("    confidence: 0.9\n")
	sb.WriteString("    reasoning: \"Entity table filtered by correlation - only validators appearing in attestation data\"\n")
	sb.WriteString("overallConfidence: 0.85\n")
	sb.WriteString("summary: \"Time-based primary range with filters from dependencies\"\n")
	sb.WriteString("warnings:\n")
	sb.WriteString("  - \"Filters applied to ensure usable seed data\"\n")
	sb.WriteString("```\n\n")

	sb.WriteString("IMPORTANT:\n")
	sb.WriteString("- Use actual values from the available ranges shown above\n")
	sb.WriteString("- Pick a recent time window (last hour or so) within the intersection of all available ranges\n")
	sb.WriteString("- For block_number tables, estimate block numbers that correspond to the chosen time window\n")
	sb.WriteString("- Include ALL external models in the strategies list\n")
	sb.WriteString("- **ANALYZE ALL WHERE CLAUSES** in transformation and intermediate SQL - missing filters will cause empty test output!\n")
	sb.WriteString("- Include `filterSql` for each model (empty string if no additional filters needed)\n\n")

	sb.WriteString("**YOUR RESPONSE MUST START WITH `primaryRangeType:` - no preamble, no explanations, no markdown, just the raw YAML.**\n")

	return sb.String()
}

// parseDiscoveryResponse parses Claude's YAML response.
func (c *ClaudeDiscoveryClient) parseDiscoveryResponse(response string) (*DiscoveryResult, error) {
	// Extract YAML from response
	yamlContent := extractYAMLFromResponse(response)
	if yamlContent == "" {
		// Log the raw response for debugging
		c.log.WithField("response_preview", truncateString(response, 500)).Error("no valid YAML found in Claude response")

		return nil, fmt.Errorf("no valid YAML found in Claude response")
	}

	// Normalize field names (Claude may output snake_case instead of camelCase)
	beforeNorm := yamlContent
	yamlContent = normalizeDiscoveryYAMLFields(yamlContent)

	if beforeNorm != yamlContent {
		c.log.Debug("YAML normalization applied field name corrections")
	}

	c.log.WithField("yaml_preview", truncateString(yamlContent, 300)).Debug("extracted YAML content")

	var result DiscoveryResult

	if err := yaml.Unmarshal([]byte(yamlContent), &result); err != nil {
		c.log.WithFields(logrus.Fields{
			"error":        err,
			"yaml_content": yamlContent,
		}).Error("failed to parse discovery YAML")

		// Include YAML preview in error for UI visibility
		yamlPreview := yamlContent
		if len(yamlPreview) > 800 {
			yamlPreview = yamlPreview[:800] + "..."
		}

		return nil, fmt.Errorf("failed to parse discovery YAML: %w\n\nClaude's output:\n%s", err, yamlPreview)
	}

	// Validate result
	if err := c.validateDiscoveryResult(&result); err != nil {
		c.log.WithFields(logrus.Fields{
			"error":        err,
			"yaml_content": yamlContent, // Full content for debugging
			"parsed":       result,
		}).Warn("invalid discovery result - showing YAML for debugging")

		// Include YAML preview in error for UI visibility
		yamlPreview := yamlContent
		if len(yamlPreview) > 500 {
			yamlPreview = yamlPreview[:500] + "..."
		}

		return nil, fmt.Errorf("invalid discovery result: %w\n\nClaude's YAML output:\n%s", err, yamlPreview)
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

		// Tables can be filtered by:
		// 1. Range column (rangeColumn + fromValue/toValue)
		// 2. Correlation filter (subquery)
		// 3. None type (dimension table - accepts all or filtered by filterSQL)
		hasRangeFilter := s.RangeColumn != "" && s.FromValue != "" && s.ToValue != ""
		hasCorrelationFilter := s.CorrelationFilter != ""
		isNoneType := s.ColumnType == RangeColumnTypeNone

		// Must have at least one filtering mechanism
		if !hasRangeFilter && !hasCorrelationFilter && !isNoneType {
			return fmt.Errorf("strategy %d (%s): requires range filter (rangeColumn+from/to), correlationFilter, or columnType: none", i, s.Model)
		}

		// If range column is specified, from/to are required
		if s.RangeColumn != "" && !hasCorrelationFilter && (s.FromValue == "" || s.ToValue == "") {
			return fmt.Errorf("strategy %d (%s): from_value and to_value are required when range_column is set without correlationFilter", i, s.Model)
		}
	}

	return nil
}

// normalizeDiscoveryYAMLFields converts common field name variations to expected camelCase
// and fixes common YAML formatting issues in Claude's output.
func normalizeDiscoveryYAMLFields(yamlContent string) string {
	// Map of various field name formats to expected camelCase
	// Includes snake_case, PascalCase, typos, and other variations Claude might output
	replacements := map[string]string{
		// snake_case variations
		"primary_range_type:":   "primaryRangeType:",
		"primary_range_column:": "primaryRangeColumn:",
		// Common typos (missing capital letters)
		"primaryrangeType:":   "primaryRangeType:",
		"primaryrangeColumn:": "primaryRangeColumn:",
		"primaryRangetype:":   "primaryRangeType:",
		"primaryRangecolumn:": "primaryRangeColumn:",
		"from_value:":         "fromValue:",
		"to_value:":           "toValue:",
		"range_column:":       "rangeColumn:",
		"column_type:":        "columnType:",
		"filter_sql:":         "filterSql:",
		"correlation_filter:": "correlationFilter:",
		"requires_bridge:":    "requiresBridge:",
		"bridge_table:":       "bridgeTable:",
		"bridge_join_sql:":    "bridgeJoinSql:",
		"overall_confidence:": "overallConfidence:",
		// PascalCase variations
		"PrimaryRangeType:":   "primaryRangeType:",
		"PrimaryRangeColumn:": "primaryRangeColumn:",
		"FromValue:":          "fromValue:",
		"ToValue:":            "toValue:",
		"RangeColumn:":        "rangeColumn:",
		"ColumnType:":         "columnType:",
		"FilterSql:":          "filterSql:",
		"FilterSQL:":          "filterSql:",
		"CorrelationFilter:":  "correlationFilter:",
		"RequiresBridge:":     "requiresBridge:",
		"BridgeTable:":        "bridgeTable:",
		"BridgeJoinSql:":      "bridgeJoinSql:",
		"BridgeJoinSQL:":      "bridgeJoinSql:",
		"OverallConfidence:":  "overallConfidence:",
		// Common typos/variations
		"filterSql:": "filterSql:",
		"filter:":    "filterSql:", // Claude might shorten this
	}

	result := yamlContent
	for variant, camel := range replacements {
		result = strings.ReplaceAll(result, variant, camel)
	}

	// Fix unquoted datetime values (e.g., "fromValue: 2025-01-01 00:00:00" -> "fromValue: \"2025-01-01 00:00:00\"")
	result = fixUnquotedDatetimes(result)

	return result
}

// fixUnquotedDatetimes finds unquoted datetime values and adds quotes.
// Matches patterns like "fromValue: 2025-01-01 00:00:00" where the datetime is not quoted.
func fixUnquotedDatetimes(yamlContent string) string {
	// Pattern: key followed by colon, space, then YYYY-MM-DD HH:MM:SS (not already quoted)
	// We need to be careful not to double-quote already quoted values
	lines := strings.Split(yamlContent, "\n")
	result := make([]string, 0, len(lines))

	datetimePattern := regexp.MustCompile(`^(\s*\w+:\s*)(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})(\s*#.*)?$`)

	for _, line := range lines {
		matches := datetimePattern.FindStringSubmatch(line)
		if matches != nil {
			// Found unquoted datetime - add quotes
			prefix := matches[1]
			datetime := matches[2]

			suffix := ""
			if len(matches) > 3 {
				suffix = matches[3]
			}

			line = prefix + "\"" + datetime + "\"" + suffix
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen] + "..."
}

// findTimeColumnInSchema looks for a time-based column in the schema.
// Prefers columns with "date_time" in the name, falls back to any DateTime column.
func findTimeColumnInSchema(columns []ColumnInfo) string {
	// First, look for columns with "date_time" in the name (most common pattern)
	for _, col := range columns {
		colLower := strings.ToLower(col.Name)
		if strings.Contains(colLower, "date_time") {
			return col.Name
		}
	}

	// Fall back to any DateTime column
	for _, col := range columns {
		typeLower := strings.ToLower(col.Type)
		if strings.Contains(typeLower, "datetime") {
			return col.Name
		}
	}

	return ""
}

// contains checks if a string slice contains a value.
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}

	return false
}

// categorizeModelsByType groups models into time, block, entity, and unknown categories.
// Uses interval types from frontmatter if available, falls back to column-based detection.
func categorizeModelsByType(
	models []string,
	intervalTypes map[string]IntervalType,
	rangeInfos map[string]*RangeColumnInfo,
) (timeModels, blockModels, entityModels, unknownModels []string) {
	timeModels = make([]string, 0, len(models))
	blockModels = make([]string, 0, len(models))
	entityModels = make([]string, 0, len(models))
	unknownModels = make([]string, 0, len(models))

	for _, model := range models {
		// First check frontmatter interval type (most accurate)
		if intervalTypes != nil {
			if intervalType, ok := intervalTypes[model]; ok {
				switch intervalType {
				case IntervalTypeEntity:
					// Entity models need special handling - they have time columns but indexed by entity
					entityModels = append(entityModels, model)

					continue
				case IntervalTypeBlock:
					blockModels = append(blockModels, model)

					continue
				case IntervalTypeSlot:
					timeModels = append(timeModels, model)

					continue
				}
			}
		}

		// Fallback: use range column detection
		info, ok := rangeInfos[model]
		if !ok {
			unknownModels = append(unknownModels, model)

			continue
		}

		colLower := strings.ToLower(info.RangeColumn)

		switch {
		case strings.Contains(colLower, "date_time") || strings.Contains(colLower, "timestamp"):
			timeModels = append(timeModels, model)
		case strings.Contains(colLower, "block"):
			blockModels = append(blockModels, model)
		default:
			unknownModels = append(unknownModels, model)
		}
	}

	return timeModels, blockModels, entityModels, unknownModels
}
