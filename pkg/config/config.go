package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the xcli root configuration
// It contains stack-specific configurations and optional workspace-level settings
type Config struct {
	// Lab stack configuration
	Lab *LabConfig `yaml:"lab,omitempty"`

	// Future stacks can be added here:
	// Contributoor *ContributoorConfig `yaml:"contributoor,omitempty"`
	// Xatu *XatuConfig `yaml:"xatu,omitempty"`

	// Workspace-level settings (optional, for future use)
	// Workspace *WorkspaceConfig `yaml:"workspace,omitempty"`
}

// LabConfig represents the lab stack configuration
type LabConfig struct {
	Repos          LabReposConfig       `yaml:"repos"`
	Mode           string               `yaml:"mode"`
	Networks       []NetworkConfig      `yaml:"networks"`
	Infrastructure InfrastructureConfig `yaml:"infrastructure"`
	Ports          LabPortsConfig       `yaml:"ports"`
	Dev            LabDevConfig         `yaml:"dev"`
}

// LabReposConfig contains paths to lab repositories
type LabReposConfig struct {
	CBT        string `yaml:"cbt"`
	XatuCBT    string `yaml:"xatu_cbt"`
	CBTAPI     string `yaml:"cbt_api"`
	LabBackend string `yaml:"lab_backend"`
	Lab        string `yaml:"lab"`
}

// NetworkConfig represents a network configuration
type NetworkConfig struct {
	Name       string `yaml:"name"`
	Enabled    bool   `yaml:"enabled"`
	PortOffset int    `yaml:"port_offset"`
}

// InfrastructureConfig contains infrastructure settings
type InfrastructureConfig struct {
	ClickHouse ClickHouseConfig `yaml:"clickhouse"`
	Redis      RedisConfig      `yaml:"redis"`
	Volumes    VolumesConfig    `yaml:"volumes"`
}

// ClickHouseConfig contains ClickHouse cluster configuration
type ClickHouseConfig struct {
	Xatu ClickHouseClusterConfig `yaml:"xatu"`
	CBT  ClickHouseClusterConfig `yaml:"cbt"`
}

// ClickHouseClusterConfig represents a ClickHouse cluster
type ClickHouseClusterConfig struct {
	Mode             string `yaml:"mode"` // "local" or "external"
	ExternalURL      string `yaml:"external_url,omitempty"`
	ExternalDatabase string `yaml:"external_database,omitempty"`
	ExternalUsername string `yaml:"external_username,omitempty"`
	ExternalPassword string `yaml:"external_password,omitempty"`
}

// RedisConfig contains Redis configuration
type RedisConfig struct {
	Port int `yaml:"port"`
}

// VolumesConfig contains volume settings
type VolumesConfig struct {
	Persist bool `yaml:"persist"`
}

// LabPortsConfig contains lab stack port assignments
type LabPortsConfig struct {
	LabBackend  int `yaml:"lab_backend"`
	LabFrontend int `yaml:"lab_frontend"`
	CBTBase     int `yaml:"cbt_base"`
	CBTAPIBase  int `yaml:"cbt_api_base"`
}

// LabDevConfig contains lab stack development features
type LabDevConfig struct {
	LabRebuildOnChange bool `yaml:"lab_rebuild_on_change"`
	HotReload          bool `yaml:"hot_reload"`
}

// Default returns a root configuration with sensible defaults
func Default() *Config {
	return &Config{
		Lab: DefaultLab(),
	}
}

// DefaultLab returns a lab configuration with sensible defaults
func DefaultLab() *LabConfig {
	return &LabConfig{
		Repos: LabReposConfig{
			CBT:        "../cbt",
			XatuCBT:    "../xatu-cbt",
			CBTAPI:     "../cbt-api",
			LabBackend: "../lab-backend",
			Lab:        "../lab",
		},
		Mode: "local",
		Networks: []NetworkConfig{
			{Name: "mainnet", Enabled: true, PortOffset: 0},
			{Name: "sepolia", Enabled: true, PortOffset: 1},
		},
		Infrastructure: InfrastructureConfig{
			ClickHouse: ClickHouseConfig{
				Xatu: ClickHouseClusterConfig{Mode: "local"},
				CBT:  ClickHouseClusterConfig{Mode: "local"},
			},
			Redis:   RedisConfig{Port: 6380},
			Volumes: VolumesConfig{Persist: true},
		},
		Ports: LabPortsConfig{
			LabBackend:  8080,
			LabFrontend: 5173,
			CBTBase:     8081,
			CBTAPIBase:  8091,
		},
		Dev: LabDevConfig{
			LabRebuildOnChange: false,
			HotReload:          true,
		},
	}
}

// Load reads and parses a config file
// Supports both old (flat) and new (namespaced) config formats for backward compatibility
func Load(path string) (*Config, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return Default(), nil
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

	// Apply defaults for missing sections
	if cfg.Lab == nil {
		cfg.Lab = DefaultLab()
	}

	return &cfg, nil
}

// Save writes the configuration to a file
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
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate checks if the root configuration is valid
func (c *Config) Validate() error {
	// Validate lab config if present
	if c.Lab != nil {
		if err := c.Lab.Validate(); err != nil {
			return fmt.Errorf("lab config validation failed: %w", err)
		}
	}

	// Future: Validate other stacks when added
	// if c.Contributoor != nil { ... }
	// if c.Xatu != nil { ... }

	return nil
}

// Validate checks if the lab configuration is valid
func (c *LabConfig) Validate() error {
	// Check mode
	if c.Mode != "local" && c.Mode != "hybrid" {
		return fmt.Errorf("invalid mode: %s (must be 'local' or 'hybrid')", c.Mode)
	}

	// Check repo paths exist
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
	if c.Mode == "hybrid" && c.Infrastructure.ClickHouse.Xatu.Mode == "external" {
		if c.Infrastructure.ClickHouse.Xatu.ExternalURL == "" {
			return fmt.Errorf("external_url is required for hybrid mode with external Xatu cluster\n" +
				"Add to .xcli.yaml:\n" +
				"  lab:\n" +
				"    infrastructure:\n" +
				"      clickhouse:\n" +
				"        xatu:\n" +
				"          external_url: \"https://username:password@prod-xatu.example.com:8443\"")
		}

		// Validate URL format
		if !strings.HasPrefix(c.Infrastructure.ClickHouse.Xatu.ExternalURL, "http://") &&
			!strings.HasPrefix(c.Infrastructure.ClickHouse.Xatu.ExternalURL, "https://") &&
			!strings.HasPrefix(c.Infrastructure.ClickHouse.Xatu.ExternalURL, "clickhouse://") {
			return fmt.Errorf("external_url must start with http://, https://, or clickhouse://")
		}

		// Warn if using http in production
		if strings.HasPrefix(c.Infrastructure.ClickHouse.Xatu.ExternalURL, "http://") {
			fmt.Fprintf(os.Stderr, "WARNING: Using unencrypted HTTP connection to external Xatu cluster\n")
		}
	}

	return nil
}

// EnabledNetworks returns a slice of enabled networks
func (c *LabConfig) EnabledNetworks() []NetworkConfig {
	enabled := make([]NetworkConfig, 0, len(c.Networks))
	for _, net := range c.Networks {
		if net.Enabled {
			enabled = append(enabled, net)
		}
	}
	return enabled
}

// GetCBTPort returns the CBT port for a given network
func (c *LabConfig) GetCBTPort(network string) int {
	for _, net := range c.Networks {
		if net.Name == network && net.Enabled {
			return c.Ports.CBTBase + net.PortOffset
		}
	}
	return 0
}

// GetCBTAPIPort returns the cbt-api port for a given network
func (c *LabConfig) GetCBTAPIPort(network string) int {
	for _, net := range c.Networks {
		if net.Name == network && net.Enabled {
			return c.Ports.CBTAPIBase + net.PortOffset
		}
	}
	return 0
}
