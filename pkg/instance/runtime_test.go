package instance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/workspace"
	"github.com/stretchr/testify/require"
)

func TestNewRuntimeFromWorkspaceAssemblesAndClaimsDistinctPlans(t *testing.T) {
	registry := NewRegistry(filepath.Join(t.TempDir(), "lab", "instances"))
	allocator := NewAllocator(registry, false)

	firstConfig := writeRuntimeConfig(t, "first")
	secondConfig := writeRuntimeConfig(t, "second")
	firstLabCfg, firstWorkspace, err := workspace.LoadLabConfig(firstConfig, false)
	require.NoError(t, err)
	secondLabCfg, secondWorkspace, err := workspace.LoadLabConfig(secondConfig, false)
	require.NoError(t, err)

	first, err := NewRuntimeFromWorkspace(context.Background(), firstWorkspace, firstLabCfg, "", RuntimeOptions{
		Registry:   registry,
		Allocator:  allocator,
		ClaimPorts: true,
	})
	require.NoError(t, err)

	second, err := NewRuntimeFromWorkspace(context.Background(), secondWorkspace, secondLabCfg, "", RuntimeOptions{
		Registry:   registry,
		Allocator:  allocator,
		ClaimPorts: true,
	})
	require.NoError(t, err)

	require.NotEqual(t, first.InstanceID, second.InstanceID)
	require.Equal(t, 0, first.Ports.Slot)
	require.Equal(t, 1, second.Ports.Slot)
	require.Empty(t, first.Ports.Overlaps(second.Ports))
	require.Equal(t, first.Ports, first.Manifest.Ports)
	require.Equal(t, InstanceStateDir(first.Workspace.RootDir, first.InstanceID), first.Manifest.StateDir)
	require.Equal(t, filepath.Join(filepath.Dir(firstConfig), "repos", "cbt"), first.LabConfig.Repos.CBT)
}

func TestResolveRuntimeExplicitInstanceLoadsManifestWorkspace(t *testing.T) {
	registry := NewRegistry(filepath.Join(t.TempDir(), "lab", "instances"))
	allocator := NewAllocator(registry, false)

	foreignConfig := writeRuntimeConfig(t, "foreign")
	currentConfig := writeRuntimeConfig(t, "current")
	foreignLabCfg, foreignWorkspace, err := workspace.LoadLabConfig(foreignConfig, false)
	require.NoError(t, err)

	foreign, err := NewRuntimeFromWorkspace(context.Background(), foreignWorkspace, foreignLabCfg, "", RuntimeOptions{
		Registry:   registry,
		Allocator:  allocator,
		ClaimPorts: true,
	})
	require.NoError(t, err)

	currentLabCfg, currentWorkspace, err := workspace.LoadLabConfig(currentConfig, false)
	require.NoError(t, err)

	resolved, err := ResolveRuntimeFromWorkspace(
		context.Background(),
		currentWorkspace,
		currentLabCfg,
		foreign.InstanceID,
		RuntimeOptions{Registry: registry, ProbePorts: false},
	)
	require.NoError(t, err)
	require.Equal(t, foreign.Workspace.ConfigPath, resolved.Workspace.ConfigPath)
	require.Equal(t, foreign.Workspace.RootDir, resolved.Workspace.RootDir)
	require.Equal(t, foreign.Manifest.ConfigPath, resolved.Manifest.ConfigPath)
	require.Equal(t, filepath.Join(foreign.Workspace.RootDir, "repos", "cbt"), resolved.LabConfig.Repos.CBT)
	require.NotEqual(t, currentWorkspace.ConfigPath, resolved.Workspace.ConfigPath)
}

func writeRuntimeConfig(t *testing.T, name string) string {
	t.Helper()

	rootDir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.MkdirAll(filepath.Join(rootDir, "repos"), 0755))

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
