package seeddata

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
)

// BatchGenerateOptions contains options for batch seed data generation.
type BatchGenerateOptions struct {
	TransformationModel string   // The transformation model name (used for parquet naming)
	ExternalModels      []string // List of external model names to generate
	Network             string   // Network name (e.g., "mainnet", "sepolia")
	RangeColumn         string   // Column to filter on (e.g., "slot_start_date_time")
	From                string   // Range start value
	To                  string   // Range end value
	Filters             []Filter // Additional filters (applied to all models)
	Limit               int      // Max rows per model (0 = unlimited)
	OutputDir           string   // Output directory for parquet files
	SanitizeIPs         bool     // Enable IP address sanitization
	Salt                string   // Salt for IP sanitization (shared across all models)
}

// BatchGenerateResult contains the result of batch seed data generation.
type BatchGenerateResult struct {
	Results map[string]*GenerateResult // model name -> generate result
}

// BatchGenerate generates seed data for all external models with consistent range.
// Parquet files are named as {transformation}_{external_model}.parquet.
func (g *Generator) BatchGenerate(ctx context.Context, opts BatchGenerateOptions) (*BatchGenerateResult, error) {
	if len(opts.ExternalModels) == 0 {
		return nil, fmt.Errorf("no external models specified")
	}

	if opts.Network == "" {
		return nil, fmt.Errorf("network is required")
	}

	if opts.OutputDir == "" {
		opts.OutputDir = "."
	}

	result := &BatchGenerateResult{
		Results: make(map[string]*GenerateResult, len(opts.ExternalModels)),
	}

	for _, model := range opts.ExternalModels {
		// Build output filename: {transformation}_{external_model}.parquet
		filename := fmt.Sprintf("%s_%s.parquet", opts.TransformationModel, model)
		outputPath := fmt.Sprintf("%s/%s", opts.OutputDir, filename)

		g.log.WithFields(logrus.Fields{
			"transformation": opts.TransformationModel,
			"external_model": model,
			"output":         outputPath,
		}).Info("generating seed data for external model")

		genOpts := GenerateOptions{
			Model:       model,
			Network:     opts.Network,
			RangeColumn: opts.RangeColumn,
			From:        opts.From,
			To:          opts.To,
			Filters:     opts.Filters,
			Limit:       opts.Limit,
			OutputPath:  outputPath,
			SanitizeIPs: opts.SanitizeIPs,
			Salt:        opts.Salt,
		}

		genResult, err := g.Generate(ctx, genOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to generate seed data for %s: %w", model, err)
		}

		result.Results[model] = genResult
	}

	return result, nil
}

// GetParquetFilename returns the parquet filename for an external model within a transformation.
func GetParquetFilename(transformationModel, externalModel string) string {
	return fmt.Sprintf("%s_%s.parquet", transformationModel, externalModel)
}
