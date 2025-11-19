package diagnostic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// timestampFormat is the format used in report filenames.
	timestampFormat = "20060102-150405"

	// defaultRetention is the default retention period for diagnostic reports.
	defaultRetention = 7 * 24 * time.Hour // 7 days
)

// Store handles persistence of diagnostic reports.
type Store struct {
	log     logrus.FieldLogger
	baseDir string
}

// NewStore creates a store with the given base directory.
func NewStore(log logrus.FieldLogger, baseDir string) *Store {
	return &Store{
		log:     log.WithField("component", "diagnostic-store"),
		baseDir: baseDir,
	}
}

// Save persists a rebuild report to disk.
// File format: {baseDir}/{timestamp}-{id}.json
// Timestamp format: 20060102-150405
func (s *Store) Save(report *RebuildReport) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(s.baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create errors directory: %w", err)
	}

	// Generate filename with timestamp and ID
	timestamp := report.StartTime.Format(timestampFormat)
	filename := fmt.Sprintf("%s-%s.json", timestamp, report.ID)
	filePath := filepath.Join(s.baseDir, filename)

	// Marshal report to JSON with indentation for readability
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write report file: %w", err)
	}

	s.log.WithFields(logrus.Fields{
		"id":   report.ID,
		"file": filename,
	}).Debug("saved diagnostic report")

	// Automatically clean up old reports to prevent buildup
	if err := s.Cleanup(defaultRetention); err != nil {
		s.log.WithError(err).Warn("failed to cleanup old diagnostic reports")
	}

	return nil
}

// Load retrieves a report by ID (searches for file containing ID).
func (s *Store) Load(id string) (*RebuildReport, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("report not found: %s", id)
		}

		return nil, fmt.Errorf("failed to read errors directory: %w", err)
	}

	// Search for file containing the ID
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		// Check if filename contains the ID
		if strings.Contains(name, id) {
			filePath := filepath.Join(s.baseDir, name)

			data, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read report file: %w", err)
			}

			var report RebuildReport
			if err := json.Unmarshal(data, &report); err != nil {
				return nil, fmt.Errorf("failed to unmarshal report: %w", err)
			}

			s.log.WithFields(logrus.Fields{
				"id":   id,
				"file": name,
			}).Debug("loaded diagnostic report")

			return &report, nil
		}
	}

	return nil, fmt.Errorf("report not found: %s", id)
}

// List returns recent reports, newest first.
func (s *Store) List(limit int) ([]*RebuildReport, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*RebuildReport{}, nil
		}

		return nil, fmt.Errorf("failed to read errors directory: %w", err)
	}

	// Filter for JSON files
	var jsonFiles []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".json") {
			jsonFiles = append(jsonFiles, name)
		}
	}

	// Sort by filename descending (newest first since filenames start with timestamp)
	sort.Sort(sort.Reverse(sort.StringSlice(jsonFiles)))

	// Apply limit
	if limit > 0 && len(jsonFiles) > limit {
		jsonFiles = jsonFiles[:limit]
	}

	// Load each report
	reports := make([]*RebuildReport, 0, len(jsonFiles))

	for _, name := range jsonFiles {
		filePath := filepath.Join(s.baseDir, name)

		data, err := os.ReadFile(filePath)
		if err != nil {
			s.log.WithFields(logrus.Fields{
				"file":  name,
				"error": err,
			}).Warn("failed to read report file, skipping")

			continue
		}

		var report RebuildReport
		if err := json.Unmarshal(data, &report); err != nil {
			s.log.WithFields(logrus.Fields{
				"file":  name,
				"error": err,
			}).Warn("failed to unmarshal report, skipping")

			continue
		}

		reports = append(reports, &report)
	}

	s.log.WithField("count", len(reports)).Debug("listed diagnostic reports")

	return reports, nil
}

// Latest returns the most recent report.
func (s *Store) Latest() (*RebuildReport, error) {
	reports, err := s.List(1)
	if err != nil {
		return nil, err
	}

	if len(reports) == 0 {
		return nil, fmt.Errorf("no reports found")
	}

	return reports[0], nil
}

// Cleanup removes reports older than retention period.
func (s *Store) Cleanup(retention time.Duration) error {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("failed to read errors directory: %w", err)
	}

	cutoff := time.Now().Add(-retention)
	var removed int

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		// Parse timestamp from filename (format: 20060102-150405-{id}.json)
		// Extract timestamp portion (first 15 characters: YYYYMMDD-HHMMSS)
		if len(name) < len(timestampFormat) {
			continue
		}

		timestampStr := name[:len(timestampFormat)]

		fileTime, err := time.Parse(timestampFormat, timestampStr)
		if err != nil {
			s.log.WithFields(logrus.Fields{
				"file":  name,
				"error": err,
			}).Warn("failed to parse timestamp from filename, skipping")

			continue
		}

		// Remove if older than retention period
		if fileTime.Before(cutoff) {
			filePath := filepath.Join(s.baseDir, name)

			if err := os.Remove(filePath); err != nil {
				s.log.WithFields(logrus.Fields{
					"file":  name,
					"error": err,
				}).Warn("failed to remove old report")

				continue
			}

			removed++
		}
	}

	if removed > 0 {
		s.log.WithFields(logrus.Fields{
			"removed":   removed,
			"retention": retention.String(),
		}).Info("cleaned up old diagnostic reports")
	}

	return nil
}

// GetErrorsDir returns the errors directory path.
func (s *Store) GetErrorsDir() string {
	return s.baseDir
}
