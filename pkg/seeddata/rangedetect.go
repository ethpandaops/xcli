package seeddata

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// DefaultRangeColumn is the fallback range column when detection fails.
	DefaultRangeColumn = "slot_start_date_time"
)

// rangeColumnPatterns are regex patterns to detect range columns from external model SQL.
// Order matters - more specific patterns should come first.
var rangeColumnPatterns = []*regexp.Regexp{
	// Pattern: toUnixTimestamp(min(column_name)) as min
	regexp.MustCompile(`toUnixTimestamp\s*\(\s*min\s*\(\s*(\w+)\s*\)\s*\)`),
	// Pattern: toUnixTimestamp(max(column_name)) as max
	regexp.MustCompile(`toUnixTimestamp\s*\(\s*max\s*\(\s*(\w+)\s*\)\s*\)`),
	// Pattern: min(column_name) as min
	regexp.MustCompile(`(?:^|[^(])\bmin\s*\(\s*(\w+)\s*\)\s+as\s+min`),
	// Pattern: max(column_name) as max
	regexp.MustCompile(`(?:^|[^(])\bmax\s*\(\s*(\w+)\s*\)\s+as\s+max`),
}

// DetectRangeColumn parses an external model SQL file to detect the range column
// used in bounds queries. It looks for patterns like toUnixTimestamp(min(column_name)).
// Falls back to DefaultRangeColumn if detection fails.
func DetectRangeColumn(externalModelPath string) (string, error) {
	file, err := os.Open(externalModelPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	var content strings.Builder

	for scanner.Scan() {
		content.WriteString(scanner.Text())
		content.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading file: %w", err)
	}

	sqlContent := strings.ToLower(content.String())

	// Try each pattern to find the range column
	for _, pattern := range rangeColumnPatterns {
		matches := pattern.FindStringSubmatch(sqlContent)
		if len(matches) > 1 {
			return matches[1], nil
		}
	}

	// No pattern matched, return default
	return DefaultRangeColumn, nil
}

// DetectRangeColumnForModel detects the range column for an external model by name.
func DetectRangeColumnForModel(model, xatuCBTPath string) (string, error) {
	modelPath := filepath.Join(xatuCBTPath, "models", "external", model+".sql")

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return "", fmt.Errorf("external model '%s' not found", model)
	}

	return DetectRangeColumn(modelPath)
}

// RangeColumnInfo contains information about a model's range column.
type RangeColumnInfo struct {
	Model       string
	RangeColumn string
	Detected    bool // true if detected from SQL, false if using default
}

// DetectRangeColumnsForModels detects range columns for multiple external models.
// Returns a map of model name to range column info.
func DetectRangeColumnsForModels(models []string, xatuCBTPath string) (map[string]*RangeColumnInfo, error) {
	result := make(map[string]*RangeColumnInfo, len(models))

	for _, model := range models {
		modelPath := filepath.Join(xatuCBTPath, "models", "external", model+".sql")

		if _, err := os.Stat(modelPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("external model '%s' not found", model)
		}

		rangeCol, err := DetectRangeColumn(modelPath)
		if err != nil {
			return nil, fmt.Errorf("failed to detect range column for %s: %w", model, err)
		}

		result[model] = &RangeColumnInfo{
			Model:       model,
			RangeColumn: rangeCol,
			Detected:    rangeCol != DefaultRangeColumn,
		}
	}

	return result, nil
}

// FindCommonRangeColumn finds a common range column across all models.
// Returns the common column if all models share the same one, or DefaultRangeColumn otherwise.
func FindCommonRangeColumn(rangeInfos map[string]*RangeColumnInfo) string {
	if len(rangeInfos) == 0 {
		return DefaultRangeColumn
	}

	var commonColumn string

	for _, info := range rangeInfos {
		if commonColumn == "" {
			commonColumn = info.RangeColumn

			continue
		}

		if info.RangeColumn != commonColumn {
			// Different range columns, fall back to default
			return DefaultRangeColumn
		}
	}

	return commonColumn
}
