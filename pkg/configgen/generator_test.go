package configgen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"dario.cat/mergo"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/ethpandaops/xcli/pkg/workspace"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const (
	fctBlock            = "fct_block"
	fctBlockHead        = "fct_block_head"
	fctAttestation      = "fct_attestation"
	fctProposerSlashing = "fct_proposer_slashing"
	fctBlockSummary     = "fct_block_summary"
	fctSummary          = "fct_summary"
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

	externalDir := filepath.Join(repoDir, keyModels, "external")
	require.NoError(t, os.MkdirAll(externalDir, 0755))

	for _, name := range external {
		path := filepath.Join(externalDir, name+".sql")
		require.NoError(t, os.WriteFile(path, []byte("SELECT 1"), 0600))
	}

	transformDir := filepath.Join(repoDir, keyModels, "transformations")
	require.NoError(t, os.MkdirAll(transformDir, 0755))

	for _, name := range transformations {
		path := filepath.Join(transformDir, name+".sql")
		require.NoError(t, os.WriteFile(path, []byte("SELECT 1"), 0600))
	}

	return repoDir
}

func TestGetLocallyEnabledTables(t *testing.T) {
	allExternal := []string{
		fctBlock,
		fctBlockHead,
		fctAttestation,
		fctProposerSlashing,
	}
	allTransformations := []string{fctBlockSummary}

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
				fctAttestation,
				fctBlock,
				fctBlockHead,
				fctBlockSummary,
				fctProposerSlashing,
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
				fctBlock,
				fctBlockHead,
				fctBlockSummary,
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
				fctAttestation,
				fctBlock,
				fctBlockHead,
				fctBlockSummary,
				fctProposerSlashing,
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
				fctBlock,
				fctBlockSummary,
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
				fctBlock,
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
			[]string{fctBlock, fctAttestation},
			[]string{fctSummary},
		)

		models, err := discoverAllModels(repoDir)
		require.NoError(t, err)
		assert.Equal(t, []string{
			fctAttestation,
			fctBlock,
			fctSummary,
		}, models)
	})

	t.Run("external models with database frontmatter use prefixed keys", func(t *testing.T) {
		repoDir := setupFakeXatuCBTRepo(
			t,
			[]string{"cpu_utilization", fctBlock},
			[]string{fctSummary},
		)

		// Add database frontmatter to cpu_utilization to simulate observoor.
		sqlWithFrontmatter := "---\ndatabase: observoor\ntable: cpu_utilization\n---\nSELECT 1"
		path := filepath.Join(repoDir, keyModels, "external", "cpu_utilization.sql")
		require.NoError(t, os.WriteFile(path, []byte(sqlWithFrontmatter), 0600))

		models, err := discoverAllModels(repoDir)
		require.NoError(t, err)
		assert.Equal(t, []string{
			fctBlock,
			fctSummary,
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
		assert.True(t, states.disabled[fctBlock])
		assert.True(t, states.disabled[fctAttestation])
		assert.False(t, states.disabled[fctBlockHead])
		assert.True(t, states.enabled[fctBlockHead])
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
		assert.True(t, states.enabled[fctBlock])
		assert.True(t, states.disabled[fctAttestation])
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
				keyModels: map[string]any{
					keyEnv: map[string]any{},
				},
			},
			expected: map[string]any{},
		},
		{
			name: "preserves non-empty nested map",
			input: map[string]any{
				keyModels: map[string]any{
					keyEnv: map[string]any{
						keyNetwork: networkMainnet,
					},
				},
			},
			expected: map[string]any{
				keyModels: map[string]any{
					keyEnv: map[string]any{
						keyNetwork: networkMainnet,
					},
				},
			},
		},
		{
			name: "removes only empty branches",
			input: map[string]any{
				keyModels: map[string]any{
					keyEnv:   map[string]any{},
					"config": "keep-me",
				},
				"other": keyValue,
			},
			expected: map[string]any{
				keyModels: map[string]any{
					"config": "keep-me",
				},
				"other": keyValue,
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
	scriptsPath := filepath.Join(constants.RepoXatuCBT, keyModels, "scripts")
	baseConfig := map[string]any{
		keyModels: map[string]any{
			keyEnv: map[string]any{
				keyNetwork:                   networkMainnet,
				envExternalModelMinTimestamp: "1234567890",
				envExternalModelMinBlock:     "23800000",
				envModelsScriptsPath:         scriptsPath,
			},
		},
	}

	userOverrides := map[string]any{
		keyModels: map[string]any{
			keyEnv: map[string]any{},
		},
	}

	removeEmptyMaps(userOverrides)

	err := mergo.Merge(&baseConfig, userOverrides, mergo.WithOverride)
	require.NoError(t, err)

	models, ok := baseConfig[keyModels].(map[string]any)
	require.True(t, ok, "models section must exist")

	env, ok := models[keyEnv].(map[string]any)
	require.True(t, ok, "models.env section must exist")

	assert.Equal(t, networkMainnet, env[keyNetwork])
	assert.Equal(t, "1234567890", env[envExternalModelMinTimestamp])
	assert.Equal(t, "23800000", env[envExternalModelMinBlock])
	assert.Equal(t, scriptsPath, env[envModelsScriptsPath])
}

func TestUserOverridesTakePrecedence(t *testing.T) {
	baseConfig := map[string]any{
		keyModels: map[string]any{
			keyEnv: map[string]any{
				keyNetwork:                   networkMainnet,
				envExternalModelMinTimestamp: "1234567890",
				envExternalModelMinBlock:     "23800000",
			},
		},
	}

	userOverrides := map[string]any{
		keyModels: map[string]any{
			keyEnv: map[string]any{
				envExternalModelMinTimestamp: "0",
				envExternalModelMinBlock:     "0",
			},
		},
	}

	removeEmptyMaps(userOverrides)

	err := mergo.Merge(&baseConfig, userOverrides, mergo.WithOverride)
	require.NoError(t, err)

	models, ok := baseConfig[keyModels].(map[string]any)
	require.True(t, ok)

	env, ok := models[keyEnv].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "0", env[envExternalModelMinTimestamp])
	assert.Equal(t, "0", env[envExternalModelMinBlock])
	assert.Equal(t, networkMainnet, env[keyNetwork])
}

func TestCommentOnlyEnvOverrideDoesNotWipeAutoDefaults(t *testing.T) {
	scriptsPath := filepath.Join(constants.RepoXatuCBT, keyModels, "scripts")
	baseConfig := map[string]any{
		keyModels: map[string]any{
			keyEnv: map[string]any{
				keyNetwork:                   networkMainnet,
				envExternalModelMinTimestamp: "1234567890",
				envExternalModelMinBlock:     "23800000",
				envModelsScriptsPath:         scriptsPath,
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

	models, ok := baseConfig[keyModels].(map[string]any)
	require.True(t, ok)

	env, ok := models[keyEnv].(map[string]any)
	require.True(t, ok, "models.env section must remain populated")

	assert.Equal(t, networkMainnet, env[keyNetwork])
	assert.Equal(t, "1234567890", env[envExternalModelMinTimestamp])
	assert.Equal(t, "23800000", env[envExternalModelMinBlock])
	assert.Equal(t, scriptsPath, env[envModelsScriptsPath])
}

func TestExpandDefaultEnabled(t *testing.T) {
	t.Run("no defaultEnabled does nothing", func(t *testing.T) {
		repoDir := setupFakeXatuCBTRepo(t,
			[]string{fctBlock, fctAttestation}, nil,
		)

		gen := NewGenerator(logrus.New(), &config.LabConfig{
			Repos: config.LabReposConfig{XatuCBT: repoDir},
		})

		overrides := map[string]any{
			keyModels: map[string]any{
				keyOverrides: map[string]any{
					fctBlock: map[string]any{},
				},
			},
		}

		gen.expandDefaultEnabled(overrides)

		models, ok := overrides[keyModels].(map[string]any)
		require.True(t, ok)

		ov, ok := models[keyOverrides].(map[string]any)
		require.True(t, ok)

		// Only the original entry should exist.
		assert.Len(t, ov, 1)
		assert.Contains(t, ov, fctBlock)
	})

	t.Run("defaultEnabled false expands unlisted models", func(t *testing.T) {
		repoDir := setupFakeXatuCBTRepo(t,
			[]string{fctBlock, fctAttestation, "fct_proposer"},
			[]string{fctSummary},
		)

		gen := NewGenerator(logrus.New(), &config.LabConfig{
			Repos: config.LabReposConfig{XatuCBT: repoDir},
		})

		overrides := map[string]any{
			keyModels: map[string]any{
				"defaultEnabled": false,
				keyOverrides: map[string]any{
					fctBlock: map[string]any{},
				},
			},
		}

		gen.expandDefaultEnabled(overrides)

		models, ok := overrides[keyModels].(map[string]any)
		require.True(t, ok)

		ov, ok := models[keyOverrides].(map[string]any)
		require.True(t, ok)

		// fct_block should be untouched (still listed, no enabled:false injected).
		assert.Equal(t, map[string]any{}, ov[fctBlock])

		// All other models should have enabled: false injected.
		for _, name := range []string{fctAttestation, "fct_proposer", fctSummary} {
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
			[]string{fctBlock, fctAttestation}, nil,
		)

		gen := NewGenerator(logrus.New(), &config.LabConfig{
			Repos: config.LabReposConfig{XatuCBT: repoDir},
		})

		overrides := map[string]any{
			keyModels: map[string]any{
				"defaultEnabled": true,
				keyOverrides: map[string]any{
					fctBlock: map[string]any{},
				},
			},
		}

		gen.expandDefaultEnabled(overrides)

		models, ok := overrides[keyModels].(map[string]any)
		require.True(t, ok)

		ov, ok := models[keyOverrides].(map[string]any)
		require.True(t, ok)

		// Should not inject anything.
		assert.Len(t, ov, 1)
	})
}

func TestRuntimeGeneratorUsesAllocatedPathsAndPorts(t *testing.T) {
	registry := instance.NewRegistry(filepath.Join(t.TempDir(), "lab", "instances"))
	allocator := instance.NewAllocator(registry, false)

	firstRuntime := newConfiggenRuntime(t, registry, allocator, "first")
	secondRuntime := newConfiggenRuntime(t, registry, allocator, "second")

	require.Equal(t, 0, firstRuntime.Ports.Slot)
	require.Equal(t, 1, secondRuntime.Ports.Slot)
	require.Empty(t, firstRuntime.Ports.Overlaps(secondRuntime.Ports))

	firstDir, err := NewRuntimeGenerator(logrus.New(), firstRuntime).GenerateRuntimeConfigs()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(firstRuntime.Manifest.StateDir, constants.DirConfigs), firstDir)

	secondDir, err := NewRuntimeGenerator(logrus.New(), secondRuntime).GenerateRuntimeConfigs()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(secondRuntime.Manifest.StateDir, constants.DirConfigs), secondDir)

	assertGeneratedConfigMatchesRuntime(t, firstRuntime, firstDir)
	assertGeneratedConfigMatchesRuntime(t, secondRuntime, secondDir)
	assertNoForbiddenRuntimeLiterals(t, firstRuntime, firstDir)
	assertNoForbiddenRuntimeLiterals(t, secondRuntime, secondDir)
}

func TestRuntimeGeneratorCopiesCustomConfigsFromRootState(t *testing.T) {
	registry := instance.NewRegistry(filepath.Join(t.TempDir(), "lab", "instances"))
	allocator := instance.NewAllocator(registry, false)
	runtime := newConfiggenRuntime(t, registry, allocator, "custom")

	customConfigsDir := filepath.Join(runtime.Workspace.StateDir, constants.DirCustomConfigs)
	require.NoError(t, os.MkdirAll(customConfigsDir, 0755))

	customAPIConfig := "custom: true\n"
	customAPIPath := filepath.Join(
		customConfigsDir,
		fmt.Sprintf(constants.ConfigFileCBTAPI, networkMainnet),
	)
	require.NoError(t, os.WriteFile(customAPIPath, []byte(customAPIConfig), 0600))

	configsDir, err := NewRuntimeGenerator(logrus.New(), runtime).GenerateRuntimeConfigs()
	require.NoError(t, err)

	renderedAPIConfig := readGeneratedConfig(
		t,
		configsDir,
		fmt.Sprintf(constants.ConfigFileCBTAPI, networkMainnet),
	)
	require.Equal(t, customAPIConfig, renderedAPIConfig)
}

func newConfiggenRuntime(
	t *testing.T,
	registry *instance.Registry,
	allocator *instance.Allocator,
	name string,
) *instance.Runtime {
	t.Helper()

	configPath := writeConfiggenRuntimeConfig(t, name)
	labCfg, ws, err := workspace.LoadLabConfig(configPath, false)
	require.NoError(t, err)

	runtime, err := instance.NewRuntimeFromWorkspace(
		context.Background(),
		ws,
		labCfg,
		"",
		instance.RuntimeOptions{
			Registry:   registry,
			Allocator:  allocator,
			ClaimPorts: true,
		},
	)
	require.NoError(t, err)

	return runtime
}

func writeConfiggenRuntimeConfig(t *testing.T, name string) string {
	t.Helper()

	rootDir := filepath.Join(t.TempDir(), name)
	reposDir := filepath.Join(rootDir, "repos")
	repos := []string{
		constants.RepoCBT,
		constants.RepoXatuCBT,
		constants.RepoCBTAPI,
		constants.RepoLabBackend,
		constants.RepoLab,
	}
	for _, repo := range repos {
		require.NoError(t, os.MkdirAll(filepath.Join(reposDir, repo), 0755))
	}

	xatuCBTRepo := filepath.Join(reposDir, constants.RepoXatuCBT)
	require.NoError(t, os.MkdirAll(filepath.Join(xatuCBTRepo, keyModels, "external"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(xatuCBTRepo, keyModels, "transformations"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(xatuCBTRepo, keyModels, "external", fctBlock+".sql"),
		[]byte("SELECT 1"),
		0600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(xatuCBTRepo, keyModels, "transformations", fctSummary+".sql"),
		[]byte("SELECT 1"),
		0600,
	))

	overridesPath := filepath.Join(rootDir, constants.CBTOverridesFile)
	overrides := `
models:
  env:
    EXTERNAL_MODEL_MIN_BLOCK: "123"
`
	require.NoError(t, os.WriteFile(overridesPath, []byte(overrides), 0600))

	configPath := filepath.Join(rootDir, config.DefaultConfigFileName)
	content := `
lab:
  mode: hybrid
  repos:
    cbt: repos/cbt
    xatuCbt: repos/xatu-cbt
    cbtApi: repos/cbt-api
    labBackend: repos/lab-backend
    lab: repos/lab
  networks:
    - name: mainnet
      enabled: true
      portOffset: 0
    - name: sepolia
      enabled: true
      portOffset: 1
  infrastructure:
    clickhouse:
      xatu:
        mode: external
        externalUrl: "http://example.invalid:9000"
      cbt:
        mode: local
    redis:
      port: 6380
    observability:
      enabled: true
      prometheusPort: 9090
      grafanaPort: 3000
    clickhouseXatuPort: 8125
    clickhouseCbtPort: 8123
    redisPort: 6380
  ports:
    labBackend: 8080
    labFrontend: 5173
    cbtBase: 8081
    cbtApiBase: 8091
    cbtFrontendBase: 8085
`
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0644))

	return configPath
}

func assertGeneratedConfigMatchesRuntime(
	t *testing.T,
	runtime *instance.Runtime,
	configsDir string,
) {
	t.Helper()

	xatuCBTRepo := runtime.LabConfig.Repos.XatuCBT
	require.True(t, filepath.IsAbs(xatuCBTRepo))

	for _, network := range runtime.LabConfig.EnabledNetworks() {
		ports := runtime.Ports.Networks[network.Name]

		cbtConfig := readGeneratedConfig(
			t,
			configsDir,
			fmt.Sprintf(constants.ConfigFileCBT, network.Name),
		)
		assert.Contains(t, cbtConfig, filepath.Join(xatuCBTRepo, keyModels, "external"))
		assert.Contains(t, cbtConfig, filepath.Join(xatuCBTRepo, keyModels, "transformations"))
		assert.Contains(t, cbtConfig, filepath.Join(xatuCBTRepo, keyModels, "scripts"))
		assert.Contains(
			t,
			cbtConfig,
			"localhost:"+strconv.Itoa(runtime.Ports.ClickHouseCBT01HTTP),
		)
		assert.Contains(t, cbtConfig, "localhost:"+strconv.Itoa(runtime.Ports.Redis))
		assert.Contains(t, cbtConfig, strconv.Itoa(ports.CBTMetrics))
		assert.Contains(t, cbtConfig, strconv.Itoa(ports.CBTFrontend))
		assertRuntimeOverrideMerged(t, cbtConfig)

		apiConfig := readGeneratedConfig(
			t,
			configsDir,
			fmt.Sprintf(constants.ConfigFileCBTAPI, network.Name),
		)
		assert.Contains(t, apiConfig, "port: "+strconv.Itoa(ports.CBTAPI))
		assert.Contains(t, apiConfig, "metrics_port: "+strconv.Itoa(ports.CBTAPIMetrics))
		assert.Contains(
			t,
			apiConfig,
			"localhost:"+strconv.Itoa(runtime.Ports.ClickHouseCBT01TCP),
		)
	}

	backendConfig := readGeneratedConfig(t, configsDir, constants.ConfigFileLabBackend)
	assert.Contains(t, backendConfig, "port: "+strconv.Itoa(runtime.Ports.LabBackend))
	assert.Contains(t, backendConfig, "localhost:"+strconv.Itoa(runtime.Ports.Redis))
	for _, network := range runtime.LabConfig.EnabledNetworks() {
		assert.Contains(
			t,
			backendConfig,
			"localhost:"+strconv.Itoa(runtime.Ports.Networks[network.Name].CBTAPI),
		)
	}

	prometheusConfig := readGeneratedConfig(t, configsDir, "prometheus.yml")
	assert.Contains(
		t,
		prometheusConfig,
		"host.docker.internal:"+strconv.Itoa(runtime.Ports.LabBackend),
	)
	for _, network := range runtime.LabConfig.EnabledNetworks() {
		ports := runtime.Ports.Networks[network.Name]
		assert.Contains(
			t,
			prometheusConfig,
			"host.docker.internal:"+strconv.Itoa(ports.CBTMetrics),
		)
		assert.Contains(
			t,
			prometheusConfig,
			"host.docker.internal:"+strconv.Itoa(ports.CBTAPIMetrics),
		)
	}

	datasourceConfig := readGeneratedConfig(
		t,
		configsDir,
		filepath.Join("grafana", "provisioning", "datasources", "datasource.yml"),
	)
	assert.Contains(
		t,
		datasourceConfig,
		"host.docker.internal:"+strconv.Itoa(runtime.Ports.Prometheus),
	)
}

func assertRuntimeOverrideMerged(t *testing.T, cbtConfig string) {
	t.Helper()

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(cbtConfig), &parsed))

	models, ok := parsed[keyModels].(map[string]any)
	require.True(t, ok)

	env, ok := models[keyEnv].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "123", env[envExternalModelMinBlock])
}

func assertNoForbiddenRuntimeLiterals(
	t *testing.T,
	runtime *instance.Runtime,
	configsDir string,
) {
	t.Helper()

	generated := readGeneratedTree(t, configsDir)
	require.NotContains(t, generated, "../xatu-cbt")

	conditionalLiterals := map[string]int{
		"localhost:8123": 8123,
		"localhost:9000": 9000,
		"localhost:6380": 6380,
	}

	for literal, port := range conditionalLiterals {
		if !planContainsPort(runtime.Ports, port) {
			require.NotContains(t, generated, literal)
		}
	}

	for _, port := range []int{9100, 9200} {
		if !planContainsPort(runtime.Ports, port) {
			require.NotRegexp(t, regexp.MustCompile(`\b`+strconv.Itoa(port)+`\b`), generated)
		}
	}
}

func readGeneratedConfig(t *testing.T, configsDir, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(configsDir, name))
	require.NoError(t, err)

	return string(data)
}

func readGeneratedTree(t *testing.T, root string) string {
	t.Helper()

	var builder strings.Builder
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		builder.Write(data)
		builder.WriteByte('\n')

		return nil
	})
	require.NoError(t, err)

	return builder.String()
}

func planContainsPort(plan instance.PortPlan, port int) bool {
	for _, planned := range plan.AllPorts() {
		if planned == port {
			return true
		}
	}

	return false
}
