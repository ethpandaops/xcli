package configgen

import (
	"testing"

	"dario.cat/mergo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveEmptyMaps(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "empty top-level map",
			input:    map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "removes empty nested map",
			input: map[string]interface{}{
				"models": map[string]interface{}{
					"env": map[string]interface{}{},
				},
			},
			expected: map[string]interface{}{},
		},
		{
			name: "preserves non-empty nested map",
			input: map[string]interface{}{
				"models": map[string]interface{}{
					"env": map[string]interface{}{
						"NETWORK": "mainnet",
					},
				},
			},
			expected: map[string]interface{}{
				"models": map[string]interface{}{
					"env": map[string]interface{}{
						"NETWORK": "mainnet",
					},
				},
			},
		},
		{
			name: "removes only empty branches",
			input: map[string]interface{}{
				"models": map[string]interface{}{
					"env":    map[string]interface{}{},
					"config": "keep-me",
				},
				"other": "value",
			},
			expected: map[string]interface{}{
				"models": map[string]interface{}{
					"config": "keep-me",
				},
				"other": "value",
			},
		},
		{
			name: "preserves non-map values",
			input: map[string]interface{}{
				"string_val": "hello",
				"int_val":    42,
				"bool_val":   true,
			},
			expected: map[string]interface{}{
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
	// Simulate auto-defaults with populated env
	baseConfig := map[string]interface{}{
		"models": map[string]interface{}{
			"env": map[string]interface{}{
				"NETWORK":                      "mainnet",
				"EXTERNAL_MODEL_MIN_TIMESTAMP": "1234567890",
				"EXTERNAL_MODEL_MIN_BLOCK":     "23800000",
				"MODELS_SCRIPTS_PATH":          "../xatu-cbt/models/scripts",
			},
		},
	}

	// Simulate user overrides with empty env (YAML comments only)
	userOverrides := map[string]interface{}{
		"models": map[string]interface{}{
			"env": map[string]interface{}{},
		},
	}

	removeEmptyMaps(userOverrides)

	err := mergo.Merge(&baseConfig, userOverrides, mergo.WithOverride)
	require.NoError(t, err)

	// Auto-defaults must survive
	models, ok := baseConfig["models"].(map[string]interface{})
	require.True(t, ok, "models section must exist")

	env, ok := models["env"].(map[string]interface{})
	require.True(t, ok, "models.env section must exist")

	assert.Equal(t, "mainnet", env["NETWORK"])
	assert.Equal(t, "1234567890", env["EXTERNAL_MODEL_MIN_TIMESTAMP"])
	assert.Equal(t, "23800000", env["EXTERNAL_MODEL_MIN_BLOCK"])
	assert.Equal(t, "../xatu-cbt/models/scripts", env["MODELS_SCRIPTS_PATH"])
}

func TestUserOverridesTakePrecedence(t *testing.T) {
	baseConfig := map[string]interface{}{
		"models": map[string]interface{}{
			"env": map[string]interface{}{
				"NETWORK":                      "mainnet",
				"EXTERNAL_MODEL_MIN_TIMESTAMP": "1234567890",
				"EXTERNAL_MODEL_MIN_BLOCK":     "23800000",
			},
		},
	}

	// User explicitly sets values
	userOverrides := map[string]interface{}{
		"models": map[string]interface{}{
			"env": map[string]interface{}{
				"EXTERNAL_MODEL_MIN_TIMESTAMP": "0",
				"EXTERNAL_MODEL_MIN_BLOCK":     "0",
			},
		},
	}

	removeEmptyMaps(userOverrides)

	err := mergo.Merge(&baseConfig, userOverrides, mergo.WithOverride)
	require.NoError(t, err)

	models, ok := baseConfig["models"].(map[string]interface{})
	require.True(t, ok)

	env, ok := models["env"].(map[string]interface{})
	require.True(t, ok)

	// User values must override
	assert.Equal(t, "0", env["EXTERNAL_MODEL_MIN_TIMESTAMP"])
	assert.Equal(t, "0", env["EXTERNAL_MODEL_MIN_BLOCK"])

	// Untouched auto-defaults must survive
	assert.Equal(t, "mainnet", env["NETWORK"])
}
