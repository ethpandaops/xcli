package seeddata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ModelRange represents the available data range for a model.
type ModelRange struct {
	Model       string
	Network     string
	RangeColumn string
	Min         time.Time
	Max         time.Time
	MinRaw      string // Original value from query (for display)
	MaxRaw      string
}

// QueryModelRange queries external ClickHouse for a model's available data range.
// Uses ORDER BY ... LIMIT 1 instead of MIN/MAX for better performance on large tables.
func (g *Generator) QueryModelRange(ctx context.Context, model, network, rangeColumn string) (*ModelRange, error) {
	tableRef := g.resolveTableRef(model)

	// Query for minimum value (oldest data)
	minQuery := fmt.Sprintf(`
		SELECT %s as val
		FROM %s
		WHERE meta_network_name = '%s'
		ORDER BY %s ASC
		LIMIT 1
		FORMAT JSON
	`, rangeColumn, tableRef, network, rangeColumn)

	g.log.WithField("query", minQuery).Debug("querying model min range")

	minResult, err := g.executeSingleValueQuery(ctx, minQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query min range for %s: %w", model, err)
	}

	// Query for maximum value (newest data)
	maxQuery := fmt.Sprintf(`
		SELECT %s as val
		FROM %s
		WHERE meta_network_name = '%s'
		ORDER BY %s DESC
		LIMIT 1
		FORMAT JSON
	`, rangeColumn, tableRef, network, rangeColumn)

	g.log.WithField("query", maxQuery).Debug("querying model max range")

	maxResult, err := g.executeSingleValueQuery(ctx, maxQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query max range for %s: %w", model, err)
	}

	return &ModelRange{
		Model:       model,
		Network:     network,
		RangeColumn: rangeColumn,
		Min:         minResult.Time,
		Max:         maxResult.Time,
		MinRaw:      minResult.Raw,
		MaxRaw:      maxResult.Raw,
	}, nil
}

// singleValueResult holds a single time value result.
type singleValueResult struct {
	Time time.Time
	Raw  string
}

// executeSingleValueQuery executes a query that returns a single time value.
func (g *Generator) executeSingleValueQuery(ctx context.Context, query string) (*singleValueResult, error) {
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
		Timeout: 30 * time.Second, // Shorter timeout for indexed queries
	}

	resp, err := client.Do(req) //nolint:gosec // G704 - URL is from trusted internal configuration
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

	var jsonResp clickHouseJSONResponse

	if unmarshalErr := json.Unmarshal(body, &jsonResp); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", unmarshalErr)
	}

	if len(jsonResp.Data) == 0 {
		return nil, fmt.Errorf("no data returned from query")
	}

	row := jsonResp.Data[0]

	val, ok := row["val"]
	if !ok {
		return nil, fmt.Errorf("val not found in response")
	}

	t, raw, parseErr := parseTimeValue(val)
	if parseErr != nil {
		return nil, fmt.Errorf("failed to parse time value: %w", parseErr)
	}

	return &singleValueResult{
		Time: t,
		Raw:  raw,
	}, nil
}

// clickHouseJSONResponse represents ClickHouse JSON format response.
type clickHouseJSONResponse struct {
	Data []map[string]any `json:"data"`
}

// parseTimeValue parses a time value from ClickHouse JSON response.
// It handles both DateTime strings and Unix timestamps.
func parseTimeValue(val any) (time.Time, string, error) {
	switch v := val.(type) {
	case string:
		// Try parsing as DateTime string
		t, err := time.Parse("2006-01-02 15:04:05", v)
		if err != nil {
			// Try with timezone
			t, err = time.Parse(time.RFC3339, v)
			if err != nil {
				return time.Time{}, v, fmt.Errorf("failed to parse time string '%s': %w", v, err)
			}
		}

		return t, v, nil

	case float64:
		// Unix timestamp (JSON numbers are float64)
		t := time.Unix(int64(v), 0).UTC()

		return t, fmt.Sprintf("%.0f", v), nil

	case int64:
		t := time.Unix(v, 0).UTC()

		return t, fmt.Sprintf("%d", v), nil

	default:
		return time.Time{}, fmt.Sprintf("%v", val), fmt.Errorf("unsupported time value type: %T", val)
	}
}

// QueryModelRanges queries ranges for multiple models.
// If overrideColumn is non-empty, it will be used for all models instead of detected columns.
func (g *Generator) QueryModelRanges(ctx context.Context, models []string, network string, rangeInfos map[string]*RangeColumnInfo, overrideColumn string) ([]*ModelRange, error) {
	ranges := make([]*ModelRange, 0, len(models))

	for _, model := range models {
		rangeCol := DefaultRangeColumn

		// Use override if provided, otherwise use detected column
		if overrideColumn != "" {
			rangeCol = overrideColumn
		} else if info, ok := rangeInfos[model]; ok {
			rangeCol = info.RangeColumn
		}

		modelRange, err := g.QueryModelRange(ctx, model, network, rangeCol)
		if err != nil {
			return nil, fmt.Errorf("failed to query range for %s: %w", model, err)
		}

		ranges = append(ranges, modelRange)
	}

	return ranges, nil
}

// FindIntersection finds the overlapping range across all model ranges.
// Returns nil if there is no intersection.
func FindIntersection(ranges []*ModelRange) (*ModelRange, error) {
	if len(ranges) == 0 {
		return nil, fmt.Errorf("no ranges provided")
	}

	if len(ranges) == 1 {
		return ranges[0], nil
	}

	// Find the maximum of all minimums and minimum of all maximums
	maxMin := ranges[0].Min
	minMax := ranges[0].Max

	for _, r := range ranges[1:] {
		if r.Min.After(maxMin) {
			maxMin = r.Min
		}

		if r.Max.Before(minMax) {
			minMax = r.Max
		}
	}

	// Check if there's an intersection
	if maxMin.After(minMax) {
		return nil, fmt.Errorf("no intersecting range found: ranges do not overlap")
	}

	return &ModelRange{
		Model:       "intersection",
		RangeColumn: ranges[0].RangeColumn,
		Min:         maxMin,
		Max:         minMax,
		MinRaw:      maxMin.Format("2006-01-02 15:04:05"),
		MaxRaw:      minMax.Format("2006-01-02 15:04:05"),
	}, nil
}

// FormatRange returns a human-readable string representation of the range.
func (r *ModelRange) FormatRange() string {
	return fmt.Sprintf("%s to %s",
		r.Min.Format("2006-01-02 15:04:05"),
		r.Max.Format("2006-01-02 15:04:05"))
}
