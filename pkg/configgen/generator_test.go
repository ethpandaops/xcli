package configgen

import (
	"os"
	"path/filepath"
	"testing"

	"dario.cat/mergo"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupFakeXatuCBTRepo creates a fake xatu-cbt repo directory structure
// with the given external and transformation model names.
func setupFakeXatuCBTRepo(
	t *testing.T,
	external []string,
	transformations []string,
) string {
	t.Helper()

	repoDir := t.TempDir()

	externalDir := filepath.Join(repoDir, "models", "external")
	require.NoError(t, os.MkdirAll(externalDir, 0755))

	for _, name := range external {
		path := filepath.Join(externalDir, name+".sql")
		require.NoError(t, os.WriteFile(path, []byte("SELECT 1"), 0600))
	}

	transformDir := filepath.Join(repoDir, "models", "transformations")
	require.NoError(t, os.MkdirAll(transformDir, 0755))

	for _, name := range transformations {
		path := filepath.Join(transformDir, name+".sql")
		require.NoError(t, os.WriteFile(path, []byte("SELECT 1"), 0600))
	}

	return repoDir
}

func TestGetLocallyEnabledTables(t *testing.T) {
	allExternal := []string{
		"fct_block",
		"fct_block_head",
		"fct_attestation",
		"fct_proposer_slashing",
	}
	allTransformations := []string{"fct_block_summary"}

	tests := []struct {
		name            string
		overridesYAML   string
		noOverridesFile bool
		expectedTables  []string
		expectError     bool
	}{
		{
			name: "no disabled models returns all",
			overridesYAML: `
models:
  overrides: {}
`,
			expectedTables: []string{
				"fct_attestation",
				"fct_block",
				"fct_block_head",
				"fct_block_summary",
				"fct_proposer_slashing",
			},
		},
		{
			name: "disabled models are excluded",
			overridesYAML: `
models:
  overrides:
    fct_attestation:
      enabled: false
    fct_proposer_slashing:
      enabled: false
`,
			expectedTables: []string{
				"fct_block",
				"fct_block_head",
				"fct_block_summary",
			},
		},
		{
			name: "all disabled returns empty",
			overridesYAML: `
models:
  overrides:
    fct_block:
      enabled: false
    fct_block_head:
      enabled: false
    fct_attestation:
      enabled: false
    fct_proposer_slashing:
      enabled: false
    fct_block_summary:
      enabled: false
`,
			expectedTables: []string{},
		},
		{
			name:            "missing overrides file returns all models",
			noOverridesFile: true,
			expectedTables: []string{
				"fct_attestation",
				"fct_block",
				"fct_block_head",
				"fct_block_summary",
				"fct_proposer_slashing",
			},
		},
		{
			name: "allowlist mode returns only listed models",
			overridesYAML: `
models:
  defaultEnabled: false
  overrides:
    fct_block: {}
    fct_block_summary: {}
`,
			expectedTables: []string{
				"fct_block",
				"fct_block_summary",
			},
		},
		{
			name: "allowlist mode with all disabled returns empty",
			overridesYAML: `
models:
  defaultEnabled: false
  overrides: {}
`,
			expectedTables: []string{},
		},
		{
			name: "allowlist mode excludes explicitly disabled",
			overridesYAML: `
models:
  defaultEnabled: false
  overrides:
    fct_block: {}
    fct_attestation:
      enabled: false
`,
			expectedTables: []string{
				"fct_block",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoDir := setupFakeXatuCBTRepo(
				t, allExternal, allTransformations,
			)

			tmpDir := t.TempDir()
			overridesPath := filepath.Join(tmpDir, ".cbt-overrides.yaml")

			if tt.noOverridesFile {
				overridesPath = filepath.Join(tmpDir, "nonexistent.yaml")
			} else if tt.overridesYAML != "" {
				err := os.WriteFile(
					overridesPath,
					[]byte(tt.overridesYAML),
					0600,
				)
				require.NoError(t, err)
			}

			gen := NewGenerator(
				logrus.New(),
				&config.LabConfig{
					Repos: config.LabReposConfig{
						XatuCBT: repoDir,
					},
				},
			)

			tables, err := gen.getLocallyEnabledTables(overridesPath)

			if tt.expectError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expectedTables, tables)
		})
	}
}

func TestDiscoverAllModels(t *testing.T) {
	t.Run("models without database frontmatter", func(t *testing.T) {
		repoDir := setupFakeXatuCBTRepo(
			t,
			[]string{"fct_block", "fct_attestation"},
			[]string{"fct_summary"},
		)

		models, err := discoverAllModels(repoDir)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"fct_attestation",
			"fct_block",
			"fct_summary",
		}, models)
	})

	t.Run("external models with database frontmatter use prefixed keys", func(t *testing.T) {
		repoDir := setupFakeXatuCBTRepo(
			t,
			[]string{"cpu_utilization", "fct_block"},
			[]string{"fct_summary"},
		)

		// Add database frontmatter to cpu_utilization to simulate observoor.
		sqlWithFrontmatter := "---\ndatabase: observoor\ntable: cpu_utilization\n---\nSELECT 1"
		path := filepath.Join(repoDir, "models", "external", "cpu_utilization.sql")
		require.NoError(t, os.WriteFile(path, []byte(sqlWithFrontmatter), 0600))

		models, err := discoverAllModels(repoDir)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"fct_block",
			"fct_summary",
			"observoor.cpu_utilization",
		}, models)
	})
}

func TestLoadModelStates(t *testing.T) {
	t.Run("denylist mode", func(t *testing.T) {
		tmpDir := t.TempDir()
		overridesPath := filepath.Join(tmpDir, ".cbt-overrides.yaml")

		overridesYAML := `
models:
  overrides:
    fct_block:
      enabled: false
    fct_attestation:
      enabled: false
    fct_block_head: {}
`
		require.NoError(t,
			os.WriteFile(overridesPath, []byte(overridesYAML), 0600),
		)

		states, err := loadModelStates(overridesPath)
		require.NoError(t, err)
		assert.True(t, states.defaultEnabled)
		assert.True(t, states.disabled["fct_block"])
		assert.True(t, states.disabled["fct_attestation"])
		assert.False(t, states.disabled["fct_block_head"])
		assert.True(t, states.enabled["fct_block_head"])
	})

	t.Run("allowlist mode", func(t *testing.T) {
		tmpDir := t.TempDir()
		overridesPath := filepath.Join(tmpDir, ".cbt-overrides.yaml")

		overridesYAML := `
models:
  defaultEnabled: false
  overrides:
    fct_block: {}
    fct_attestation:
      enabled: false
`
		require.NoError(t,
			os.WriteFile(overridesPath, []byte(overridesYAML), 0600),
		)

		states, err := loadModelStates(overridesPath)
		require.NoError(t, err)
		assert.False(t, states.defaultEnabled)
		assert.True(t, states.enabled["fct_block"])
		assert.True(t, states.disabled["fct_attestation"])
	})
}

func TestRemoveEmptyMaps(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:     "empty top-level map",
			input:    map[string]any{},
			expected: map[string]any{},
		},
		{
			name: "removes empty nested map",
			input: map[string]any{
				"models": map[string]any{
					"env": map[string]any{},
				},
			},
			expected: map[string]any{},
		},
		{
			name: "preserves non-empty nested map",
			input: map[string]any{
				"models": map[string]any{
					"env": map[string]any{
						"NETWORK": "mainnet",
					},
				},
			},
			expected: map[string]any{
				"models": map[string]any{
					"env": map[string]any{
						"NETWORK": "mainnet",
					},
				},
			},
		},
		{
			name: "removes only empty branches",
			input: map[string]any{
				"models": map[string]any{
					"env":    map[string]any{},
					"config": "keep-me",
				},
				"other": "value",
			},
			expected: map[string]any{
				"models": map[string]any{
					"config": "keep-me",
				},
				"other": "value",
			},
		},
		{
			name: "preserves non-map values",
			input: map[string]any{
				"string_val": "hello",
				"int_val":    42,
				"bool_val":   true,
			},
			expected: map[string]any{
				"string_val": "hello",
				"int_val":    42,
				"bool_val":   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removeEmptyMaps(tt.input)
			assert.Equal(t, tt.expected, tt.input)
		})
	}
}

func TestEmptyOverrideDoesNotWipeAutoDefaults(t *testing.T) {
	baseConfig := map[string]any{
		"models": map[string]any{
			"env": map[string]any{
				"NETWORK":                      "mainnet",
				"EXTERNAL_MODEL_MIN_TIMESTAMP": "1234567890",
				"EXTERNAL_MODEL_MIN_BLOCK":     "23800000",
				"MODELS_SCRIPTS_PATH":          "../xatu-cbt/models/scripts",
			},
		},
	}

	userOverrides := map[string]any{
		"models": map[string]any{
			"env": map[string]any{},
		},
	}

	removeEmptyMaps(userOverrides)

	err := mergo.Merge(&baseConfig, userOverrides, mergo.WithOverride)
	require.NoError(t, err)

	models, ok := baseConfig["models"].(map[string]any)
	require.True(t, ok, "models section must exist")

	env, ok := models["env"].(map[string]any)
	require.True(t, ok, "models.env section must exist")

	assert.Equal(t, "mainnet", env["NETWORK"])
	assert.Equal(t, "1234567890", env["EXTERNAL_MODEL_MIN_TIMESTAMP"])
	assert.Equal(t, "23800000", env["EXTERNAL_MODEL_MIN_BLOCK"])
	assert.Equal(t, "../xatu-cbt/models/scripts", env["MODELS_SCRIPTS_PATH"])
}

func TestUserOverridesTakePrecedence(t *testing.T) {
	baseConfig := map[string]any{
		"models": map[string]any{
			"env": map[string]any{
				"NETWORK":                      "mainnet",
				"EXTERNAL_MODEL_MIN_TIMESTAMP": "1234567890",
				"EXTERNAL_MODEL_MIN_BLOCK":     "23800000",
			},
		},
	}

	userOverrides := map[string]any{
		"models": map[string]any{
			"env": map[string]any{
				"EXTERNAL_MODEL_MIN_TIMESTAMP": "0",
				"EXTERNAL_MODEL_MIN_BLOCK":     "0",
			},
		},
	}

	removeEmptyMaps(userOverrides)

	err := mergo.Merge(&baseConfig, userOverrides, mergo.WithOverride)
	require.NoError(t, err)

	models, ok := baseConfig["models"].(map[string]any)
	require.True(t, ok)

	env, ok := models["env"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "0", env["EXTERNAL_MODEL_MIN_TIMESTAMP"])
	assert.Equal(t, "0", env["EXTERNAL_MODEL_MIN_BLOCK"])
	assert.Equal(t, "mainnet", env["NETWORK"])
}

func TestCommentOnlyEnvOverrideDoesNotWipeAutoDefaults(t *testing.T) {
	baseConfig := map[string]any{
		"models": map[string]any{
			"env": map[string]any{
				"NETWORK":                      "mainnet",
				"EXTERNAL_MODEL_MIN_TIMESTAMP": "1234567890",
				"EXTERNAL_MODEL_MIN_BLOCK":     "23800000",
				"MODELS_SCRIPTS_PATH":          "../xatu-cbt/models/scripts",
			},
		},
	}

	tmpDir := t.TempDir()
	overridesPath := filepath.Join(tmpDir, ".cbt-overrides.yaml")
	overridesYAML := `
models:
  env:
    # EXTERNAL_MODEL_MIN_TIMESTAMP: "0"
    # EXTERNAL_MODEL_MIN_BLOCK: "0"
`
	require.NoError(t, os.WriteFile(overridesPath, []byte(overridesYAML), 0600))

	userOverrides, err := loadYAMLFile(overridesPath)
	require.NoError(t, err)
	removeEmptyMaps(userOverrides)

	err = mergo.Merge(&baseConfig, userOverrides, mergo.WithOverride)
	require.NoError(t, err)

	models, ok := baseConfig["models"].(map[string]any)
	require.True(t, ok)

	env, ok := models["env"].(map[string]any)
	require.True(t, ok, "models.env section must remain populated")

	assert.Equal(t, "mainnet", env["NETWORK"])
	assert.Equal(t, "1234567890", env["EXTERNAL_MODEL_MIN_TIMESTAMP"])
	assert.Equal(t, "23800000", env["EXTERNAL_MODEL_MIN_BLOCK"])
	assert.Equal(t, "../xatu-cbt/models/scripts", env["MODELS_SCRIPTS_PATH"])
}

func TestExpandDefaultEnabled(t *testing.T) {
	t.Run("no defaultEnabled does nothing", func(t *testing.T) {
		repoDir := setupFakeXatuCBTRepo(t,
			[]string{"fct_block", "fct_attestation"}, nil,
		)

		gen := NewGenerator(logrus.New(), &config.LabConfig{
			Repos: config.LabReposConfig{XatuCBT: repoDir},
		})

		overrides := map[string]any{
			"models": map[string]any{
				"overrides": map[string]any{
					"fct_block": map[string]any{},
				},
			},
		}

		gen.expandDefaultEnabled(overrides)

		models, ok := overrides["models"].(map[string]any)
		require.True(t, ok)

		ov, ok := models["overrides"].(map[string]any)
		require.True(t, ok)

		// Only the original entry should exist.
		assert.Len(t, ov, 1)
		assert.Contains(t, ov, "fct_block")
	})

	t.Run("defaultEnabled false expands unlisted models", func(t *testing.T) {
		repoDir := setupFakeXatuCBTRepo(t,
			[]string{"fct_block", "fct_attestation", "fct_proposer"},
			[]string{"fct_summary"},
		)

		gen := NewGenerator(logrus.New(), &config.LabConfig{
			Repos: config.LabReposConfig{XatuCBT: repoDir},
		})

		overrides := map[string]any{
			"models": map[string]any{
				"defaultEnabled": false,
				"overrides": map[string]any{
					"fct_block": map[string]any{},
				},
			},
		}

		gen.expandDefaultEnabled(overrides)

		models, ok := overrides["models"].(map[string]any)
		require.True(t, ok)

		ov, ok := models["overrides"].(map[string]any)
		require.True(t, ok)

		// fct_block should be untouched (still listed, no enabled:false injected).
		assert.Equal(t, map[string]any{}, ov["fct_block"])

		// All other models should have enabled: false injected.
		for _, name := range []string{"fct_attestation", "fct_proposer", "fct_summary"} {
			entry, ok := ov[name]
			require.True(t, ok, "expected %s to be injected", name)

			entryMap, ok := entry.(map[string]any)
			require.True(t, ok)
			assert.Equal(t, false, entryMap["enabled"])
		}

		// defaultEnabled key should be removed after expansion.
		_, hasDefault := models["defaultEnabled"]
		assert.False(t, hasDefault, "defaultEnabled should be removed after expansion")
	})

	t.Run("defaultEnabled true does nothing", func(t *testing.T) {
		repoDir := setupFakeXatuCBTRepo(t,
			[]string{"fct_block", "fct_attestation"}, nil,
		)

		gen := NewGenerator(logrus.New(), &config.LabConfig{
			Repos: config.LabReposConfig{XatuCBT: repoDir},
		})

		overrides := map[string]any{
			"models": map[string]any{
				"defaultEnabled": true,
				"overrides": map[string]any{
					"fct_block": map[string]any{},
				},
			},
		}

		gen.expandDefaultEnabled(overrides)

		models, ok := overrides["models"].(map[string]any)
		require.True(t, ok)

		ov, ok := models["overrides"].(map[string]any)
		require.True(t, ok)

		// Should not inject anything.
		assert.Len(t, ov, 1)
	})
}
