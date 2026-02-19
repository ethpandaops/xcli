// Package configgen generates service configuration files from templates
// for CBT engines, APIs, and other lab services.
package configgen

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	xatuCfg := g.cfg.Infrastructure.ClickHouse.Xatu
	if xatuCfg.Mode == constants.InfraModeExternal &&
		xatuCfg.ExternalDatabase != "" {
		externalDatabase = g.cfg.Infrastructure.ClickHouse.Xatu.ExternalDatabase
	}

	frontendPort := g.cfg.GetCBTFrontendPort(network)

	var genesisTimestamp uint64
	if timestamp, ok := constants.NetworkGenesisTimestamps[network]; ok {
		genesisTimestamp = timestamp
	}

	data := map[string]any{
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
	var baseConfig map[string]any

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
func (g *Generator) generateAutoDefaults(network string) (map[string]any, error) {
	externalModelMinTimestamp := time.Now().Add(-1 * time.Hour).Unix()

	// Set sane default for mainnet
	externalModelMinBlock := 0
	if network == "mainnet" {
		externalModelMinBlock = 23800000
	}

	// Build models section with env
	modelsSection := map[string]any{
		"env": map[string]any{
			"NETWORK":                      network,
			"EXTERNAL_MODEL_MIN_TIMESTAMP": fmt.Sprintf("%d", externalModelMinTimestamp),
			"EXTERNAL_MODEL_MIN_BLOCK":     fmt.Sprintf("%d", externalModelMinBlock),
			"MODELS_SCRIPTS_PATH":          "../xatu-cbt/models/scripts",
		},
	}

	return map[string]any{
		"models": modelsSection,
	}, nil
}

// loadYAMLFile loads a YAML file as a generic map.
// Returns an empty map if the file doesn't exist.
func loadYAMLFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}

		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var result map[string]any
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

	data := map[string]any{
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
// In hybrid mode, userOverridesPath is used to determine which tables
// should be routed to the local cbt-api instead of the external one.
func (g *Generator) GenerateLabBackendConfig(
	userOverridesPath string,
) (string, error) {
	isHybrid := g.cfg.Infrastructure.ClickHouse.Xatu.Mode == constants.InfraModeExternal

	var localTables []string

	if isHybrid && userOverridesPath != "" {
		tables, err := g.getLocallyEnabledTables(userOverridesPath)
		if err != nil {
			g.log.WithError(err).Warn(
				"failed to determine local tables, falling back to non-hybrid",
			)

			isHybrid = false
		} else {
			localTables = tables
		}
	}

	networks := make([]map[string]any, 0, len(g.cfg.Networks))

	for _, net := range g.cfg.Networks {
		entry := map[string]any{
			"Name":    net.Name,
			"Port":    g.cfg.GetCBTAPIPort(net.Name),
			"Enabled": net.Enabled,
		}

		if isHybrid {
			if len(localTables) > 0 {
				entry["IsHybrid"] = true
				entry["LocalTables"] = localTables
			} else {
				// All models disabled — no local routing, pure Cartographoor.
				entry["IsHybrid"] = true
			}
		}

		networks = append(networks, entry)
	}

	data := map[string]any{
		"Networks":     networks,
		"Port":         g.cfg.Ports.LabBackend,
		"FrontendPort": g.cfg.Ports.LabFrontend,
	}

	tmpl, err := template.New("lab-backend-config").ParseFS(
		templatesFS, "templates/lab-backend.yaml.tmpl",
	)
	if err != nil {
		return "", fmt.Errorf("failed to parse lab-backend config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "lab-backend-config", data); err != nil {
		return "", fmt.Errorf("failed to execute lab-backend config template: %w", err)
	}

	return buf.String(), nil
}

// getLocallyEnabledTables discovers all models from the xatu-cbt repo and
// returns those NOT explicitly disabled in the overrides file.
// The overrides file is a deny list: it only contains disabled models.
// So we discover all models, then subtract the disabled ones.
func (g *Generator) getLocallyEnabledTables(overridesPath string) ([]string, error) {
	xatuCBTPath := g.cfg.Repos.XatuCBT
	if xatuCBTPath == "" {
		return nil, fmt.Errorf("xatu-cbt repo path not configured")
	}

	// Discover all models from the xatu-cbt repo.
	allModels, err := discoverAllModels(xatuCBTPath)
	if err != nil {
		return nil, fmt.Errorf("failed to discover models: %w", err)
	}

	if len(allModels) == 0 {
		return nil, nil
	}

	// Load overrides to find disabled models.
	disabled, err := loadDisabledModels(overridesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load overrides: %w", err)
	}

	// Return all models that are NOT disabled.
	tables := make([]string, 0, len(allModels))

	for _, name := range allModels {
		if !disabled[name] {
			tables = append(tables, name)
		}
	}

	return tables, nil
}

// discoverAllModels scans the xatu-cbt repo for external and transformation
// model files, returning a sorted list of all model names.
func discoverAllModels(xatuCBTPath string) ([]string, error) {
	models := make([]string, 0, 64)

	// Discover external models (.sql files).
	externalDir := filepath.Join(xatuCBTPath, "models", "external")

	entries, err := os.ReadDir(externalDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read external models directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".sql") {
			models = append(models, strings.TrimSuffix(name, ".sql"))
		}
	}

	// Discover transformation models (.sql, .yml, .yaml files).
	transformDir := filepath.Join(xatuCBTPath, "models", "transformations")

	entries, err = os.ReadDir(transformDir)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read transformations directory: %w", err,
		)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		for _, ext := range []string{".sql", ".yml", ".yaml"} {
			if strings.HasSuffix(name, ext) {
				models = append(models, strings.TrimSuffix(name, ext))

				break
			}
		}
	}

	sort.Strings(models)

	return models, nil
}

// loadDisabledModels parses the overrides file and returns a set of model
// names that are explicitly disabled (enabled: false).
func loadDisabledModels(overridesPath string) (map[string]bool, error) {
	data, err := os.ReadFile(overridesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]bool), nil
		}

		return nil, fmt.Errorf("failed to read overrides: %w", err)
	}

	var overrides struct {
		Models struct {
			Overrides map[string]struct {
				Enabled *bool `yaml:"enabled"`
			} `yaml:"overrides"`
		} `yaml:"models"`
	}

	if err := yaml.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("failed to parse overrides: %w", err)
	}

	disabled := make(map[string]bool, len(overrides.Models.Overrides))

	for name, override := range overrides.Models.Overrides {
		if override.Enabled != nil && !*override.Enabled {
			disabled[name] = true
		}
	}

	return disabled, nil
}
