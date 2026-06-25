package cc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/ethpandaops/xcli/pkg/orchestrator"
	"github.com/ethpandaops/xcli/pkg/workspace"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestLabBackendRedisAddrUsesRuntimePort(t *testing.T) {
	labCfg := config.DefaultLab()
	labCfg.Infrastructure.RedisPort = 6380
	runtime := &instance.Runtime{
		Ports: instance.PortPlan{Redis: 7380},
		Manifest: &instance.Manifest{
			Ports: instance.PortPlan{Redis: 7380},
		},
	}

	backend := &labBackend{
		labCfg:  labCfg,
		runtime: runtime,
	}

	require.Equal(t, "localhost:7380", backend.RedisAddr())

	summary, ok := backend.GetConfigSummary().(configResponse)
	require.True(t, ok)
	require.Equal(t, 7380, summary.Ports.Redis)
}

func TestLabBackendConfigOverrideUsesWorkspaceCustomConfigDir(t *testing.T) {
	runtime := newTestLabRuntime(t, "cc-override")
	orch, err := orchestrator.NewOrchestratorWithRuntime(logrus.New(), runtime)
	require.NoError(t, err)

	backend := newLabBackend(
		logrus.New(),
		orch,
		runtime.LabConfig,
		runtime.Workspace.ConfigPath,
		runtime,
		nil,
	)
	name := fmt.Sprintf(constants.ConfigFileCBTAPI, "mainnet")
	content := "custom: true\n"

	require.NoError(t, backend.PutConfigFileOverride(name, content))

	workspaceOverride := filepath.Join(runtime.Workspace.StateDir, constants.DirCustomConfigs, name)
	instanceOverride := filepath.Join(runtime.Manifest.StateDir, constants.DirCustomConfigs, name)
	generated := filepath.Join(runtime.Manifest.StateDir, constants.DirConfigs, name)

	require.FileExists(t, workspaceOverride)
	require.NoFileExists(t, instanceOverride)
	require.FileExists(t, generated)

	generatedContent, err := os.ReadFile(generated)
	require.NoError(t, err)
	require.Equal(t, content, string(generatedContent))

	file, err := backend.GetConfigFile(name)
	require.NoError(t, err)
	require.True(t, file.HasOverride)
	require.Equal(t, content, file.OverrideContent)
}

func newTestLabRuntime(t *testing.T, instanceID string) *instance.Runtime {
	t.Helper()

	rootDir := t.TempDir()
	reposDir := filepath.Join(rootDir, "repos")
	repoPaths := map[string]string{
		constants.RepoCBT:        filepath.Join(reposDir, constants.RepoCBT),
		constants.RepoXatuCBT:    filepath.Join(reposDir, constants.RepoXatuCBT),
		constants.RepoCBTAPI:     filepath.Join(reposDir, constants.RepoCBTAPI),
		constants.RepoLabBackend: filepath.Join(reposDir, constants.RepoLabBackend),
		constants.RepoLab:        filepath.Join(reposDir, constants.RepoLab),
	}

	for _, path := range repoPaths {
		require.NoError(t, os.MkdirAll(path, 0755))
	}

	require.NoError(t, os.MkdirAll(filepath.Join(repoPaths[constants.RepoXatuCBT], "models", "external"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoPaths[constants.RepoXatuCBT], "models", "transformations"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(repoPaths[constants.RepoXatuCBT], "models", "scripts"), 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(repoPaths[constants.RepoXatuCBT], "models", "external", "fct_block.sql"),
		[]byte("SELECT 1"),
		0600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(repoPaths[constants.RepoXatuCBT], "models", "transformations", "fct_summary.sql"),
		[]byte("SELECT 1"),
		0600,
	))

	labCfg := config.DefaultLab()
	labCfg.Repos = config.LabReposConfig{
		CBT:        repoPaths[constants.RepoCBT],
		XatuCBT:    repoPaths[constants.RepoXatuCBT],
		CBTAPI:     repoPaths[constants.RepoCBTAPI],
		LabBackend: repoPaths[constants.RepoLabBackend],
		Lab:        repoPaths[constants.RepoLab],
	}
	labCfg.Networks = []config.NetworkConfig{{Name: "mainnet", Enabled: true, PortOffset: 0}}
	labCfg.Infrastructure.Observability.Enabled = false

	configPath := filepath.Join(rootDir, config.DefaultConfigFileName)
	require.NoError(t, (&config.Config{Lab: labCfg}).Save(configPath))

	ws := &workspace.Workspace{
		RootDir:       rootDir,
		ConfigPath:    configPath,
		OverridesPath: filepath.Join(rootDir, constants.CBTOverridesFile),
		StateDir:      filepath.Join(rootDir, ".xcli"),
		ConfigExists:  true,
	}

	manifest, err := instance.NewManifest(context.Background(), ws, labCfg, instanceID)
	require.NoError(t, err)
	ports, err := instance.BuildPortPlan(labCfg, 0)
	require.NoError(t, err)

	dockerPlan := instance.NewDockerPlan(manifest.InstanceID, manifest.ConfigPath)
	manifest.Ports = ports
	manifest.Docker = dockerPlan

	return &instance.Runtime{
		Workspace:  ws,
		LabConfig:  labCfg,
		Registry:   instance.NewRegistry(filepath.Join(t.TempDir(), "instances")),
		InstanceID: manifest.InstanceID,
		Manifest:   manifest,
		Ports:      ports,
		Docker:     dockerPlan,
		Repos:      manifest.Repos,
	}
}
