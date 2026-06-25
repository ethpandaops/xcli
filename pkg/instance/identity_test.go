package instance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/workspace"
	"github.com/stretchr/testify/require"
)

const testConfigPath = "/tmp/.xcli.yaml"

func TestResolveIDDerivesDeterministicConfigPathID(t *testing.T) {
	rootDir := t.TempDir()
	configPath := filepath.Join(rootDir, config.DefaultConfigFileName)
	ws := &workspace.Workspace{RootDir: rootDir, ConfigPath: configPath}

	got, err := ResolveID(ws, &config.LabConfig{}, "")
	require.NoError(t, err)

	sum := sha256.Sum256([]byte(configPath))
	want := fmt.Sprintf("%s-%s", filepath.Base(rootDir), hex.EncodeToString(sum[:])[:idHashLength])
	require.Equal(t, want, got)
	require.Len(t, strings.TrimPrefix(got, filepath.Base(rootDir)+"-"), 16)

	gotAgain, err := ResolveID(ws, &config.LabConfig{}, "")
	require.NoError(t, err)
	require.Equal(t, got, gotAgain)
}

func TestResolveIDConfigOverride(t *testing.T) {
	ws := &workspace.Workspace{RootDir: t.TempDir(), ConfigPath: testConfigPath}
	labCfg := &config.LabConfig{
		Instance: config.LabInstanceConfig{ID: "My Lab.Instance!"},
	}

	got, err := ResolveID(ws, labCfg, "")
	require.NoError(t, err)
	require.Equal(t, "my-lab-instance", got)
}

func TestResolveIDCLIOverrideWins(t *testing.T) {
	ws := &workspace.Workspace{RootDir: t.TempDir(), ConfigPath: testConfigPath}
	labCfg := &config.LabConfig{
		Instance: config.LabInstanceConfig{ID: "from-config"},
	}

	got, err := ResolveID(ws, labCfg, "from-cli")
	require.NoError(t, err)
	require.Equal(t, "from-cli", got)
}

func TestResolveIDRejectsEmptySanitizedOverride(t *testing.T) {
	ws := &workspace.Workspace{RootDir: t.TempDir(), ConfigPath: testConfigPath}

	_, err := ResolveID(ws, &config.LabConfig{}, "!!!")
	require.Error(t, err)
	require.Contains(t, err.Error(), "instance id is empty")
}
