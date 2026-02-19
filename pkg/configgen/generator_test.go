package configgen

import (
	"os"
	"path/filepath"
	"testing"

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
}

func TestLoadDisabledModels(t *testing.T) {
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

	disabled, err := loadDisabledModels(overridesPath)
	require.NoError(t, err)
	assert.True(t, disabled["fct_block"])
	assert.True(t, disabled["fct_attestation"])
	assert.False(t, disabled["fct_block_head"])
}
