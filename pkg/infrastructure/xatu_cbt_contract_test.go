package infrastructure

import (
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/ethpandaops/xcli/pkg/mode"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestXatuCBTInfraCommandsUseInstanceProjectAndPortEnv(t *testing.T) {
	labCfg := xatuCBTContractConfig(t)
	slot0, err := instance.BuildPortPlan(labCfg, 0)
	require.NoError(t, err)
	slot1, err := instance.BuildPortPlan(labCfg, 1)
	require.NoError(t, err)

	require.Equal(t, 8123, slot0.ClickHouseCBT01HTTP)
	require.Equal(t, 9000, slot0.ClickHouseCBT01TCP)
	require.Equal(t, 8124, slot0.ClickHouseCBT02HTTP)
	require.Equal(t, 9001, slot0.ClickHouseCBT02TCP)
	require.Equal(t, 8125, slot0.ClickHouseXatu01HTTP)
	require.Equal(t, 9002, slot0.ClickHouseXatu01TCP)
	require.Equal(t, 8126, slot0.ClickHouseXatu02HTTP)
	require.Equal(t, 9003, slot0.ClickHouseXatu02TCP)
	require.Equal(t, 6380, slot0.Redis)

	for _, tt := range []struct {
		name       string
		instanceID string
		ports      instance.PortPlan
	}{
		{name: "slot 0 defaults", instanceID: "alpha", ports: slot0},
		{name: "slot 1 stride", instanceID: "beta", ports: slot1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			runtime := xatuCBTContractRuntime(t, labCfg, tt.instanceID, tt.ports)
			manager := NewManagerWithRuntime(logrus.New(), labCfg, nil, runtime.Manifest.StateDir, runtime)
			var captured *exec.Cmd
			manager.runCmd = func(cmd *exec.Cmd, _ bool) error {
				captured = cmd
				return nil
			}

			commands := map[string][]string{
				"start":   manager.infraStartArgs(constants.InfraModeLocal),
				"stop":    manager.infraActionArgs("stop"),
				"reset":   manager.infraActionArgs("reset"),
				"migrate": manager.infraActionArgs("migrate-xatu"),
			}

			for commandName, args := range commands {
				captured = nil
				require.NoError(t, manager.runXatuCBT(context.Background(), args...))
				require.NotNil(t, captured, commandName)

				require.Contains(t, captured.Args, "--project-name", commandName)
				requireArgValue(t, captured.Args, "--project-name", "xcli-"+tt.instanceID)
				require.Equal(t, labCfg.Repos.XatuCBT, captured.Dir)
				requirePortEnv(t, captured.Env, tt.ports)
			}
		})
	}
}

func TestXatuCBTRunHelperHandsScopedCommandToRunner(t *testing.T) {
	labCfg := xatuCBTContractConfig(t)
	ports, err := instance.BuildPortPlan(labCfg, 1)
	require.NoError(t, err)

	runtime := xatuCBTContractRuntime(t, labCfg, "runner", ports)
	manager := NewManagerWithRuntime(logrus.New(), labCfg, nil, runtime.Manifest.StateDir, runtime)

	var captured *exec.Cmd
	manager.runCmd = func(cmd *exec.Cmd, _ bool) error {
		captured = cmd
		return nil
	}

	require.NoError(t, manager.runXatuCBT(context.Background(), manager.infraActionArgs("migrate-xatu")...))
	require.NotNil(t, captured)
	requireArgValue(t, captured.Args, "--project-name", "xcli-runner")
	requirePortEnv(t, captured.Env, ports)
}

func TestSafeStopDestroyAndRedisResetUseSeparateCommands(t *testing.T) {
	labCfg := xatuCBTContractConfig(t)
	labCfg.Infrastructure.Observability.Enabled = false
	ports, err := instance.BuildPortPlan(labCfg, 1)
	require.NoError(t, err)

	runtime := xatuCBTContractRuntime(t, labCfg, "safety", ports)
	stackMode, err := mode.NewMode(&config.Config{Lab: labCfg})
	require.NoError(t, err)

	manager := NewManagerWithRuntime(logrus.New(), labCfg, stackMode, runtime.Manifest.StateDir, runtime)
	var captured []*exec.Cmd
	manager.runCmd = func(cmd *exec.Cmd, _ bool) error {
		captured = append(captured, cmd)
		return nil
	}

	require.NoError(t, manager.Stop(context.Background()))
	require.Len(t, captured, 1)
	require.Equal(t, []string{
		manager.xatuCBTPath,
		"infra",
		"stop",
		"--project-name",
		"xcli-safety",
	}, captured[0].Args)
	require.NotContains(t, strings.Join(captured[0].Args, " "), "reset")
	require.NotContains(t, strings.Join(captured[0].Args, " "), "FLUSHALL")

	captured = nil
	require.NoError(t, manager.Destroy(context.Background()))
	require.Len(t, captured, 1)
	require.Equal(t, []string{
		manager.xatuCBTPath,
		"infra",
		"reset",
		"--project-name",
		"xcli-safety",
	}, captured[0].Args)

	captured = nil
	require.NoError(t, manager.ResetRedis(context.Background()))
	require.Len(t, captured, 1)
	require.Equal(t, []string{
		"redis-cli",
		"-h",
		"127.0.0.1",
		"-p",
		strconv.Itoa(ports.Redis),
		"FLUSHALL",
	}, captured[0].Args)
}

func TestBoundsSeederUsesRuntimeRedisPort(t *testing.T) {
	labCfg := xatuCBTContractConfig(t)
	ports, err := instance.BuildPortPlan(labCfg, 2)
	require.NoError(t, err)

	runtime := xatuCBTContractRuntime(t, labCfg, "bounds", ports)
	seeder := NewBoundsSeederWithRuntime(logrus.New(), runtime)

	require.Equal(t, []string{
		"-h",
		"127.0.0.1",
		"-p",
		strconv.Itoa(ports.Redis),
		"-n",
		"3",
		"KEYS",
		"cbt:external:*",
	}, seeder.localRedisArgs(3, "KEYS", "cbt:external:*"))
}

func xatuCBTContractConfig(t *testing.T) *config.LabConfig {
	t.Helper()

	root := t.TempDir()
	cfg := config.DefaultLab()
	cfg.Mode = constants.ModeLocal
	cfg.Repos = config.LabReposConfig{
		CBT:        filepath.Join(root, constants.RepoCBT),
		XatuCBT:    filepath.Join(root, constants.RepoXatuCBT),
		CBTAPI:     filepath.Join(root, constants.RepoCBTAPI),
		LabBackend: filepath.Join(root, constants.RepoLabBackend),
		Lab:        filepath.Join(root, constants.RepoLab),
	}
	cfg.Infrastructure.ClickHouse.Xatu.Mode = constants.InfraModeLocal
	cfg.Infrastructure.ClickHouse.Xatu.ExternalURL = ""

	return cfg
}

func xatuCBTContractRuntime(
	t *testing.T,
	labCfg *config.LabConfig,
	instanceID string,
	ports instance.PortPlan,
) *instance.Runtime {
	t.Helper()

	root := t.TempDir()
	configPath := filepath.Join(root, config.DefaultConfigFileName)
	docker := instance.NewDockerPlan(instanceID, configPath)

	return &instance.Runtime{
		LabConfig:  labCfg,
		InstanceID: instanceID,
		Ports:      ports,
		Docker:     docker,
		Manifest: &instance.Manifest{
			InstanceID: instanceID,
			ConfigPath: configPath,
			StateDir:   instance.InstanceStateDir(root, instanceID),
			Ports:      ports,
			Docker:     docker,
		},
	}
}

func requireArgValue(t *testing.T, args []string, flag string, want string) {
	t.Helper()

	index := slices.Index(args, flag)
	require.NotEqual(t, -1, index, "missing %s in %v", flag, args)
	require.Greater(t, len(args), index+1)
	require.Equal(t, want, args[index+1])
}

func requirePortEnv(t *testing.T, env []string, ports instance.PortPlan) {
	t.Helper()

	values := envMap(env)
	require.Equal(t, strconv.Itoa(ports.ClickHouseCBT01HTTP), values["CLICKHOUSE_CBT_01_HTTP_PORT"])
	require.Equal(t, strconv.Itoa(ports.ClickHouseCBT01TCP), values["CLICKHOUSE_CBT_01_NATIVE_PORT"])
	require.Equal(t, strconv.Itoa(ports.ClickHouseCBT02HTTP), values["CLICKHOUSE_CBT_02_HTTP_PORT"])
	require.Equal(t, strconv.Itoa(ports.ClickHouseCBT02TCP), values["CLICKHOUSE_CBT_02_NATIVE_PORT"])
	require.Equal(t, strconv.Itoa(ports.ClickHouseXatu01HTTP), values["CLICKHOUSE_XATU_01_HTTP_PORT"])
	require.Equal(t, strconv.Itoa(ports.ClickHouseXatu01TCP), values["CLICKHOUSE_XATU_01_NATIVE_PORT"])
	require.Equal(t, strconv.Itoa(ports.ClickHouseXatu02HTTP), values["CLICKHOUSE_XATU_02_HTTP_PORT"])
	require.Equal(t, strconv.Itoa(ports.ClickHouseXatu02TCP), values["CLICKHOUSE_XATU_02_NATIVE_PORT"])
	require.Equal(t, strconv.Itoa(ports.Redis), values["REDIS_PORT"])

	for _, name := range instance.XatuCBTBindAddressVars() {
		require.Equal(t, "127.0.0.1", values[name])
	}
}

func envMap(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = value
		}
	}

	return values
}
