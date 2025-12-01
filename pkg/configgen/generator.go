// Package configgen generates service configuration files from templates
// for CBT engines, APIs, and other lab services.
package configgen

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"dario.cat/mergo"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

//go:embed templates/*
var templatesFS embed.FS

const (
	// cbtMetricsPortBase is the base port for CBT engine metrics endpoints.
	cbtMetricsPortBase = 9100
	// cbtAPIMetricsPortBase is the base port for CBT API metrics endpoints.
	cbtAPIMetricsPortBase = 9200
)

// Generator generates service configuration files.
type Generator struct {
	log logrus.FieldLogger
	cfg *config.LabConfig
}

// NewGenerator creates a new Generator instance.
func NewGenerator(log logrus.FieldLogger, cfg *config.LabConfig) *Generator {
	return &Generator{
		log: log.WithField("component", "config-generator"),
		cfg: cfg,
	}
}

// GenerateCBTConfig generates CBT configuration for a network.
// It generates a base config from template, then deep merges auto-generated
// defaults and user overrides on top. User overrides take ultimate precedence.
func (g *Generator) GenerateCBTConfig(network string, userOverridesPath string) (string, error) {
	metricsPort := cbtMetricsPortBase
	redisDB := 0

	for i, net := range g.cfg.EnabledNetworks() {
		if net.Name == network {
			metricsPort = cbtMetricsPortBase + i
			redisDB = i // mainnet=0, sepolia=1, holesky=2, etc.

			break
		}
	}

	// Determine the external ClickHouse database name
	// - If Xatu mode is "local", use "default" (local Xatu cluster uses default database)
	// - If Xatu mode is "external", use configured ExternalDatabase (or "default" if not set)
	externalDatabase := "default"
	if g.cfg.Infrastructure.ClickHouse.Xatu.Mode == constants.InfraModeExternal && g.cfg.Infrastructure.ClickHouse.Xatu.ExternalDatabase != "" {
		externalDatabase = g.cfg.Infrastructure.ClickHouse.Xatu.ExternalDatabase
	}

	frontendPort := g.cfg.GetCBTFrontendPort(network)

	var genesisTimestamp uint64
	if timestamp, ok := constants.NetworkGenesisTimestamps[network]; ok {
		genesisTimestamp = timestamp
	}

	data := map[string]interface{}{
		"Network":                    network,
		"MetricsPort":                metricsPort,
		"RedisDB":                    redisDB,
		"FrontendPort":               frontendPort,
		"GenesisTimestamp":           genesisTimestamp,
		"ExternalClickHouseDatabase": externalDatabase,
	}

	tmpl, err := template.New("cbt-config").ParseFS(templatesFS, "templates/cbt.yaml.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse CBT config template: %w", err)
	}

	var buf bytes.Buffer

	err = tmpl.ExecuteTemplate(&buf, "cbt-config", data)
	if err != nil {
		return "", fmt.Errorf("failed to execute CBT config template: %w", err)
	}

	// Parse base config to map for merging
	var baseConfig map[string]interface{}

	err = yaml.Unmarshal(buf.Bytes(), &baseConfig)
	if err != nil {
		return "", fmt.Errorf("failed to parse base config: %w", err)
	}

	// Generate auto-defaults (env, model overrides for backfill limits, schedules, lag)
	autoDefaults, err := g.generateAutoDefaults(network)
	if err != nil {
		g.log.WithError(err).Warn("failed to generate auto-defaults, continuing without them")
	} else {
		// Deep merge auto-defaults into base config
		err = mergo.Merge(&baseConfig, autoDefaults, mergo.WithOverride)
		if err != nil {
			return "", fmt.Errorf("failed to merge auto-defaults: %w", err)
		}
	}

	// Load and merge user overrides if file exists
	if userOverridesPath != "" {
		userOverrides, loadErr := loadYAMLFile(userOverridesPath)
		if loadErr != nil {
			g.log.WithError(loadErr).Warn("failed to load user overrides, continuing without them")
		} else if len(userOverrides) > 0 {
			// Deep merge user overrides (user takes ultimate precedence)
			err = mergo.Merge(&baseConfig, userOverrides, mergo.WithOverride)
			if err != nil {
				return "", fmt.Errorf("failed to merge user overrides: %w", err)
			}

			g.log.WithField("path", userOverridesPath).Debug("merged user CBT overrides")
		}
	}

	// Marshal final merged config
	finalYAML, err := yaml.Marshal(baseConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal final config: %w", err)
	}

	return string(finalYAML), nil
}

// generateAutoDefaults creates xcli-generated defaults for models (env, overrides).
func (g *Generator) generateAutoDefaults(network string) (map[string]interface{}, error) {
	externalModelMinTimestamp := time.Now().Add(-1 * time.Hour).Unix()

	// Set sane default for mainnet
	externalModelMinBlock := 0
	if network == "mainnet" {
		externalModelMinBlock = 23800000
	}

	// Build models section with env and overrides
	modelsSection := map[string]interface{}{
		"env": map[string]interface{}{
			"NETWORK":                      network,
			"EXTERNAL_MODEL_MIN_TIMESTAMP": fmt.Sprintf("%d", externalModelMinTimestamp),
			"EXTERNAL_MODEL_MIN_BLOCK":     fmt.Sprintf("%d", externalModelMinBlock),
		},
	}

	// Generate model overrides (backfill limits, schedules, lag)
	overridesConfig, err := g.GenerateCBTOverridesConfig(network)
	if err != nil {
		return nil, fmt.Errorf("failed to generate overrides config: %w", err)
	}

	if overridesConfig != nil {
		cbtFormat := overridesConfig.ToCBTOverrides()
		if overrides, ok := cbtFormat["overrides"]; ok {
			modelsSection["overrides"] = overrides
		}
	}

	return map[string]interface{}{
		"models": modelsSection,
	}, nil
}

// loadYAMLFile loads a YAML file as a generic map.
// Returns an empty map if the file doesn't exist.
func loadYAMLFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}

		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return result, nil
}

// GenerateCBTAPIConfig generates cbt-api configuration for a network.
func (g *Generator) GenerateCBTAPIConfig(network string) (string, error) {
	port := g.cfg.GetCBTAPIPort(network)
	metricsPort := cbtAPIMetricsPortBase

	for i, net := range g.cfg.EnabledNetworks() {
		if net.Name == network {
			metricsPort = cbtAPIMetricsPortBase + i

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
	for _, net := range g.cfg.Networks {
		networks = append(networks, map[string]interface{}{
			"Name":    net.Name,
			"Port":    g.cfg.GetCBTAPIPort(net.Name),
			"Enabled": net.Enabled,
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

// GenerateCBTOverridesConfig generates auto-generated CBT model overrides.
// This returns only the xcli-generated defaults (backfill limits, schedules, lag).
// User overrides are merged separately in GenerateCBTConfig.
func (g *Generator) GenerateCBTOverridesConfig(network string) (*config.CBTOverridesConfig, error) {
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

	return defaultOverrides, nil
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
