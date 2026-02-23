// Package config handles loading, validation, and management of xcli configuration
// including lab stack settings, network configurations, and infrastructure options.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ethpandaops/xcli/pkg/constants"
)

// Config represents the xcli root configuration
// It contains stack-specific configurations and optional workspace-level settings.
type Config struct {
	// Lab stack configuration
	Lab *LabConfig `yaml:"lab,omitempty"`

	// Xatu stack configuration (docker-compose based)
	Xatu *XatuConfig `yaml:"xatu,omitempty"`

	// Workspace-level settings (optional, for future use)
	// Workspace *WorkspaceConfig `yaml:"workspace,omitempty"`
}

// LabConfig represents the lab stack configuration.
type LabConfig struct {
	Repos          LabReposConfig       `yaml:"repos"`
	Mode           string               `yaml:"mode"`
	Networks       []NetworkConfig      `yaml:"networks"`
	Infrastructure InfrastructureConfig `yaml:"infrastructure"`
	Ports          LabPortsConfig       `yaml:"ports"`
	Dev            LabDevConfig         `yaml:"dev"`
	TUI            TUIConfig            `yaml:"tui"`
}

// LabReposConfig contains paths to lab repositories.
type LabReposConfig struct {
	CBT        string `yaml:"cbt"`
	XatuCBT    string `yaml:"xatuCbt"`
	CBTAPI     string `yaml:"cbtApi"`
	LabBackend string `yaml:"labBackend"`
	Lab        string `yaml:"lab"`
}

// NetworkConfig represents a network configuration.
type NetworkConfig struct {
	Name             string `yaml:"name"`
	Enabled          bool   `yaml:"enabled"`
	PortOffset       int    `yaml:"portOffset"`
	GenesisTimestamp uint64 `yaml:"genesisTimestamp,omitempty"` // Optional: Unix timestamp for custom networks
}

// InfrastructureConfig contains infrastructure settings.
type InfrastructureConfig struct {
	ClickHouse         ClickHouseConfig    `yaml:"clickhouse"`
	Redis              RedisConfig         `yaml:"redis"`
	Volumes            VolumesConfig       `yaml:"volumes"`
	Observability      ObservabilityConfig `yaml:"observability"`
	ClickHouseXatuPort int                 `yaml:"clickhouseXatuPort"`
	ClickHouseCBTPort  int                 `yaml:"clickhouseCbtPort"`
	RedisPort          int                 `yaml:"redisPort"`
}

// ClickHouseConfig contains ClickHouse cluster configuration.
type ClickHouseConfig struct {
	Xatu ClickHouseClusterConfig `yaml:"xatu"`
	CBT  ClickHouseClusterConfig `yaml:"cbt"`
}

// ClickHouseClusterConfig represents a ClickHouse cluster.
type ClickHouseClusterConfig struct {
	Mode             string `yaml:"mode"` // "local" or "external"
	ExternalURL      string `yaml:"externalUrl,omitempty"`
	ExternalDatabase string `yaml:"externalDatabase,omitempty"`
	ExternalUsername string `yaml:"externalUsername,omitempty"`
	ExternalPassword string `yaml:"externalPassword,omitempty"`
}

// RedisConfig contains Redis configuration.
type RedisConfig struct {
	Port int `yaml:"port"`
}

// VolumesConfig contains volume settings.
type VolumesConfig struct {
	Persist bool `yaml:"persist"`
}

// ObservabilityConfig contains observability stack settings (Prometheus + Grafana).
type ObservabilityConfig struct {
	Enabled        bool `yaml:"enabled"`
	PrometheusPort int  `yaml:"prometheusPort,omitempty"`
	GrafanaPort    int  `yaml:"grafanaPort,omitempty"`
}

// LabPortsConfig contains lab stack port assignments.
type LabPortsConfig struct {
	LabBackend      int `yaml:"labBackend"`
	LabFrontend     int `yaml:"labFrontend"`
	CBTBase         int `yaml:"cbtBase"`
	CBTAPIBase      int `yaml:"cbtApiBase"`
	CBTFrontendBase int `yaml:"cbtFrontendBase"`
}

// LabDevConfig contains lab stack development features.
type LabDevConfig struct {
	LabRebuildOnChange bool   `yaml:"labRebuildOnChange"`
	HotReload          bool   `yaml:"hotReload"`
	XatuRef            string `yaml:"xatuRef,omitempty"` // Git ref (branch/tag/commit) for xatu repo, defaults to "master"
}

// TUIConfig contains TUI dashboard settings.
type TUIConfig struct {
	// MaxLogLines sets the maximum log lines to keep per service in the TUI.
	// Set to -1 for unlimited (warning: may consume significant memory).
	// Default: 1000000
	MaxLogLines int `yaml:"maxLogLines"`
}

// XatuConfig represents the xatu stack configuration (docker-compose based).
type XatuConfig struct {
	Repos        XatuReposConfig   `yaml:"repos"`
	Profiles     []string          `yaml:"profiles,omitempty"`     // Docker compose profiles
	EnvOverrides map[string]string `yaml:"envOverrides,omitempty"` // Override .env vars
}

// XatuReposConfig contains paths to xatu repositories.
type XatuReposConfig struct {
	Xatu string `yaml:"xatu"`
}

// LoadResult contains the loaded config and the resolved config file path.
type LoadResult struct {
	Config     *Config
	ConfigPath string // Absolute path to the config file that was loaded
}

// Save writes the configuration to a file.
func (c *Config) Save(path string) error {
	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write file
	//nolint:gosec // Config file permissions are intentionally 0644 for readability
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate checks if the root configuration is valid.
func (c *Config) Validate() error {
	// Validate lab config if present
	if c.Lab != nil {
		if err := c.Lab.Validate(); err != nil {
			return fmt.Errorf("lab config validation failed: %w", err)
		}
	}

	// Validate xatu config if present
	if c.Xatu != nil {
		if err := c.Xatu.Validate(); err != nil {
			return fmt.Errorf("xatu config validation failed: %w", err)
		}
	}

	return nil
}

// ValidateRepos checks if repository paths are valid.
func (c *LabConfig) ValidateRepos() error {
	repos := map[string]string{
		"cbt":         c.Repos.CBT,
		"xatu-cbt":    c.Repos.XatuCBT,
		"cbt-api":     c.Repos.CBTAPI,
		"lab-backend": c.Repos.LabBackend,
		"lab":         c.Repos.Lab,
	}

	for name, path := range repos {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("invalid path for %s: %w", name, err)
		}

		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			return fmt.Errorf("repository %s not found at: %s", name, absPath)
		}
	}

	return nil
}

// Validate checks if the lab configuration is valid.
func (c *LabConfig) Validate() error {
	// Check mode
	if c.Mode != constants.ModeLocal && c.Mode != constants.ModeHybrid {
		return fmt.Errorf("invalid mode: %s (must be '%s' or '%s')", c.Mode, constants.ModeLocal, constants.ModeHybrid)
	}

	// Check repo paths exist
	if err := c.ValidateRepos(); err != nil {
		return err
	}

	// Check at least one network is enabled
	hasEnabled := false

	for _, net := range c.Networks {
		if net.Enabled {
			hasEnabled = true

			break
		}
	}

	if !hasEnabled {
		return fmt.Errorf("at least one network must be enabled")
	}

	// Validate hybrid mode configuration
	if c.Mode == constants.ModeHybrid && c.Infrastructure.ClickHouse.Xatu.Mode == constants.InfraModeExternal {
		if c.Infrastructure.ClickHouse.Xatu.ExternalURL == "" {
			return fmt.Errorf("externalUrl is required for hybrid mode with external Xatu cluster\n" +
				"Add to .xcli.yaml:\n" +
				"  lab:\n" +
				"    infrastructure:\n" +
				"      clickhouse:\n" +
				"        xatu:\n" +
				"          externalUrl: \"https://username:password@prod-xatu.example.com:8443\"")
		}

		// Validate URL format
		if !strings.HasPrefix(c.Infrastructure.ClickHouse.Xatu.ExternalURL, "http://") &&
			!strings.HasPrefix(c.Infrastructure.ClickHouse.Xatu.ExternalURL, "https://") &&
			!strings.HasPrefix(c.Infrastructure.ClickHouse.Xatu.ExternalURL, "clickhouse://") {
			return fmt.Errorf("external_url must start with http://, https://, or clickhouse://")
		}
	}

	return nil
}

// EnabledNetworks returns a slice of enabled networks.
func (c *LabConfig) EnabledNetworks() []NetworkConfig {
	enabled := make([]NetworkConfig, 0, len(c.Networks))
	for _, net := range c.Networks {
		if net.Enabled {
			enabled = append(enabled, net)
		}
	}

	return enabled
}

// GetCBTPort returns the CBT port for a given network.
func (c *LabConfig) GetCBTPort(network string) int {
	for _, net := range c.Networks {
		if net.Name == network {
			return c.Ports.CBTBase + net.PortOffset
		}
	}

	return 0
}

// GetCBTAPIPort returns the cbt-api port for a given network.
func (c *LabConfig) GetCBTAPIPort(network string) int {
	for _, net := range c.Networks {
		if net.Name == network {
			return c.Ports.CBTAPIBase + net.PortOffset
		}
	}

	return 0
}

// GetCBTFrontendPort returns the CBT frontend port for a given network.
func (c *LabConfig) GetCBTFrontendPort(network string) int {
	for _, net := range c.Networks {
		if net.Name == network {
			return c.Ports.CBTFrontendBase + net.PortOffset
		}
	}

	return 0
}

// Validate checks if the xatu configuration is valid.
// Validate checks if the xatu configuration is valid.
func (c *XatuConfig) Validate() error {
	if c.Repos.Xatu == "" {
		return fmt.Errorf("xatu repo path is required")
	}

	absPath, err := filepath.Abs(c.Repos.Xatu)
	if err != nil {
		return fmt.Errorf("invalid xatu repo path: %w", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("xatu repo not found at: %s", absPath)
	}

	// Check docker-compose.yml exists
	composePath := filepath.Join(absPath, "docker-compose.yml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return fmt.Errorf("docker-compose.yml not found in xatu repo: %s", composePath)
	}

	return nil
}

// DefaultXatu returns a xatu configuration with sensible defaults.
func DefaultXatu() *XatuConfig {
	return &XatuConfig{}
}

// LoadXatuConfig loads and validates xatu configuration from the config file.
// Returns the xatu config and the config file path, or an error if loading fails
// or if the xatu configuration is not found.
func LoadXatuConfig(configPath string) (*XatuConfig, string, error) {
	result, err := Load(configPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load config: %w", err)
	}

	if result.Config.Xatu == nil {
		return nil, "", fmt.Errorf("xatu configuration not found - run 'xcli xatu init' first")
	}

	return result.Config.Xatu, result.ConfigPath, nil
}

// Default returns a root configuration with sensible defaults.
func Default() *Config {
	return &Config{
		Lab: DefaultLab(),
	}
}

// DefaultLab returns a lab configuration with sensible defaults.
func DefaultLab() *LabConfig {
	return &LabConfig{
		Repos: LabReposConfig{
			CBT:        "../cbt",
			XatuCBT:    "../xatu-cbt",
			CBTAPI:     "../cbt-api",
			LabBackend: "../lab-backend",
			Lab:        "../lab",
		},
		Mode: constants.ModeHybrid,
		Networks: []NetworkConfig{
			{Name: "mainnet", Enabled: true, PortOffset: 0},
			{Name: "sepolia", Enabled: false, PortOffset: 1},
			{Name: "hoodi", Enabled: false, PortOffset: 2},
		},
		Infrastructure: InfrastructureConfig{
			ClickHouse: ClickHouseConfig{
				Xatu: ClickHouseClusterConfig{
					Mode:             constants.InfraModeExternal,
					ExternalURL:      "http://chendpoint-xatu-clickhouse.analytics.production.ethpandaops:9000",
					ExternalDatabase: "default",
				},
				CBT: ClickHouseClusterConfig{Mode: constants.InfraModeLocal},
			},
			Redis:   RedisConfig{Port: 6380},
			Volumes: VolumesConfig{Persist: true},
			Observability: ObservabilityConfig{
				Enabled:        false,
				PrometheusPort: constants.DefaultPrometheusPort,
				GrafanaPort:    constants.DefaultGrafanaPort,
			},
			ClickHouseXatuPort: 8125,
			ClickHouseCBTPort:  8123,
			RedisPort:          6380,
		},
		Ports: LabPortsConfig{
			LabBackend:      8080,
			LabFrontend:     5173,
			CBTBase:         8081,
			CBTAPIBase:      8091,
			CBTFrontendBase: 8085,
		},
		Dev: LabDevConfig{
			LabRebuildOnChange: false,
			HotReload:          true,
		},
		TUI: TUIConfig{
			MaxLogLines: 1_000_000, // 1 million default
		},
	}
}

// Load reads and parses a config file
// Supports both old (flat) and new (namespaced) config formats for backward compatibility.
// If the path is ".xcli.yaml" (the default), it will search upward through parent directories.
// Returns the config and the resolved absolute path to the config file.
func Load(path string) (*LoadResult, error) {
	// If using default path, search for it in parent directories
	if path == ".xcli.yaml" {
		path = FindConfig(path)
	}

	// Make path absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path // Fallback to original if Abs fails
	}

	// Check if file exists
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		// If original path was searched for and not found, return default
		return &LoadResult{
			Config:     Default(),
			ConfigPath: absPath,
		}, nil
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Try to parse as new format first
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Check if this is an old format config (has top-level "repos" field instead of "lab")
	// by attempting to parse as LabConfig
	var legacyCheck struct {
		Repos *LabReposConfig `yaml:"repos,omitempty"`
		Lab   *LabConfig      `yaml:"lab,omitempty"`
	}
	if err := yaml.Unmarshal(data, &legacyCheck); err == nil {
		if legacyCheck.Repos != nil && legacyCheck.Lab == nil {
			// Old format detected - migrate it
			var labCfg LabConfig
			if err := yaml.Unmarshal(data, &labCfg); err != nil {
				return nil, fmt.Errorf("failed to parse legacy config: %w", err)
			}

			cfg.Lab = &labCfg
		}
	}

	// Apply defaults to loaded config
	cfg.setDefaults()

	return &LoadResult{
		Config:     &cfg,
		ConfigPath: absPath,
	}, nil
}

// LoadLabConfig loads and validates lab configuration from the config file.
// This is a convenience function that combines Load with lab-specific validation.
// Returns the lab config and the config file path, or an error if loading fails
// or if the lab configuration is not found.
func LoadLabConfig(configPath string) (*LabConfig, string, error) {
	result, err := Load(configPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load config: %w", err)
	}

	if result.Config.Lab == nil {
		return nil, "", fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
	}

	return result.Config.Lab, result.ConfigPath, nil
}

// FindConfig searches for a config file using multiple strategies:
// 1. Search upward from current directory to filesystem root
// 2. Search immediate child directories
// 3. Check global config for registered project paths
// Returns the absolute path to the config file, or the provided fallback path if not found.
func FindConfig(fallback string) string {
	const configFileName = ".xcli.yaml"

	// Start from current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fallback
	}

	// Strategy 1: Search upward
	if path := searchUpward(cwd, configFileName); path != "" {
		return path
	}

	// Strategy 2: Search immediate child directories
	if path := searchChildren(cwd, configFileName); path != "" {
		return path
	}

	// Strategy 3: Check global config
	if path := searchGlobalConfig(); path != "" {
		return path
	}

	return fallback
}

// setDefaults applies default values to unset fields.
func (c *Config) setDefaults() {
	if c.Lab == nil {
		c.Lab = DefaultLab()

		return
	}

	// Infrastructure defaults
	if c.Lab.Infrastructure.ClickHouseXatuPort == 0 {
		c.Lab.Infrastructure.ClickHouseXatuPort = 8123
	}

	if c.Lab.Infrastructure.ClickHouseCBTPort == 0 {
		c.Lab.Infrastructure.ClickHouseCBTPort = 8124
	}

	if c.Lab.Infrastructure.RedisPort == 0 {
		c.Lab.Infrastructure.RedisPort = 6380
	}

	// TUI defaults
	if c.Lab.TUI.MaxLogLines == 0 {
		c.Lab.TUI.MaxLogLines = 1_000_000
	}

	// Observability defaults
	if c.Lab.Infrastructure.Observability.PrometheusPort == 0 {
		c.Lab.Infrastructure.Observability.PrometheusPort = constants.DefaultPrometheusPort
	}

	if c.Lab.Infrastructure.Observability.GrafanaPort == 0 {
		c.Lab.Infrastructure.Observability.GrafanaPort = constants.DefaultGrafanaPort
	}
}

// searchUpward walks up the directory tree looking for the config file.
func searchUpward(startDir, configFileName string) string {
	currentDir := startDir

	for {
		configPath := filepath.Join(currentDir, configFileName)

		// Check if config exists in current directory
		if _, err := os.Stat(configPath); err == nil {
			// Found it - return absolute path
			absPath, err := filepath.Abs(configPath)
			if err != nil {
				return configPath // Return non-absolute if Abs fails
			}

			return absPath
		}

		// Move to parent directory
		parentDir := filepath.Dir(currentDir)

		// Check if we've reached the root
		if parentDir == currentDir {
			return "" // Not found
		}

		currentDir = parentDir
	}
}

// searchChildren looks for config file in immediate child directories.
func searchChildren(parentDir, configFileName string) string {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		configPath := filepath.Join(parentDir, entry.Name(), configFileName)
		if _, err := os.Stat(configPath); err == nil {
			// Found it - return absolute path
			absPath, err := filepath.Abs(configPath)
			if err != nil {
				return configPath
			}

			return absPath
		}
	}

	return ""
}

// searchGlobalConfig checks the global config file for the xcli installation path.
func searchGlobalConfig() string {
	globalCfg, err := LoadGlobalConfig()
	if err != nil || globalCfg.XCLIPath == "" {
		return ""
	}

	configPath := filepath.Join(globalCfg.XCLIPath, ".xcli.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return configPath
	}

	return ""
}
