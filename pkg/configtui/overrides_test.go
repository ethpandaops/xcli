package configtui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool {
	return &b
}

func TestIsModelDisabled(t *testing.T) {
	tests := []struct {
		name      string
		overrides *CBTOverrides
		modelName string
		expected  bool
	}{
		{
			name:      "nil overrides returns false",
			overrides: nil,
			modelName: "fct_block",
			expected:  false,
		},
		{
			name: "nil overrides map returns false",
			overrides: &CBTOverrides{
				Models: ModelsConfig{},
			},
			modelName: "fct_block",
			expected:  false,
		},
		{
			name: "model not in overrides returns false (denylist)",
			overrides: &CBTOverrides{
				Models: ModelsConfig{
					Overrides: map[string]ModelOverride{},
				},
			},
			modelName: "fct_block",
			expected:  false,
		},
		{
			name: "model explicitly disabled returns true",
			overrides: &CBTOverrides{
				Models: ModelsConfig{
					Overrides: map[string]ModelOverride{
						"fct_block": {Enabled: boolPtr(false)},
					},
				},
			},
			modelName: "fct_block",
			expected:  true,
		},
		{
			name: "model explicitly enabled returns false",
			overrides: &CBTOverrides{
				Models: ModelsConfig{
					Overrides: map[string]ModelOverride{
						"fct_block": {Enabled: boolPtr(true)},
					},
				},
			},
			modelName: "fct_block",
			expected:  false,
		},
		{
			name: "model listed without enabled field returns false",
			overrides: &CBTOverrides{
				Models: ModelsConfig{
					Overrides: map[string]ModelOverride{
						"fct_block": {},
					},
				},
			},
			modelName: "fct_block",
			expected:  false,
		},
		{
			name: "allowlist mode: unlisted model is disabled",
			overrides: &CBTOverrides{
				Models: ModelsConfig{
					DefaultEnabled: boolPtr(false),
					Overrides:      map[string]ModelOverride{},
				},
			},
			modelName: "fct_block",
			expected:  true,
		},
		{
			name: "allowlist mode: listed model is enabled",
			overrides: &CBTOverrides{
				Models: ModelsConfig{
					DefaultEnabled: boolPtr(false),
					Overrides: map[string]ModelOverride{
						"fct_block": {},
					},
				},
			},
			modelName: "fct_block",
			expected:  false,
		},
		{
			name: "allowlist mode: listed model with enabled:false is disabled",
			overrides: &CBTOverrides{
				Models: ModelsConfig{
					DefaultEnabled: boolPtr(false),
					Overrides: map[string]ModelOverride{
						"fct_block": {Enabled: boolPtr(false)},
					},
				},
			},
			modelName: "fct_block",
			expected:  true,
		},
		{
			name: "allowlist mode: nil overrides map still disables unlisted",
			overrides: &CBTOverrides{
				Models: ModelsConfig{
					DefaultEnabled: boolPtr(false),
				},
			},
			modelName: "fct_block",
			expected:  true,
		},
		{
			name: "defaultEnabled true: same as no flag (denylist mode)",
			overrides: &CBTOverrides{
				Models: ModelsConfig{
					DefaultEnabled: boolPtr(true),
					Overrides:      map[string]ModelOverride{},
				},
			},
			modelName: "fct_block",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsModelDisabled(tt.overrides, tt.modelName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Run("file does not exist", func(t *testing.T) {
		overrides, exists, err := LoadOverrides("/nonexistent/path")
		require.NoError(t, err)
		assert.False(t, exists)
		assert.NotNil(t, overrides)
		assert.Nil(t, overrides.Models.DefaultEnabled)
	})

	t.Run("denylist mode file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, ".cbt-overrides.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`
models:
  overrides:
    fct_block:
      enabled: false
`), 0600))

		overrides, exists, err := LoadOverrides(path)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Nil(t, overrides.Models.DefaultEnabled)
		assert.True(t, IsModelDisabled(overrides, "fct_block"))
		assert.False(t, IsModelDisabled(overrides, "fct_other"))
	})

	t.Run("allowlist mode file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, ".cbt-overrides.yaml")
		require.NoError(t, os.WriteFile(path, []byte(`
models:
  defaultEnabled: false
  overrides:
    fct_block: {}
    fct_attestation:
      enabled: false
`), 0600))

		overrides, exists, err := LoadOverrides(path)
		require.NoError(t, err)
		assert.True(t, exists)
		require.NotNil(t, overrides.Models.DefaultEnabled)
		assert.False(t, *overrides.Models.DefaultEnabled)
		// fct_block is listed → enabled
		assert.False(t, IsModelDisabled(overrides, "fct_block"))
		// fct_attestation is listed with enabled:false → disabled
		assert.True(t, IsModelDisabled(overrides, "fct_attestation"))
		// fct_other is unlisted → disabled (allowlist mode)
		assert.True(t, IsModelDisabled(overrides, "fct_other"))
	})
}

func TestSaveOverridesFromEntries(t *testing.T) {
	t.Run("denylist mode writes disabled models", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, ".cbt-overrides.yaml")

		err := SaveOverridesFromEntries(
			path,
			[]ModelEntry{
				{Name: "a", OverrideKey: "a", Enabled: true},
				{Name: "b", OverrideKey: "b", Enabled: false},
			},
			[]ModelEntry{
				{Name: "c", OverrideKey: "c", Enabled: false},
			},
			"1234", true,
			"5678", true,
			nil,
			nil, // defaultEnabled nil = denylist mode
		)
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)

		content := string(data)

		// Should NOT have defaultEnabled
		assert.NotContains(t, content, "defaultEnabled")
		// Should have disabled models
		assert.Contains(t, content, "b:")
		assert.Contains(t, content, "c:")
		assert.Contains(t, content, "enabled: false")
		// Should NOT have enabled model "a" in overrides
		assert.NotContains(t, content, "    a:")
	})

	t.Run("allowlist mode writes defaultEnabled and enabled models config", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, ".cbt-overrides.yaml")

		existing := &CBTOverrides{
			Models: ModelsConfig{
				Overrides: map[string]ModelOverride{
					"a": {Config: map[string]any{"limits": map[string]any{"min": 100}}},
				},
			},
		}

		err := SaveOverridesFromEntries(
			path,
			[]ModelEntry{
				{Name: "a", OverrideKey: "a", Enabled: true},
				{Name: "b", OverrideKey: "b", Enabled: false},
			},
			nil,
			"", false,
			"", false,
			existing,
			boolPtr(false), // allowlist mode
		)
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)

		content := string(data)

		// Should have defaultEnabled: false
		assert.Contains(t, content, "defaultEnabled: false")
		// Should have "a" listed (enabled with config preserved)
		assert.Contains(t, content, "a:")
		// Should have "b" with enabled: false
		assert.Contains(t, content, "b:")
		assert.Contains(t, content, "enabled: false")
	})

	t.Run("round-trip preserves defaultEnabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, ".cbt-overrides.yaml")

		// Write allowlist mode
		err := SaveOverridesFromEntries(
			path,
			[]ModelEntry{{Name: "a", OverrideKey: "a", Enabled: true}},
			nil,
			"", false, "", false,
			nil,
			boolPtr(false),
		)
		require.NoError(t, err)

		// Load it back
		overrides, exists, err := LoadOverrides(path)
		require.NoError(t, err)
		assert.True(t, exists)
		require.NotNil(t, overrides.Models.DefaultEnabled)
		assert.False(t, *overrides.Models.DefaultEnabled)
	})

	t.Run("denylist round-trip has no defaultEnabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, ".cbt-overrides.yaml")

		err := SaveOverridesFromEntries(
			path,
			[]ModelEntry{{Name: "a", OverrideKey: "a", Enabled: false}},
			nil,
			"", false, "", false,
			nil,
			nil,
		)
		require.NoError(t, err)

		overrides, _, err := LoadOverrides(path)
		require.NoError(t, err)
		assert.Nil(t, overrides.Models.DefaultEnabled)
	})

	t.Run("empty overrides comment in allowlist mode", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, ".cbt-overrides.yaml")

		err := SaveOverridesFromEntries(
			path,
			nil, nil,
			"", false, "", false,
			nil,
			boolPtr(false),
		)
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)

		assert.Contains(t, string(data), "All models disabled by default")
	})
}

func TestSaveOverridesFromEntries_PreservesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".cbt-overrides.yaml")

	existing := &CBTOverrides{
		Models: ModelsConfig{
			Overrides: map[string]ModelOverride{
				"fct_block": {
					Enabled: boolPtr(false),
					Config:  map[string]any{"limits": map[string]any{"min": 42}},
				},
			},
		},
	}

	err := SaveOverridesFromEntries(
		path,
		[]ModelEntry{{Name: "fct_block", OverrideKey: "fct_block", Enabled: false}},
		nil,
		"", false, "", false,
		existing,
		nil,
	)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	content := string(data)

	assert.Contains(t, content, "enabled: false")
	// Config should be preserved
	assert.True(t, strings.Contains(content, "limits") && strings.Contains(content, "min"))
}
