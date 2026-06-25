package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/ethpandaops/xcli/pkg/process"
	"github.com/ethpandaops/xcli/pkg/workspace"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOrchestratorWithRuntimeUsesInstanceStateAndDockerPlan(t *testing.T) {
	rootDir := t.TempDir()
	configPath := filepath.Join(rootDir, config.DefaultConfigFileName)
	stateDir := instance.InstanceStateDir(rootDir, "gamma")
	dockerPlan := instance.NewDockerPlan("gamma", configPath)
	runtime := &instance.Runtime{
		LabConfig: testRuntimeLabConfig(),
		Docker:    dockerPlan,
		Manifest: &instance.Manifest{
			ConfigPath: configPath,
			StateDir:   stateDir,
			Docker:     dockerPlan,
		},
		Ports: instance.PortPlan{
			Prometheus: 19090,
			Grafana:    13000,
		},
	}

	orch, err := NewOrchestratorWithRuntime(logrus.New(), runtime)
	require.NoError(t, err)
	require.Equal(t, stateDir, orch.StateDir())

	containerName, ok := orch.InfrastructureManager().DockerContainerName(constants.ServicePrometheus)
	require.True(t, ok)
	require.Equal(t, "xcli-gamma-prometheus", containerName)
}

func TestFinalizeRuntimeManifestWritesRunningInstance(t *testing.T) {
	ctx := context.Background()
	runtime := newOrchestratorTestRuntime(t, "theta", 1)

	orch, err := NewOrchestratorWithRuntime(logrus.New(), runtime)
	require.NoError(t, err)

	orch.proc = &fakeProcessManager{
		processes: []*process.Process{
			{Name: constants.ServiceLabBackend, PID: 1234},
			{Name: constants.ServiceLabFrontend, PID: 5678},
		},
	}

	require.NoError(t, orch.finalizeRuntimeManifest(ctx))

	loaded, err := runtime.Registry.Load(runtime.InstanceID)
	require.NoError(t, err)
	require.Equal(t, instance.StatusRunning, loaded.Status)
	require.Equal(t, runtime.Ports.LabBackend, loaded.Ports.LabBackend)
	require.Equal(t, runtime.Docker.ProjectName, loaded.Docker.ProjectName)
	require.Equal(t, 1234, loaded.PIDs[constants.ServiceLabBackend])
	require.Equal(
		t,
		"http://localhost:6173",
		loaded.URLs[constants.ServiceLabFrontend],
	)
	require.Equal(
		t,
		"http://localhost:9080",
		loaded.URLs[constants.ServiceLabBackend],
	)
	require.Equal(
		t,
		"http://localhost:10090",
		loaded.URLs[constants.ServicePrometheus],
	)
	require.Equal(
		t,
		"http://localhost:20280",
		loaded.URLs["command-center"],
	)

	for _, repo := range []string{
		constants.RepoCBT,
		constants.RepoXatuCBT,
		constants.RepoCBTAPI,
		constants.RepoLabBackend,
		constants.RepoLab,
	} {
		version := loaded.Repos[repo]
		require.NotEmpty(t, version.Path)
		require.NotEmpty(t, version.Branch)
		require.NotEmpty(t, version.Commit)
		assert.False(t, version.Dirty)
	}

	_, err = os.Stat(instance.LocalManifestPath(runtime.Workspace.RootDir, runtime.InstanceID))
	require.NoError(t, err)
}

func TestMarkRuntimeStoppedPersistsStoppedManifest(t *testing.T) {
	runtime := newOrchestratorTestRuntime(t, "stopped", 0)
	runtime.Manifest.Status = instance.StatusRunning
	runtime.Manifest.PIDs = map[string]int{constants.ServiceLabBackend: 1234}
	require.NoError(t, runtime.Registry.Save(runtime.Manifest))

	orch, err := NewOrchestratorWithRuntime(logrus.New(), runtime)
	require.NoError(t, err)

	require.NoError(t, orch.markRuntimeStopped())

	loaded, err := runtime.Registry.Load(runtime.InstanceID)
	require.NoError(t, err)
	require.Equal(t, instance.StatusStopped, loaded.Status)
	require.Empty(t, loaded.PIDs)
}

func TestReleaseRuntimeReservationMarksStoppedWithLastError(t *testing.T) {
	runtime := newOrchestratorTestRuntime(t, "reserved-fail", 0)
	runtime.Manifest.Status = instance.StatusReserved
	require.NoError(t, runtime.Registry.Save(runtime.Manifest))

	orch, err := NewOrchestratorWithRuntime(logrus.New(), runtime)
	require.NoError(t, err)

	require.NoError(t, orch.releaseRuntimeReservation(assert.AnError))

	loaded, err := runtime.Registry.Load(runtime.InstanceID)
	require.NoError(t, err)
	require.Equal(t, instance.StatusStopped, loaded.Status)
	require.Contains(t, loaded.LastError, assert.AnError.Error())
	require.Empty(t, loaded.PIDs)
}

func TestReleaseRuntimeReservationIgnoresTransientInMemoryRunningStatus(t *testing.T) {
	runtime := newOrchestratorTestRuntime(t, "reserved-status-flipped", 0)
	runtime.Manifest.Status = instance.StatusReserved
	require.NoError(t, runtime.Registry.Save(runtime.Manifest))
	runtime.Manifest.Status = instance.StatusRunning

	orch, err := NewOrchestratorWithRuntime(logrus.New(), runtime)
	require.NoError(t, err)

	require.NoError(t, orch.releaseRuntimeReservation(assert.AnError))

	loaded, err := runtime.Registry.Load(runtime.InstanceID)
	require.NoError(t, err)
	require.Equal(t, instance.StatusStopped, loaded.Status)
	require.Contains(t, loaded.LastError, assert.AnError.Error())
}

func TestRuntimePortsDriveFrontendCommand(t *testing.T) {
	ctx := context.Background()
	runtime := newOrchestratorTestRuntime(t, "lambda", 1)
	require.NoError(t, os.MkdirAll(filepath.Join(runtime.LabConfig.Repos.Lab, "node_modules"), 0755))

	binDir := t.TempDir()
	pnpmPath := filepath.Join(binDir, "pnpm")
	require.NoError(t, os.WriteFile(pnpmPath, []byte("#!/bin/sh\nexit 0\n"), 0755))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	orch, err := NewOrchestratorWithRuntime(logrus.New(), runtime)
	require.NoError(t, err)

	fakeProc := &fakeProcessManager{}
	orch.proc = fakeProc

	require.NoError(t, orch.startLabFrontend(ctx))
	require.NotNil(t, fakeProc.lastCmd)
	require.Equal(t, runtime.LabConfig.Repos.Lab, fakeProc.lastCmd.Dir)
	require.Equal(t, constants.ServiceLabFrontend, fakeProc.lastName)
	require.Contains(t, strings.Join(fakeProc.lastCmd.Args, " "), "--port 6173")
	require.Contains(t, fakeProc.lastCmd.Env, "BACKEND=http://localhost:9080")
}

func TestRebuildObservabilityRegeneratesRuntimePorts(t *testing.T) {
	runtime := newOrchestratorTestRuntime(t, "observability-runtime", 1)
	orch, err := NewOrchestratorWithRuntime(logrus.New(), runtime)
	require.NoError(t, err)

	require.NoError(t, orch.regenerateObservabilityConfig(constants.ServicePrometheus))
	require.NoError(t, orch.regenerateObservabilityConfig(constants.ServiceGrafana))

	prometheusConfig, err := os.ReadFile(filepath.Join(runtime.Manifest.StateDir, constants.DirConfigs, "prometheus.yml"))
	require.NoError(t, err)
	require.Contains(t, string(prometheusConfig), fmt.Sprintf("host.docker.internal:%d", runtime.Ports.LabBackend))
	require.Contains(t, string(prometheusConfig), fmt.Sprintf("host.docker.internal:%d", runtime.Ports.Networks["mainnet"].CBTMetrics))
	require.NotContains(t, string(prometheusConfig), "host.docker.internal:8080")
	require.NotContains(t, string(prometheusConfig), "host.docker.internal:9100")

	datasourceConfig, err := os.ReadFile(filepath.Join(
		runtime.Manifest.StateDir,
		constants.DirConfigs,
		"grafana",
		"provisioning",
		"datasources",
		"datasource.yml",
	))
	require.NoError(t, err)
	require.Contains(t, string(datasourceConfig), fmt.Sprintf("host.docker.internal:%d", runtime.Ports.Prometheus))
	require.NotContains(t, string(datasourceConfig), "host.docker.internal:9090")
}

func testRuntimeLabConfig() *config.LabConfig {
	return &config.LabConfig{
		Mode: constants.ModeHybrid,
		Repos: config.LabReposConfig{
			CBT:        "/tmp/cbt",
			XatuCBT:    "/tmp/xatu-cbt",
			CBTAPI:     "/tmp/cbt-api",
			LabBackend: "/tmp/lab-backend",
			Lab:        "/tmp/lab",
		},
		Networks: []config.NetworkConfig{
			{Name: "mainnet", Enabled: true},
		},
		Infrastructure: config.InfrastructureConfig{
			ClickHouse: config.ClickHouseConfig{
				Xatu: config.ClickHouseClusterConfig{
					Mode:        constants.InfraModeExternal,
					ExternalURL: "http://example.invalid:9000",
				},
				CBT: config.ClickHouseClusterConfig{Mode: constants.InfraModeLocal},
			},
			Observability: config.ObservabilityConfig{
				Enabled:        true,
				PrometheusPort: 9090,
				GrafanaPort:    3000,
			},
			ClickHouseCBTPort: 8123,
			RedisPort:         6380,
		},
		Ports: config.LabPortsConfig{
			LabBackend:      8080,
			LabFrontend:     5173,
			CBTBase:         8081,
			CBTAPIBase:      8091,
			CBTFrontendBase: 8085,
		},
	}
}

func newOrchestratorTestRuntime(t *testing.T, instanceID string, slot int) *instance.Runtime {
	t.Helper()

	rootDir := t.TempDir()
	reposDir := filepath.Join(rootDir, "repos")
	cfg := testRuntimeLabConfig()
	cfg.Repos = config.LabReposConfig{
		CBT:        filepath.Join(reposDir, constants.RepoCBT),
		XatuCBT:    filepath.Join(reposDir, constants.RepoXatuCBT),
		CBTAPI:     filepath.Join(reposDir, constants.RepoCBTAPI),
		LabBackend: filepath.Join(reposDir, constants.RepoLabBackend),
		Lab:        filepath.Join(reposDir, constants.RepoLab),
	}

	for _, path := range []string{
		cfg.Repos.CBT,
		cfg.Repos.XatuCBT,
		cfg.Repos.CBTAPI,
		cfg.Repos.LabBackend,
		cfg.Repos.Lab,
	} {
		initTestGitRepo(t, path)
	}

	configPath := filepath.Join(rootDir, config.DefaultConfigFileName)
	require.NoError(t, os.WriteFile(configPath, []byte("lab: {}\n"), 0644))

	ws := &workspace.Workspace{
		RootDir:       rootDir,
		ConfigPath:    configPath,
		OverridesPath: filepath.Join(rootDir, constants.CBTOverridesFile),
		StateDir:      filepath.Join(rootDir, ".xcli"),
	}

	manifest, err := instance.NewManifest(context.Background(), ws, cfg, instanceID)
	require.NoError(t, err)

	ports, err := instance.BuildPortPlan(cfg, slot)
	require.NoError(t, err)

	dockerPlan := instance.NewDockerPlan(manifest.InstanceID, manifest.ConfigPath)
	manifest.Ports = ports
	manifest.Docker = dockerPlan

	return &instance.Runtime{
		Workspace:  ws,
		LabConfig:  cfg,
		Registry:   instance.NewRegistry(filepath.Join(t.TempDir(), "instances")),
		InstanceID: manifest.InstanceID,
		Manifest:   manifest,
		Ports:      ports,
		Docker:     dockerPlan,
		Repos:      manifest.Repos,
	}
}

func initTestGitRepo(t *testing.T, path string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(path, 0755))
	runGit(t, path, "init")
	runGit(t, path, "config", "commit.gpgsign", "false")
	runGit(t, path, "config", "user.email", "xcli-test@example.invalid")
	runGit(t, path, "config", "user.name", "xcli test")
	require.NoError(t, os.WriteFile(filepath.Join(path, "README.md"), []byte("# test\n"), 0644))
	runGit(t, path, "add", "README.md")
	runGit(t, path, "commit", "-m", "initial")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

type fakeProcessManager struct {
	processes []*process.Process
	lastName  string
	lastCmd   *exec.Cmd
}

func (f *fakeProcessManager) Start(
	_ context.Context,
	name string,
	cmd *exec.Cmd,
	_ process.HealthChecker,
) error {
	f.lastName = name
	f.lastCmd = cmd
	f.processes = append(f.processes, &process.Process{
		Name:    name,
		Cmd:     cmd,
		PID:     9999,
		Started: time.Now(),
	})

	return nil
}

func (f *fakeProcessManager) Stop(context.Context, string) error { return nil }
func (f *fakeProcessManager) StopAll(context.Context) error      { return nil }
func (f *fakeProcessManager) Restart(context.Context, string) error {
	return nil
}
func (f *fakeProcessManager) List() []*process.Process { return f.processes }
func (f *fakeProcessManager) Get(name string) (*process.Process, bool) {
	for _, proc := range f.processes {
		if proc.Name == name {
			return proc, true
		}
	}

	return nil, false
}
func (f *fakeProcessManager) IsRunning(name string) bool {
	_, ok := f.Get(name)

	return ok
}
func (f *fakeProcessManager) ReloadPIDs()                                  {}
func (f *fakeProcessManager) TailLogs(context.Context, string, bool) error { return nil }
func (f *fakeProcessManager) CleanLogs() error                             { return nil }
