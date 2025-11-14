package configgen

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

//go:embed templates/*
var templatesFS embed.FS

// Generator generates service configuration files.
type Generator struct {
	log       logrus.FieldLogger
	cfg       *config.LabConfig
	overrides *config.CBTOverridesConfig
}

// NewGenerator creates a new Generator instance.
func NewGenerator(log logrus.FieldLogger, cfg *config.LabConfig, overrides *config.CBTOverridesConfig) *Generator {
	return &Generator{
		log:       log.WithField("component", "config-generator"),
		cfg:       cfg,
		overrides: overrides,
	}
}

// GenerateCBTConfig generates CBT configuration for a network.
func (g *Generator) GenerateCBTConfig(network string, overridesPath string) (string, error) {
	// Assign metrics ports: 9100, 9101, 9102, etc. (leave room for other services)
	metricsPort := 9100
	redisDB := 0

	for i, net := range g.cfg.EnabledNetworks() {
		if net.Name == network {
			metricsPort = 9100 + i
			redisDB = i // mainnet=0, sepolia=1, holesky=2, etc.

			break
		}
	}

	// Determine the external ClickHouse database name
	// - If Xatu mode is "local", use "default" (local Xatu cluster uses default database)
	// - If Xatu mode is "external", use configured ExternalDatabase (or "default" if not set)
	externalDatabase := "default"
	if g.cfg.Infrastructure.ClickHouse.Xatu.Mode == "external" && g.cfg.Infrastructure.ClickHouse.Xatu.ExternalDatabase != "" {
		externalDatabase = g.cfg.Infrastructure.ClickHouse.Xatu.ExternalDatabase
	}

	data := map[string]interface{}{
		"Network":                    network,
		"MetricsPort":                metricsPort,
		"RedisDB":                    redisDB,
		"IsHybrid":                   g.cfg.Mode == "hybrid",
		"XatuMode":                   g.cfg.Infrastructure.ClickHouse.Xatu.Mode,
		"ExternalClickHouseURL":      g.cfg.Infrastructure.ClickHouse.Xatu.ExternalURL,
		"ExternalClickHouseDatabase": externalDatabase,
		"ExternalClickHouseUsername": g.cfg.Infrastructure.ClickHouse.Xatu.ExternalUsername,
		"ExternalClickHousePassword": g.cfg.Infrastructure.ClickHouse.Xatu.ExternalPassword,
		"OverridesPath":              overridesPath,
		"HasOverrides":               overridesPath != "",
	}

	tmpl, err := template.New("cbt-config").ParseFS(templatesFS, "templates/cbt.yaml.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse CBT config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "cbt-config", data); err != nil {
		return "", fmt.Errorf("failed to execute CBT config template: %w", err)
	}

	return buf.String(), nil
}

// GenerateCBTAPIConfig generates cbt-api configuration for a network.
func (g *Generator) GenerateCBTAPIConfig(network string) (string, error) {
	port := g.cfg.GetCBTAPIPort(network)

	// Assign metrics ports: 9200, 9201, 9202, etc. (separate range from CBT engines)
	metricsPort := 9200

	for i, net := range g.cfg.EnabledNetworks() {
		if net.Name == network {
			metricsPort = 9200 + i

			break
		}
	}

	data := map[string]interface{}{
		"Network":     network,
		"Port":        port,
		"MetricsPort": metricsPort,
	}

	tmpl, err := template.New("cbt-api-config").ParseFS(templatesFS, "templates/cbt-api.yaml.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse cbt-api config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "cbt-api-config", data); err != nil {
		return "", fmt.Errorf("failed to execute cbt-api config template: %w", err)
	}

	return buf.String(), nil
}

// GenerateLabBackendConfig generates lab-backend configuration.
func (g *Generator) GenerateLabBackendConfig() (string, error) {
	networks := make([]map[string]interface{}, 0, len(g.cfg.Networks))
	for _, net := range g.cfg.EnabledNetworks() {
		networks = append(networks, map[string]interface{}{
			"Name": net.Name,
			"Port": g.cfg.GetCBTAPIPort(net.Name),
		})
	}

	data := map[string]interface{}{
		"Networks":     networks,
		"Port":         g.cfg.Ports.LabBackend,
		"FrontendPort": g.cfg.Ports.LabFrontend,
	}

	tmpl, err := template.New("lab-backend-config").ParseFS(templatesFS, "templates/lab-backend.yaml.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse lab-backend config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "lab-backend-config", data); err != nil {
		return "", fmt.Errorf("failed to execute lab-backend config template: %w", err)
	}

	return buf.String(), nil
}

// GenerateCBTOverrides generates CBT model overrides configuration.
// This creates an overrides.yaml file in CBT format by:
// 1. Generating default overrides (configurable backfill limit from .xcli.yaml)
// 2. Discovering all models from xatu-cbt
// 3. Applying default limits to all discovered models
// 4. Merging with user-provided overrides (if any)
// 5. Converting to CBT's format.
func (g *Generator) GenerateCBTOverrides(network string) (string, error) {
	// Find network config to get optional genesis timestamp
	var genesisTimestamp uint64

	for _, net := range g.cfg.EnabledNetworks() {
		if net.Name == network {
			genesisTimestamp = net.GenesisTimestamp

			break
		}
	}

	// Generate default overrides with configured backfill duration
	backfillDuration := g.cfg.CBT.DefaultBackfillDuration
	defaultOverrides := config.GenerateDefaultOverrides(network, backfillDuration, genesisTimestamp)

	// Discover all transformation models from xatu-cbt repo
	modelNames, err := g.discoverTransformationModels()
	if err != nil {
		g.log.WithError(err).Warn("failed to discover models, continuing without model-specific defaults")
	} else if len(modelNames) > 0 {
		// Apply default limits to all discovered models
		defaultOverrides.ApplyDefaultLimitsToAllModels(modelNames)
	}

	// Discover scheduled transformation models and set faster schedules for development
	scheduledModels, err := g.discoverScheduledTransformationModels()
	if err != nil {
		g.log.WithError(err).Warn("failed to discover scheduled models")
	} else if len(scheduledModels) > 0 {
		// Apply faster schedules for development (@every 5s instead of 24h)
		defaultOverrides.ApplyScheduleOverrides(scheduledModels, "@every 5s")
	}

	// Discover external models and apply lag settings to prevent full table scans
	externalModels, err := g.discoverExternalModels()
	if err != nil {
		g.log.WithError(err).Warn("failed to discover external models")
	} else if len(externalModels) > 0 {
		// Apply lag settings for external models
		defaultOverrides.ApplyLagOverrides(externalModels)
	}

	// Merge with user overrides (user takes precedence)
	finalOverrides := config.MergeOverrides(defaultOverrides, g.overrides)

	// Convert to CBT format
	cbtOverrides := finalOverrides.ToCBTOverrides()
	if len(cbtOverrides) == 0 {
		// No overrides to apply - output empty structure with comment
		g.log.Warn("no model overrides generated - check that xatu-cbt models are accessible")

		return "# No model overrides configured\nmodels: {}\n", nil
	}

	// Add models wrapper for CBT format
	result := map[string]interface{}{
		"models": cbtOverrides,
	}

	// Marshal to YAML
	data, err := yaml.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal CBT overrides: %w", err)
	}

	return string(data), nil
}

// discoverTransformationModels scans the xatu-cbt models directory and returns all transformation model names.
// Models are identified by .sql files in the models/transformations directory.
func (g *Generator) discoverTransformationModels() ([]string, error) {
	xatuCbtPath := g.cfg.Repos.XatuCBT
	if xatuCbtPath == "" {
		return nil, fmt.Errorf("xatu-cbt repo path not configured")
	}

	// Path to transformations models
	modelsPath := filepath.Join(xatuCbtPath, "models", "transformations")

	// Check if directory exists
	if _, err := os.Stat(modelsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("models directory not found: %s", modelsPath)
	}

	var modelNames []string

	// Walk the transformations directory
	err := filepath.WalkDir(modelsPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Only process .sql files
		if !strings.HasSuffix(d.Name(), ".sql") {
			return nil
		}

		// Extract model name (filename without .sql extension)
		modelName := strings.TrimSuffix(d.Name(), ".sql")
		modelNames = append(modelNames, modelName)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan models directory: %w", err)
	}

	g.log.WithField("count", len(modelNames)).Debug("discovered transformation models")

	return modelNames, nil
}

// discoverScheduledTransformationModels scans transformation models and returns only scheduled ones.
// Scheduled models use "type: scheduled" instead of "type: incremental" in their frontmatter.
func (g *Generator) discoverScheduledTransformationModels() ([]string, error) {
	xatuCbtPath := g.cfg.Repos.XatuCBT
	if xatuCbtPath == "" {
		return nil, fmt.Errorf("xatu-cbt repo path not configured")
	}

	// Path to transformations models
	modelsPath := filepath.Join(xatuCbtPath, "models", "transformations")

	// Check if directory exists
	if _, err := os.Stat(modelsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("models directory not found: %s", modelsPath)
	}

	var scheduledModels []string

	// Walk the transformations directory
	err := filepath.WalkDir(modelsPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Only process .sql files
		if !strings.HasSuffix(d.Name(), ".sql") {
			return nil
		}

		// Read file to check if it's a scheduled model
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			g.log.WithError(readErr).Warnf("failed to read model file: %s", path)

			return nil // Continue processing other files
		}

		// Check if the frontmatter contains "type: scheduled"
		if strings.Contains(string(content), "type: scheduled") {
			modelName := strings.TrimSuffix(d.Name(), ".sql")
			scheduledModels = append(scheduledModels, modelName)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan models directory: %w", err)
	}

	g.log.WithField("count", len(scheduledModels)).Debug("discovered scheduled transformation models")

	return scheduledModels, nil
}

// discoverExternalModels scans external models directory and returns model names with suggested lag settings.
// Returns a map of model_name -> lag_value.
func (g *Generator) discoverExternalModels() (map[string]int, error) {
	xatuCbtPath := g.cfg.Repos.XatuCBT
	if xatuCbtPath == "" {
		return nil, fmt.Errorf("xatu-cbt repo path not configured")
	}

	// Path to external models
	modelsPath := filepath.Join(xatuCbtPath, "models", "external")

	// Check if directory exists
	if _, err := os.Stat(modelsPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("external models directory not found: %s", modelsPath)
	}

	externalModels := make(map[string]int)

	// Walk the external models directory
	err := filepath.WalkDir(modelsPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Only process .sql files
		if !strings.HasSuffix(d.Name(), ".sql") {
			return nil
		}

		// Extract model name (filename without .sql extension)
		modelName := strings.TrimSuffix(d.Name(), ".sql")

		// Determine lag based on model type
		// canonical_* models use lag=0, others use lag=12
		lag := 12
		if strings.HasPrefix(modelName, "canonical_") {
			lag = 0
		}

		externalModels[modelName] = lag

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan external models directory: %w", err)
	}

	g.log.WithField("count", len(externalModels)).Debug("discovered external models")

	return externalModels, nil
}
