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
func (g *Generator) QueryModelRange(ctx context.Context, model, network, rangeColumn string) (*ModelRange, error) {
	// Build range query
	query := fmt.Sprintf(`
		SELECT
			MIN(%s) as min_val,
			MAX(%s) as max_val
		FROM default.%s
		WHERE meta_network_name = '%s'
		FORMAT JSON
	`, rangeColumn, rangeColumn, model, network)

	g.log.WithField("query", query).Debug("querying model range")

	// Execute query
	result, err := g.executeRangeQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query range for %s: %w", model, err)
	}

	return &ModelRange{
		Model:       model,
		Network:     network,
		RangeColumn: rangeColumn,
		Min:         result.Min,
		Max:         result.Max,
		MinRaw:      result.MinRaw,
		MaxRaw:      result.MaxRaw,
	}, nil
}

// rangeQueryResult holds parsed range query results.
type rangeQueryResult struct {
	Min    time.Time
	Max    time.Time
	MinRaw string
	MaxRaw string
}

// clickHouseJSONResponse represents ClickHouse JSON format response.
type clickHouseJSONResponse struct {
	Data []map[string]any `json:"data"`
}

// executeRangeQuery executes a range query and parses the result.
func (g *Generator) executeRangeQuery(ctx context.Context, query string) (*rangeQueryResult, error) {
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
		Timeout: 2 * time.Minute,
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

	var jsonResp clickHouseJSONResponse

	if unmarshalErr := json.Unmarshal(body, &jsonResp); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", unmarshalErr)
	}

	if len(jsonResp.Data) == 0 {
		return nil, fmt.Errorf("no data returned from range query")
	}

	row := jsonResp.Data[0]

	minVal, ok := row["min_val"]
	if !ok {
		return nil, fmt.Errorf("min_val not found in response")
	}

	maxVal, ok := row["max_val"]
	if !ok {
		return nil, fmt.Errorf("max_val not found in response")
	}

	minTime, minRaw, err := parseTimeValue(minVal)
	if err != nil {
		return nil, fmt.Errorf("failed to parse min_val: %w", err)
	}

	maxTime, maxRaw, err := parseTimeValue(maxVal)
	if err != nil {
		return nil, fmt.Errorf("failed to parse max_val: %w", err)
	}

	return &rangeQueryResult{
		Min:    minTime,
		Max:    maxTime,
		MinRaw: minRaw,
		MaxRaw: maxRaw,
	}, nil
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
func (g *Generator) QueryModelRanges(ctx context.Context, models []string, network string, rangeInfos map[string]*RangeColumnInfo) ([]*ModelRange, error) {
	ranges := make([]*ModelRange, 0, len(models))

	for _, model := range models {
		rangeCol := DefaultRangeColumn
		if info, ok := rangeInfos[model]; ok {
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
