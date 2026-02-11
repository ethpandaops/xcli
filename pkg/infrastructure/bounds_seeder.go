package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
)

const (
	// Production cluster hardcoded details.
	prodK8sContext    = "platform-analytics-hel1-production"
	prodNamespace     = "xatu"
	prodClickHousePod = "chi-xatu-cbt-clickhouse-replicated-0-0-0"

	// localCBTClickHouseContainer is the local Docker container name for xatu-cbt ClickHouse.
	localCBTClickHouseContainer = "xatu-cbt-clickhouse-01"
)

// BoundsSeeder handles seeding CBT bounds from production ClickHouse.
type BoundsSeeder struct {
	cfg *config.LabConfig
	log logrus.FieldLogger
}

// IncrementalBound represents a single incremental model bound.
type IncrementalBound struct {
	Database string `json:"database"`
	Table    string `json:"table"`
	Position uint64 `json:"position,string"`
	Interval uint64 `json:"interval,string"`
}

// ScheduledBound represents a single scheduled model bound.
type ScheduledBound struct {
	Database      string `json:"database"`
	Table         string `json:"table"`
	StartDateTime string `json:"startDateTime"`
}

// NewBoundsSeeder creates a new bounds seeder.
func NewBoundsSeeder(cfg *config.LabConfig, log logrus.FieldLogger) *BoundsSeeder {
	return &BoundsSeeder{
		cfg: cfg,
		log: log.WithField("component", "bounds_seeder"),
	}
}

// SeedFromProduction fetches bounds from production and inserts into local ClickHouse.
func (s *BoundsSeeder) SeedFromProduction(ctx context.Context, network string, clickhouseURL string) error {
	s.log.Info("Fetching bounds from production xatu-cbt")

	// Check kubectl is available
	if err := s.checkKubectl(ctx); err != nil {
		return fmt.Errorf("kubectl not available: %w", err)
	}

	// Seed incremental bounds
	s.log.Debug("Fetching incremental bounds...")

	incrementalBounds, err := s.fetchIncrementalBounds(ctx, network)
	if err != nil {
		return fmt.Errorf("fetching incremental bounds: %w", err)
	}

	s.log.WithField("count", len(incrementalBounds)).Info("Fetched incremental bounds")

	// Seed scheduled bounds
	s.log.Debug("Fetching scheduled bounds...")

	scheduledBounds, err := s.fetchScheduledBounds(ctx, network)
	if err != nil {
		return fmt.Errorf("fetching scheduled bounds: %w", err)
	}

	s.log.WithField("count", len(scheduledBounds)).Info("Fetched scheduled bounds")

	// Insert into local ClickHouse
	if err := s.insertIncrementalBounds(ctx, network, clickhouseURL, incrementalBounds); err != nil {
		return fmt.Errorf("inserting incremental bounds: %w", err)
	}

	if err := s.insertScheduledBounds(ctx, network, clickhouseURL, scheduledBounds); err != nil {
		return fmt.Errorf("inserting scheduled bounds: %w", err)
	}

	s.log.Info("Bounds seeded from production")

	return nil
}

// checkKubectl verifies kubectl is available.
func (s *BoundsSeeder) checkKubectl(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "kubectl", "version", "--client")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl command failed: %w", err)
	}

	return nil
}

// fetchIncrementalBounds queries production for incremental bounds.
func (s *BoundsSeeder) fetchIncrementalBounds(ctx context.Context, network string) ([]IncrementalBound, error) {
	query := fmt.Sprintf(`
		SELECT
			database,
			table,
			argMax(position, updated_date_time) as position,
			argMax(interval, updated_date_time) as interval
		FROM %s.admin_cbt_incremental
		GROUP BY database, table
		FORMAT JSON
	`, network)

	output, err := s.execClickHouseQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []IncrementalBound `json:"data"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON response: %w", err)
	}

	return result.Data, nil
}

// fetchScheduledBounds queries production for scheduled bounds.
func (s *BoundsSeeder) fetchScheduledBounds(ctx context.Context, network string) ([]ScheduledBound, error) {
	query := fmt.Sprintf(`
		SELECT
			database,
			table,
			max(start_date_time) as start_date_time
		FROM %s.admin_cbt_scheduled
		GROUP BY database, table
		FORMAT JSON
	`, network)

	output, err := s.execClickHouseQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []ScheduledBound `json:"data"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parsing JSON response: %w", err)
	}

	return result.Data, nil
}

// execClickHouseQuery executes a query on production ClickHouse via kubectl.
func (s *BoundsSeeder) execClickHouseQuery(ctx context.Context, query string) ([]byte, error) {
	args := []string{
		"--context", prodK8sContext,
		"-n", prodNamespace,
		"exec", prodClickHousePod,
		"--",
		"clickhouse-client",
		"--query", query,
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)

	s.log.WithField("query", strings.Split(query, "\n")[1:3]).Debug("Executing kubectl query")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl exec failed: %w\nOutput: %s", err, string(output))
	}

	return output, nil
}

// insertIncrementalBounds inserts bounds into local ClickHouse.
func (s *BoundsSeeder) insertIncrementalBounds(
	ctx context.Context,
	network string,
	clickhouseURL string,
	bounds []IncrementalBound,
) error {
	if len(bounds) == 0 {
		s.log.Debug("No incremental bounds to insert")

		return nil
	}

	s.log.WithField("count", len(bounds)).Debug("Inserting incremental bounds")

	for _, b := range bounds {
		insertSQL := fmt.Sprintf(`
			INSERT INTO %s.admin_cbt_incremental_local
			(updated_date_time, database, table, position, interval)
			VALUES (now(), '%s', '%s', %d, %d)
		`, network, b.Database, b.Table, b.Position, b.Interval)

		if err := s.execLocalClickHouseQuery(ctx, clickhouseURL, insertSQL); err != nil {
			s.log.WithError(err).WithFields(logrus.Fields{
				"database": b.Database,
				"table":    b.Table,
			}).Warn("Failed to insert incremental bound (non-fatal)")
		}
	}

	return nil
}

// insertScheduledBounds inserts bounds into local ClickHouse.
func (s *BoundsSeeder) insertScheduledBounds(
	ctx context.Context,
	network string,
	clickhouseURL string,
	bounds []ScheduledBound,
) error {
	if len(bounds) == 0 {
		s.log.Debug("No scheduled bounds to insert")

		return nil
	}

	s.log.WithField("count", len(bounds)).Debug("Inserting scheduled bounds")

	for _, b := range bounds {
		insertSQL := fmt.Sprintf(`
			INSERT INTO %s.admin_cbt_scheduled_local
			(updated_date_time, database, table, start_date_time)
			VALUES (now(), '%s', '%s', '%s')
		`, network, b.Database, b.Table, b.StartDateTime)

		if err := s.execLocalClickHouseQuery(ctx, clickhouseURL, insertSQL); err != nil {
			s.log.WithError(err).WithFields(logrus.Fields{
				"database": b.Database,
				"table":    b.Table,
			}).Warn("Failed to insert scheduled bound (non-fatal)")
		}
	}

	return nil
}

// execLocalClickHouseQuery executes a query on local ClickHouse via docker.
func (s *BoundsSeeder) execLocalClickHouseQuery(ctx context.Context, clickhouseURL string, query string) error {
	// Use docker exec to run clickhouse-client in the local xatu-cbt ClickHouse container.
	args := []string{
		"exec",
		localCBTClickHouseContainer,
		"clickhouse-client",
		"--query", query,
	}

	cmd := exec.CommandContext(ctx, "docker", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker exec clickhouse-client failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}
