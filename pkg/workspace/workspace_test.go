package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/stretchr/testify/require"
)

func TestResolveDefaultSearchesUpwardOnly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	origWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origWD))
	})

	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.MkdirAll(child, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(parent, config.DefaultConfigFileName), []byte("lab: {}\n"), 0644))
	require.NoError(t, os.Chdir(child))

	ws, err := Resolve(config.DefaultConfigFileName, true, false)
	require.NoError(t, err)
	require.True(t, samePath(filepath.Join(parent, config.DefaultConfigFileName), ws.ConfigPath))
	require.True(t, samePath(parent, ws.RootDir))
}

func TestResolveDefaultDoesNotSearchChildrenOrGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	origWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origWD))
	})

	cwd := t.TempDir()
	child := filepath.Join(cwd, "child")
	require.NoError(t, os.MkdirAll(child, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(child, config.DefaultConfigFileName), []byte("lab: {}\n"), 0644))

	globalRoot := filepath.Join(home, "registered")
	require.NoError(t, os.MkdirAll(globalRoot, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(globalRoot, config.DefaultConfigFileName), []byte("lab: {}\n"), 0644))
	require.NoError(t, config.SaveGlobalConfig(&config.GlobalConfig{XCLIPath: globalRoot}))

	require.NoError(t, os.Chdir(cwd))

	_, err = Resolve(config.DefaultConfigFileName, true, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no .xcli.yaml found searching upward")
}

func TestResolveExplicitConfigWins(t *testing.T) {
	origWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origWD))
	})

	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	require.NoError(t, os.MkdirAll(nested, 0755))

	defaultConfig := filepath.Join(root, config.DefaultConfigFileName)
	explicitConfig := filepath.Join(root, "custom.yaml")

	require.NoError(t, os.WriteFile(defaultConfig, []byte("lab: {}\n"), 0644))
	require.NoError(t, os.WriteFile(explicitConfig, []byte("lab: {}\n"), 0644))
	require.NoError(t, os.Chdir(nested))

	ws, err := Resolve(explicitConfig, true, false)
	require.NoError(t, err)
	require.Equal(t, explicitConfig, ws.ConfigPath)
}

func TestResolveExplicitMissingConfigReportsPath(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.yaml")

	_, err := Resolve(missingPath, true, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "config file not found: "+missingPath)
}

func TestLoadLabConfigResolvesRepoPathsRelativeToConfig(t *testing.T) {
	origWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origWD))
	})

	root := t.TempDir()
	other := t.TempDir()
	require.NoError(t, os.Chdir(other))

	configPath := filepath.Join(root, config.DefaultConfigFileName)
	require.NoError(t, os.WriteFile(configPath, []byte(`
lab:
  repos:
    cbt: ../repos/cbt
    xatuCbt: ../repos/xatu-cbt
    cbtApi: ../repos/cbt-api
    labBackend: ../repos/lab-backend
    lab: /abs/lab
  mode: local
  networks:
    - name: mainnet
      enabled: true
`), 0644))

	labCfg, _, err := LoadLabConfig(configPath, false)
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(filepath.Join(root, "../repos/cbt")), labCfg.Repos.CBT)
	require.Equal(t, filepath.Clean(filepath.Join(root, "../repos/xatu-cbt")), labCfg.Repos.XatuCBT)
	require.Equal(t, filepath.Clean(filepath.Join(root, "../repos/cbt-api")), labCfg.Repos.CBTAPI)
	require.Equal(t, filepath.Clean(filepath.Join(root, "../repos/lab-backend")), labCfg.Repos.LabBackend)
	require.Equal(t, filepath.Clean("/abs/lab"), labCfg.Repos.Lab)
}

func TestResolveRejectsDifferentCWDOverrides(t *testing.T) {
	origWD, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(origWD))
	})

	root := t.TempDir()
	child := filepath.Join(root, "child")
	require.NoError(t, os.MkdirAll(child, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, config.DefaultConfigFileName), []byte("lab: {}\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(child, constants.CBTOverridesFile), []byte("models: {}\n"), 0644))
	require.NoError(t, os.Chdir(child))

	_, err = Resolve(config.DefaultConfigFileName, true, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refusing to continue")
	require.Contains(t, err.Error(), filepath.Join(root, constants.CBTOverridesFile))
}
