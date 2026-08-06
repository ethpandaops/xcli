// Package config handles loading, validation, and management of xcli configuration
// including lab stack settings, network configurations, and infrastructure options.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ethpandaops/xcli/pkg/constants"
)

// DefaultConfigFileName is the workspace-local xcli configuration file.
const DefaultConfigFileName = ".xcli.yaml"

var runtimeConfigPath string

// SetRuntimeConfigPath records the config path selected by CLI flag parsing.
//
// Most command constructors receive their config path before Cobra parses
// persistent flags, so runtime loaders use this value when their local fallback
// is still the default path.
func SetRuntimeConfigPath(path string) {
	runtimeConfigPath = strings.TrimSpace(path)
}

// EffectiveConfigPath returns the config path that should be used by runtime
// loaders after global flag parsing has completed.
func EffectiveConfigPath(path string) string {
	if path == "" {
		path = DefaultConfigFileName
	}

	if (path == DefaultConfigFileName || path == "."+string(filepath.Separator)+DefaultConfigFileName) &&
		runtimeConfigPath != "" {
		return runtimeConfigPath
	}

	return path
}

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
	Instance       LabInstanceConfig    `yaml:"instance,omitempty"`
	Mode           string               `yaml:"mode"`
	Networks       []NetworkConfig      `yaml:"networks"`
	Infrastructure InfrastructureConfig `yaml:"infrastructure"`
	Ports          LabPortsConfig       `yaml:"ports"`
	Dev            LabDevConfig         `yaml:"dev"`
	TUI            TUIConfig            `yaml:"tui"`
	CBT            LabCBTConfig         `yaml:"cbt,omitempty"`
}

// LabCBTConfig contains CBT engine defaults applied to generated configs.
type LabCBTConfig struct {
	// DefaultBackfillDuration bounds how far back external models scan
	// (EXTERNAL_MODEL_MIN_TIMESTAMP = now - duration). Defaults to 1h.
	// Canonical (cannon-derived) tables lag hours behind head, so short
	// windows can leave canonical-sourced models with no data in range.
	DefaultBackfillDuration string `yaml:"defaultBackfillDuration,omitempty"`
}

// LabInstanceConfig contains per-instance identity settings.
type LabInstanceConfig struct {
	ID string `yaml:"id,omitempty"`
}

// LabReposConfig contains paths to lab repositories.
type LabReposConfig struct {
	CBT        string `yaml:"cbt"`
	XatuCBT    string `yaml:"xatuCbt"`
	CBTAPI     string `yaml:"cbtApi"`
	LabBackend string `yaml:"labBackend"`
	Lab        string `yaml:"lab"`
}

// LabRepoPath is one named lab repository path.
type LabRepoPath struct {
	Name string
	Path string
}

// Map returns the configured lab repositories keyed by canonical repository name.
func (r LabReposConfig) Map() map[string]string {
	return map[string]string{
		constants.RepoCBT:        r.CBT,
		constants.RepoXatuCBT:    r.XatuCBT,
		constants.RepoCBTAPI:     r.CBTAPI,
		constants.RepoLabBackend: r.LabBackend,
		constants.RepoLab:        r.Lab,
	}
}

// Ordered returns lab repositories in the standard display/init order.
func (r LabReposConfig) Ordered() []LabRepoPath {
	return []LabRepoPath{
		{Name: constants.RepoCBT, Path: r.CBT},
		{Name: constants.RepoXatuCBT, Path: r.XatuCBT},
		{Name: constants.RepoCBTAPI, Path: r.CBTAPI},
		{Name: constants.RepoLabBackend, Path: r.LabBackend},
		{Name: constants.RepoLab, Path: r.Lab},
	}
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

// ExternalURLWithCredentials returns ExternalURL with ExternalUsername and
// ExternalPassword injected into the URL's userinfo. Consumers such as
// xatu-cbt read credentials only from the URL, so the separately-configured
// fields must be embedded before the URL is handed off. Credentials already
// present in ExternalURL are preserved unless overridden by the explicit fields.
func (c ClickHouseClusterConfig) ExternalURLWithCredentials() (string, error) {
	if c.ExternalURL == "" {
		return "", nil
	}

	if c.ExternalUsername == "" && c.ExternalPassword == "" {
		return c.ExternalURL, nil
	}

	parsed, err := url.Parse(c.ExternalURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse external ClickHouse URL: %w", err)
	}

	// Seed from any credentials already embedded in the URL, then let the
	// explicit fields take precedence.
	username := ""
	password := ""

	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}

	if c.ExternalUsername != "" {
		username = c.ExternalUsername
	}

	if c.ExternalPassword != "" {
		password = c.ExternalPassword
	}

	// A password with no username is unusable by ClickHouse (it would produce a
	// ":password@host" userinfo). Fall back to the "default" user, which is what
	// a password-only configuration implies.
	if password != "" && username == "" {
		username = "default"
	}

	switch {
	case password != "":
		parsed.User = url.UserPassword(username, password)
	case username != "":
		parsed.User = url.User(username)
	}

	return parsed.String(), nil
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
	for name, path := range c.Repos.Map() {
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
					ExternalURL:      "http://chendpoint-clickhouse-raw.analytics.production.ethpandaops:9000",
					ExternalDatabase: "default",
				},
				CBT: ClickHouseClusterConfig{Mode: constants.InfraModeLocal},
			},
			Redis:   RedisConfig{Port: constants.DefaultRedisPort},
			Volumes: VolumesConfig{Persist: true},
			Observability: ObservabilityConfig{
				Enabled:        false,
				PrometheusPort: constants.DefaultPrometheusPort,
				GrafanaPort:    constants.DefaultGrafanaPort,
			},
			ClickHouseXatuPort: constants.DefaultClickHouseXatuHTTPPort,
			ClickHouseCBTPort:  constants.DefaultClickHouseCBTHTTPPort,
			RedisPort:          constants.DefaultRedisPort,
		},
		Ports: LabPortsConfig{
			LabBackend:      constants.DefaultLabBackendPort,
			LabFrontend:     constants.DefaultLabFrontendPort,
			CBTBase:         constants.DefaultCBTBasePort,
			CBTAPIBase:      constants.DefaultCBTAPIBasePort,
			CBTFrontendBase: constants.DefaultCBTFrontendBasePort,
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

// Load reads and parses a config file.
// If the path is ".xcli.yaml" (the default), it will search upward through parent directories.
// Returns the config and the resolved absolute path to the config file.
func Load(path string) (*LoadResult, error) {
	path = EffectiveConfigPath(path)

	// If using default path, search for it in parent directories
	if path == DefaultConfigFileName {
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

	// Apply defaults to loaded config
	cfg.setDefaults()

	return &LoadResult{
		Config:     &cfg,
		ConfigPath: absPath,
	}, nil
}

// FindConfig searches upward from current directory to filesystem root.
// Returns the absolute path to the config file, or the provided fallback path if not found.
func FindConfig(fallback string) string {
	// Start from current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fallback
	}

	// Search upward only. Child-directory and global fallbacks can silently
	// select another workspace when multiple xcli instances exist.
	if path := searchUpward(cwd, DefaultConfigFileName); path != "" {
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
		c.Lab.Infrastructure.ClickHouseXatuPort = constants.DefaultClickHouseXatuHTTPPort
	}

	if c.Lab.Infrastructure.ClickHouseCBTPort == 0 {
		c.Lab.Infrastructure.ClickHouseCBTPort = constants.DefaultClickHouseCBTHTTPPort
	}

	if c.Lab.Infrastructure.RedisPort == 0 {
		c.Lab.Infrastructure.RedisPort = constants.DefaultRedisPort
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
