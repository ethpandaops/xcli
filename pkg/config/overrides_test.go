package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBackfillDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uint64
	}{
		{
			name:     "empty string defaults to 2 weeks",
			input:    "",
			expected: 2 * 7 * 24 * 60 * 60,
		},
		{
			name:     "2 weeks",
			input:    "2w",
			expected: 2 * 7 * 24 * 60 * 60,
		},
		{
			name:     "4 weeks",
			input:    "4w",
			expected: 4 * 7 * 24 * 60 * 60,
		},
		{
			name:     "1 week singular",
			input:    "1week",
			expected: 1 * 7 * 24 * 60 * 60,
		},
		{
			name:     "3 weeks plural",
			input:    "3weeks",
			expected: 3 * 7 * 24 * 60 * 60,
		},
		{
			name:     "7 days",
			input:    "7d",
			expected: 7 * 24 * 60 * 60,
		},
		{
			name:     "90 days",
			input:    "90d",
			expected: 90 * 24 * 60 * 60,
		},
		{
			name:     "1 day singular",
			input:    "1day",
			expected: 1 * 24 * 60 * 60,
		},
		{
			name:     "30 days plural",
			input:    "30days",
			expected: 30 * 24 * 60 * 60,
		},
		{
			name:     "1 month",
			input:    "1mo",
			expected: 30 * 24 * 60 * 60,
		},
		{
			name:     "2 months",
			input:    "2months",
			expected: 2 * 30 * 24 * 60 * 60,
		},
		{
			name:     "uppercase 2W",
			input:    "2W",
			expected: 2 * 7 * 24 * 60 * 60,
		},
		{
			name:     "with whitespace",
			input:    "  3w  ",
			expected: 3 * 7 * 24 * 60 * 60,
		},
		{
			name:     "invalid format defaults to 2 weeks",
			input:    "invalid",
			expected: 2 * 7 * 24 * 60 * 60,
		},
		{
			name:     "number only defaults to 2 weeks",
			input:    "42",
			expected: 2 * 7 * 24 * 60 * 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseBackfillDuration(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateBackfillPosition(t *testing.T) {
	const slotDuration = 12 // seconds per slot

	// Mainnet genesis: Dec 1, 2020
	mainnetGenesis := uint64(1606824023)

	t.Run("mainnet with 2 weeks backfill", func(t *testing.T) {
		twoWeeksSeconds := uint64(14 * 24 * 60 * 60)
		slotPos, timestampPos := CalculateBackfillPosition("mainnet", twoWeeksSeconds, mainnetGenesis)

		// Verify timestamp position
		now := uint64(time.Now().Unix())
		expectedTimestamp := now - twoWeeksSeconds
		assert.InDelta(t, expectedTimestamp, timestampPos, 5) // Allow 5 second variance

		// Verify slot position is calculated
		assert.Greater(t, slotPos, uint64(0))

		// Verify slot position makes sense
		currentSlot := (now - mainnetGenesis) / slotDuration
		backfillSlots := twoWeeksSeconds / slotDuration
		expectedSlot := currentSlot - backfillSlots
		assert.InDelta(t, expectedSlot, slotPos, 10) // Allow small variance
	})

	t.Run("unknown network falls back to timestamp", func(t *testing.T) {
		fourWeeksSeconds := uint64(28 * 24 * 60 * 60)
		slotPos, timestampPos := CalculateBackfillPosition("unknown-network", fourWeeksSeconds, 0)

		// Should have timestamp position
		now := uint64(time.Now().Unix())
		expectedTimestamp := now - fourWeeksSeconds
		assert.InDelta(t, expectedTimestamp, timestampPos, 5)

		// Slot position should be 0 (no genesis known)
		assert.Equal(t, uint64(0), slotPos)
	})

	t.Run("custom network with provided genesis", func(t *testing.T) {
		customGenesis := uint64(1700000000) // Some custom genesis
		twoWeeksSeconds := uint64(14 * 24 * 60 * 60)

		slotPos, timestampPos := CalculateBackfillPosition("custom", twoWeeksSeconds, customGenesis)

		// Both should be calculated
		assert.Greater(t, slotPos, uint64(0))
		assert.Greater(t, timestampPos, uint64(0))

		// Verify calculations
		now := uint64(time.Now().Unix())
		assert.InDelta(t, now-twoWeeksSeconds, timestampPos, 5)

		currentSlot := (now - customGenesis) / slotDuration
		backfillSlots := twoWeeksSeconds / slotDuration
		expectedSlot := currentSlot - backfillSlots
		assert.InDelta(t, expectedSlot, slotPos, 10)
	})
}

func TestGenerateDefaultOverrides(t *testing.T) {
	t.Run("generates default limits for mainnet", func(t *testing.T) {
		overrides := GenerateDefaultOverrides("mainnet", "2w", 1606824023)

		require.NotNil(t, overrides)
		require.NotNil(t, overrides.DefaultLimits)
		assert.Greater(t, overrides.DefaultLimits.Min, uint64(0))
		assert.Equal(t, uint64(0), overrides.DefaultLimits.Max) // No upper limit
		assert.NotNil(t, overrides.Models)
	})

	t.Run("uses configured duration", func(t *testing.T) {
		overrides4w := GenerateDefaultOverrides("mainnet", "4w", 1606824023)
		overrides2w := GenerateDefaultOverrides("mainnet", "2w", 1606824023)

		// 4 week backfill should have lower min (further back in time)
		assert.Less(t, overrides4w.DefaultLimits.Min, overrides2w.DefaultLimits.Min)
	})

	t.Run("falls back to timestamp for unknown network", func(t *testing.T) {
		overrides := GenerateDefaultOverrides("unknown", "2w", 0)

		require.NotNil(t, overrides)
		require.NotNil(t, overrides.DefaultLimits)

		// Should use timestamp (Unix timestamp is much larger than slot numbers)
		// Mainnet is at ~10M slots, timestamps are 1.7B+
		assert.Greater(t, overrides.DefaultLimits.Min, uint64(100000000))
	})
}

func TestToCBTOverrides(t *testing.T) {
	t.Run("converts to CBT format with limits", func(t *testing.T) {
		enabled := true
		config := &CBTOverridesConfig{
			Models: map[string]ModelOverride{
				"test_model": {
					Enabled: &enabled,
					Config: &ModelConfig{
						Limits: &ModelLimits{
							Min: 1000,
							Max: 2000,
						},
					},
				},
			},
		}

		result := config.ToCBTOverrides()

		overrides, ok := result["overrides"].(map[string]interface{})
		require.True(t, ok)

		model, ok := overrides["test_model"].(map[string]interface{})
		require.True(t, ok)

		assert.Equal(t, true, model["enabled"])

		modelConfig, ok := model["config"].(map[string]interface{})
		require.True(t, ok)

		limits, ok := modelConfig["limits"].(map[string]interface{})
		require.True(t, ok)

		assert.Equal(t, uint64(1000), limits["min"])
		assert.Equal(t, uint64(2000), limits["max"])
	})

	t.Run("uses default limits when model has no explicit limits", func(t *testing.T) {
		enabled := true
		config := &CBTOverridesConfig{
			DefaultLimits: &ModelLimits{Min: 5000},
			Models: map[string]ModelOverride{
				"test_model": {
					Enabled: &enabled,
					Config:  &ModelConfig{}, // No explicit limits
				},
			},
		}

		result := config.ToCBTOverrides()

		overrides, ok := result["overrides"].(map[string]interface{})
		assert.True(t, ok)
		model, ok := overrides["test_model"].(map[string]interface{})
		assert.True(t, ok)
		modelConfig, ok := model["config"].(map[string]interface{})
		assert.True(t, ok)
		limits, ok := modelConfig["limits"].(map[string]interface{})
		assert.True(t, ok)

		assert.Equal(t, uint64(5000), limits["min"])
	})

	t.Run("includes schedules when specified", func(t *testing.T) {
		config := &CBTOverridesConfig{
			Models: map[string]ModelOverride{
				"test_model": {
					Config: &ModelConfig{
						Schedules: &ScheduleConfig{
							ForwardFill: "@every 1m",
							Backfill:    "@every 5m",
						},
					},
				},
			},
		}

		result := config.ToCBTOverrides()

		overrides, ok := result["overrides"].(map[string]interface{})
		assert.True(t, ok)
		model, ok := overrides["test_model"].(map[string]interface{})
		assert.True(t, ok)
		modelConfig, ok := model["config"].(map[string]interface{})
		assert.True(t, ok)
		schedules, ok := modelConfig["schedules"].(map[string]interface{})
		assert.True(t, ok)

		assert.Equal(t, "@every 1m", schedules["forwardfill"])
		assert.Equal(t, "@every 5m", schedules["backfill"])
	})
}
