package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the xcli configuration
type Config struct {
	Repos          ReposConfig          `yaml:"repos"`
	Mode           string               `yaml:"mode"`
	Networks       []NetworkConfig      `yaml:"networks"`
	Infrastructure InfrastructureConfig `yaml:"infrastructure"`
	Ports          PortsConfig          `yaml:"ports"`
	Dev            DevConfig            `yaml:"dev"`
}

// ReposConfig contains paths to all repositories
type ReposConfig struct {
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

// PortsConfig contains port assignments
type PortsConfig struct {
	LabBackend  int `yaml:"lab_backend"`
	LabFrontend int `yaml:"lab_frontend"`
	CBTBase     int `yaml:"cbt_base"`
	CBTAPIBase  int `yaml:"cbt_api_base"`
}

// DevConfig contains development features
type DevConfig struct {
	LabRebuildOnChange bool `yaml:"lab_rebuild_on_change"`
	HotReload          bool `yaml:"hot_reload"`
}

// Default returns a configuration with sensible defaults
func Default() *Config {
	return &Config{
		Repos: ReposConfig{
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
		Ports: PortsConfig{
			LabBackend:  8080,
			LabFrontend: 5173,
			CBTBase:     8081,
			CBTAPIBase:  8091,
		},
		Dev: DevConfig{
			LabRebuildOnChange: false,
			HotReload:          true,
		},
	}
}

// Load reads and parses a config file
func Load(path string) (*Config, error) {
	// Start with defaults
	cfg := Default()

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
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

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
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
				"  infrastructure:\n" +
				"    clickhouse:\n" +
				"      xatu:\n" +
				"        external_url: \"https://username:password@prod-xatu.example.com:8443\"")
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
func (c *Config) EnabledNetworks() []NetworkConfig {
	enabled := make([]NetworkConfig, 0, len(c.Networks))
	for _, net := range c.Networks {
		if net.Enabled {
			enabled = append(enabled, net)
		}
	}
	return enabled
}

// GetCBTPort returns the CBT port for a given network
func (c *Config) GetCBTPort(network string) int {
	for _, net := range c.Networks {
		if net.Name == network && net.Enabled {
			return c.Ports.CBTBase + net.PortOffset
		}
	}
	return 0
}

// GetCBTAPIPort returns the cbt-api port for a given network
func (c *Config) GetCBTAPIPort(network string) int {
	for _, net := range c.Networks {
		if net.Name == network && net.Enabled {
			return c.Ports.CBTAPIBase + net.PortOffset
		}
	}
	return 0
}
