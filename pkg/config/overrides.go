package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ethpandaops/xcli/pkg/constants"
	"gopkg.in/yaml.v3"
)

// CBTOverridesConfig represents CBT model overrides configuration.
// This file allows configuring CBT model behavior without modifying source models.
type CBTOverridesConfig struct {
	// Global default limits applied to all incremental models (unless overridden per-model)
	DefaultLimits *ModelLimits `yaml:"defaultLimits,omitempty"`

	// Per-model overrides
	Models map[string]ModelOverride `yaml:"models,omitempty"`
}

// ModelOverride represents overrides for a specific model.
type ModelOverride struct {
	// Disable this model entirely
	Enabled *bool `yaml:"enabled,omitempty"`

	// Override model configuration
	Config *ModelConfig `yaml:"config,omitempty"`
}

// ModelConfig represents model configuration overrides.
type ModelConfig struct {
	// Position limits for backfill
	Limits *ModelLimits `yaml:"limits,omitempty"`

	// Schedule overrides
	Schedules *ScheduleConfig `yaml:"schedules,omitempty"`

	// Lag for external models (slots to look back when querying)
	Lag *int `yaml:"lag,omitempty"`

	// Tags override
	Tags []string `yaml:"tags,omitempty"`
}

// ModelLimits defines position boundaries for processing.
type ModelLimits struct {
	// Minimum position to process (e.g., slot number)
	Min uint64 `yaml:"min,omitempty"`

	// Maximum position to process (0 = no limit)
	Max uint64 `yaml:"max,omitempty"`
}

// ScheduleConfig defines schedule overrides.
type ScheduleConfig struct {
	// Schedule for scheduled-type models (cron format)
	Schedule string `yaml:"schedule,omitempty"`

	// Forward fill schedule (cron format) - for incremental models
	ForwardFill string `yaml:"forwardfill,omitempty"`

	// Backfill schedule (cron format) - for incremental models
	Backfill string `yaml:"backfill,omitempty"`
}

// DefaultCBTOverrides returns an empty overrides configuration.
func DefaultCBTOverrides() *CBTOverridesConfig {
	return &CBTOverridesConfig{
		Models: make(map[string]ModelOverride),
	}
}

// LoadCBTOverrides reads and parses a CBT overrides file.
func LoadCBTOverrides(path string) (*CBTOverridesConfig, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultCBTOverrides(), nil
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read overrides file: %w", err)
	}

	// Parse YAML
	var cfg CBTOverridesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse overrides file: %w", err)
	}

	// Initialize models map if nil
	if cfg.Models == nil {
		cfg.Models = make(map[string]ModelOverride)
	}

	return &cfg, nil
}

// Save writes the overrides configuration to a file.
func (c *CBTOverridesConfig) Save(path string) error {
	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal overrides: %w", err)
	}

	// Write file
	//nolint:gosec // Overrides file permissions are intentionally 0644 for readability
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write overrides file: %w", err)
	}

	return nil
}

// ToCBTOverrides converts overrides to CBT's overrides format.
// CBT expects format: { overrides: { model_name: { enabled: bool, config: {...} } } }.
func (c *CBTOverridesConfig) ToCBTOverrides() map[string]interface{} {
	result := make(map[string]interface{})

	// Add per-model overrides
	if len(c.Models) > 0 {
		overrides := make(map[string]interface{}, len(c.Models))
		for modelName, override := range c.Models {
			modelOverride := make(map[string]interface{})

			// Add enabled flag if specified
			if override.Enabled != nil {
				modelOverride["enabled"] = *override.Enabled
			}

			// Add config overrides
			if override.Config != nil {
				config := make(map[string]interface{})

				// Add limits if specified (or use defaults)
				limits := override.Config.Limits
				if limits == nil && c.DefaultLimits != nil {
					limits = c.DefaultLimits
				}

				if limits != nil {
					limitsMap := make(map[string]interface{})

					if limits.Min > 0 {
						limitsMap["min"] = limits.Min
					}

					if limits.Max > 0 {
						limitsMap["max"] = limits.Max
					}

					if len(limitsMap) > 0 {
						config["limits"] = limitsMap
					}
				}

				// Add schedules if specified
				if override.Config.Schedules != nil {
					// Scheduled models use a single "schedule" field
					if override.Config.Schedules.Schedule != "" {
						config["schedule"] = override.Config.Schedules.Schedule
					}

					// Incremental models use separate forwardfill/backfill schedules
					if override.Config.Schedules.ForwardFill != "" || override.Config.Schedules.Backfill != "" {
						schedules := make(map[string]interface{})
						if override.Config.Schedules.ForwardFill != "" {
							schedules["forwardfill"] = override.Config.Schedules.ForwardFill
						}

						if override.Config.Schedules.Backfill != "" {
							schedules["backfill"] = override.Config.Schedules.Backfill
						}

						if len(schedules) > 0 {
							config["schedules"] = schedules
						}
					}
				}

				// Add lag if specified
				if override.Config.Lag != nil {
					config["lag"] = *override.Config.Lag
				}

				// Add tags if specified
				if len(override.Config.Tags) > 0 {
					config["tags"] = override.Config.Tags
				}

				if len(config) > 0 {
					modelOverride["config"] = config
				}
			}

			if len(modelOverride) > 0 {
				overrides[modelName] = modelOverride
			}
		}

		if len(overrides) > 0 {
			result["overrides"] = overrides
		}
	}

	return result
}

// ApplyDefaultLimitsToAllModels creates overrides for models that don't have explicit overrides.
// This applies defaultLimits to all models in the modelNames list that aren't already overridden.
func (c *CBTOverridesConfig) ApplyDefaultLimitsToAllModels(modelNames []string) {
	if c.DefaultLimits == nil {
		return
	}

	for _, modelName := range modelNames {
		// Skip if model already has an override with explicit limits
		if existing, exists := c.Models[modelName]; exists {
			if existing.Config != nil && existing.Config.Limits != nil {
				continue
			}
		}

		// Apply default limits
		if _, exists := c.Models[modelName]; !exists {
			c.Models[modelName] = ModelOverride{
				Config: &ModelConfig{
					Limits: c.DefaultLimits,
				},
			}
		} else {
			// Merge with existing override
			existing := c.Models[modelName]
			if existing.Config == nil {
				existing.Config = &ModelConfig{}
			}

			if existing.Config.Limits == nil {
				existing.Config.Limits = c.DefaultLimits
			}

			c.Models[modelName] = existing
		}
	}
}

// ParseBackfillDuration parses a duration string like "30m", "1h", "2w", "4w", "1mo", "90d"
// Returns duration in seconds. Supported units: m (minutes), h (hours), d (days), w (weeks), mo (months ~30d)
// Defaults to 2 weeks if parsing fails.
func ParseBackfillDuration(durationStr string) uint64 {
	const (
		minuteSeconds = 60
		hourSeconds   = 60 * minuteSeconds
		daySeconds    = 24 * hourSeconds
		weekSeconds   = 7 * daySeconds
		monthSeconds  = 30 * daySeconds // Approximate month
	)

	// Default to 2 weeks
	if durationStr == "" {
		return 2 * weekSeconds
	}

	// Match number + unit: "30m", "1h", "2w", "4d", "1mo"
	re := regexp.MustCompile(`^(\d+)(m|h|d|w|mo|min|mins?|minutes?|hours?|days?|weeks?|months?)$`)
	matches := re.FindStringSubmatch(strings.ToLower(strings.TrimSpace(durationStr)))

	if matches == nil {
		return 2 * weekSeconds // default on parse error
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 2 * weekSeconds
	}

	unit := matches[2]

	switch {
	case strings.HasPrefix(unit, "mo"): // Check 'mo' before 'm' to avoid conflict
		//nolint:gosec // value parsed from user input, overflow unlikely for reasonable durations
		return uint64(value * monthSeconds)
	case strings.HasPrefix(unit, "m"):
		//nolint:gosec // value parsed from user input, overflow unlikely for reasonable durations
		return uint64(value * minuteSeconds)
	case strings.HasPrefix(unit, "h"):
		//nolint:gosec // value parsed from user input, overflow unlikely for reasonable durations
		return uint64(value * hourSeconds)
	case strings.HasPrefix(unit, "d"):
		//nolint:gosec // value parsed from user input, overflow unlikely for reasonable durations
		return uint64(value * daySeconds)
	case strings.HasPrefix(unit, "w"):
		//nolint:gosec // value parsed from user input, overflow unlikely for reasonable durations
		return uint64(value * weekSeconds)
	default:
		return 2 * weekSeconds
	}
}

// CalculateBackfillPosition calculates position for a specific duration ago for a network.
// Returns both slot-based and timestamp-based values.
// genesisTimestamp: optional network-specific genesis (from .xcli.yaml), 0 = use built-in map.
func CalculateBackfillPosition(network string, backfillDurationSeconds uint64, genesisTimestamp uint64) (slotPosition, timestampPosition uint64) {
	const slotDuration = 12 // seconds per slot

	//nolint:gosec // time.Now().Unix() is always positive, conversion is safe
	now := uint64(time.Now().Unix())
	timestampPosition = now - backfillDurationSeconds

	// Use provided genesis, or fall back to built-in map
	if genesisTimestamp == 0 {
		if timestamp, ok := constants.NetworkGenesisTimestamps[network]; ok {
			genesisTimestamp = timestamp
		}
	}

	// Calculate slot position if we have genesis timestamp
	if genesisTimestamp > 0 && now > genesisTimestamp {
		currentSlot := (now - genesisTimestamp) / slotDuration

		backfillSlots := backfillDurationSeconds / slotDuration
		if currentSlot > backfillSlots {
			slotPosition = currentSlot - backfillSlots
		}
		// else slotPosition = 0 (less than configured duration of chain history)
	}

	return slotPosition, timestampPosition
}

// CalculateTwoWeeksAgoPosition calculates position for "2 weeks ago" for a network.
// This is a convenience wrapper around CalculateBackfillPosition.
// Returns both slot-based and timestamp-based values.
func CalculateTwoWeeksAgoPosition(network string, genesisTimestamp uint64) (slotPosition, timestampPosition uint64) {
	const twoWeeksSeconds = 14 * 24 * 60 * 60 // 1,209,600 seconds

	return CalculateBackfillPosition(network, twoWeeksSeconds, genesisTimestamp)
}

// GenerateDefaultOverrides creates default overrides with configurable backfill limit.
// durationStr: e.g. "2w", "4w", "1mo", "90d"
// genesisTimestamp: optional network genesis (0 = use built-in map).
func GenerateDefaultOverrides(network string, durationStr string, genesisTimestamp uint64) *CBTOverridesConfig {
	backfillDuration := ParseBackfillDuration(durationStr)
	slotPos, timestampPos := CalculateBackfillPosition(network, backfillDuration, genesisTimestamp)

	// Use slot position as primary (most common), fall back to timestamp if no genesis
	defaultMin := slotPos
	if defaultMin == 0 {
		defaultMin = timestampPos
	}

	return &CBTOverridesConfig{
		DefaultLimits: &ModelLimits{
			Min: defaultMin,
			Max: 0, // No upper limit
		},
		Models: make(map[string]ModelOverride),
	}
}

// ApplyScheduleOverrides applies schedule overrides to specified models.
// This sets the schedule for all models in the list that don't already have a schedule override.
func (c *CBTOverridesConfig) ApplyScheduleOverrides(modelNames []string, schedule string) {
	if schedule == "" {
		return
	}

	for _, modelName := range modelNames {
		// Check if model already has an override with explicit schedule
		if existing, exists := c.Models[modelName]; exists {
			if existing.Config != nil && existing.Config.Schedules != nil {
				continue // Skip - user has explicit schedule override
			}
		}

		// Get or create model override
		modelOverride, exists := c.Models[modelName]
		if !exists {
			modelOverride = ModelOverride{
				Config: &ModelConfig{},
			}
		} else if modelOverride.Config == nil {
			modelOverride.Config = &ModelConfig{}
		}

		// Set schedule for scheduled models (single schedule, not forwardfill/backfill)
		if modelOverride.Config.Schedules == nil {
			modelOverride.Config.Schedules = &ScheduleConfig{}
		}

		modelOverride.Config.Schedules.Schedule = schedule

		c.Models[modelName] = modelOverride
	}
}

// ApplyLagOverrides applies lag settings to external models.
// This prevents full table scans on large external tables by limiting lookback.
func (c *CBTOverridesConfig) ApplyLagOverrides(models map[string]int) {
	if len(models) == 0 {
		return
	}

	for modelName, lagValue := range models {
		// Check if model already has an override with explicit lag
		if existing, exists := c.Models[modelName]; exists {
			if existing.Config != nil && existing.Config.Lag != nil {
				continue // Skip - user has explicit lag override
			}
		}

		// Get or create model override
		modelOverride, exists := c.Models[modelName]
		if !exists {
			modelOverride = ModelOverride{
				Config: &ModelConfig{},
			}
		} else if modelOverride.Config == nil {
			modelOverride.Config = &ModelConfig{}
		}

		// Set lag
		modelOverride.Config.Lag = &lagValue

		c.Models[modelName] = modelOverride
	}
}

// MergeOverrides merges user overrides on top of generated defaults.
// User overrides take precedence.
func MergeOverrides(defaults, user *CBTOverridesConfig) *CBTOverridesConfig {
	if user == nil {
		return defaults
	}

	merged := &CBTOverridesConfig{
		DefaultLimits: defaults.DefaultLimits,
		Models:        make(map[string]ModelOverride),
	}

	// User's default limits override generated defaults
	if user.DefaultLimits != nil {
		merged.DefaultLimits = user.DefaultLimits
	}

	// Start with default models
	for name, override := range defaults.Models {
		merged.Models[name] = override
	}

	// Apply user model overrides (takes precedence)
	for name, override := range user.Models {
		merged.Models[name] = override
	}

	return merged
}
