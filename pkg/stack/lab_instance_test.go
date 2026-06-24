package stack

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	xcligit "github.com/ethpandaops/xcli/pkg/git"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestLabStackInstanceOverrideLoadsManifestRuntime(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	rootDir := t.TempDir()
	configPath := filepath.Join(rootDir, config.DefaultConfigFileName)
	writeLifecycleConfig(t, rootDir, configPath)

	registry, err := instance.DefaultRegistry()
	require.NoError(t, err)

	manifest := &instance.Manifest{
		SchemaVersion: instance.SchemaVersion,
		InstanceID:    "beta",
		Status:        instance.StatusRunning,
		RootDir:       rootDir,
		ConfigPath:    configPath,
		OverridesPath: filepath.Join(rootDir, constants.CBTOverridesFile),
		StateDir:      instance.InstanceStateDir(rootDir, "beta"),
		Mode:          constants.ModeHybrid,
		Repos:         map[string]xcligit.RepoVersion{},
		Ports:         instance.PortPlan{LabBackend: 9080},
		Docker:        instance.NewDockerPlan("beta", configPath),
		PIDs:          map[string]int{},
		URLs:          map[string]string{},
		CreatedAt:     time.Now().UTC(),
	}
	require.NoError(t, registry.Save(manifest))

	override := "beta"
	stack := NewLabStack(logrus.New(), filepath.Join(t.TempDir(), config.DefaultConfigFileName), &override)
	runtime, err := stack.loadLifecycleRuntime()
	require.NoError(t, err)

	require.Equal(t, "beta", runtime.InstanceID)
	require.Equal(t, "xcli-beta", runtime.Docker.ProjectName)
	require.Equal(t, configPath, runtime.Manifest.ConfigPath)
	require.Equal(t, 9080, runtime.Ports.LabBackend)
}

func writeLifecycleConfig(t *testing.T, rootDir, configPath string) {
	t.Helper()

	reposDir := filepath.Join(rootDir, "repos")
	for _, repo := range []string{
		constants.RepoCBT,
		constants.RepoXatuCBT,
		constants.RepoCBTAPI,
		constants.RepoLabBackend,
		constants.RepoLab,
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(reposDir, repo), 0755))
	}

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
}
