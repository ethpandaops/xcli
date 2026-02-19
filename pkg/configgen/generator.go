// Package configgen generates service configuration files from templates
// for CBT engines, APIs, and other lab services.
package configgen

import (
	"bytes"
	"embed"
	"fmt"
	"os"
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
			removeEmptyMaps(userOverrides)
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

	// Build models section with env
	modelsSection := map[string]interface{}{
		"env": map[string]interface{}{
			"NETWORK":                      network,
			"EXTERNAL_MODEL_MIN_TIMESTAMP": fmt.Sprintf("%d", externalModelMinTimestamp),
			"EXTERNAL_MODEL_MIN_BLOCK":     fmt.Sprintf("%d", externalModelMinBlock),
			"MODELS_SCRIPTS_PATH":          "../xatu-cbt/models/scripts",
		},
	}

	return map[string]interface{}{
		"models": modelsSection,
	}, nil
}

// removeEmptyMaps recursively removes empty map entries from a map.
// This prevents YAML keys with only comments (parsed as empty maps)
// from overriding populated auto-defaults during mergo merge.
func removeEmptyMaps(m map[string]interface{}) {
	for key, val := range m {
		if nested, ok := val.(map[string]interface{}); ok {
			removeEmptyMaps(nested)

			if len(nested) == 0 {
				delete(m, key)
			}
		}
	}
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
