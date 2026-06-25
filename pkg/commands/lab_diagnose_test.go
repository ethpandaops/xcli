package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/instance"
	"github.com/stretchr/testify/require"
)

func TestBuildLabDiagnosticContextSurfacesInstancePathsPortsAndTraps(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	rootDir := t.TempDir()
	configPath := filepath.Join(rootDir, config.DefaultConfigFileName)
	require.NoError(t, os.WriteFile(configPath, []byte(`
lab:
  mode: local
  repos:
    cbt: missing/cbt
    xatuCbt: missing/xatu-cbt
    cbtApi: missing/cbt-api
    labBackend: missing/lab-backend
    lab: missing/lab
  networks:
    - name: mainnet
      enabled: true
      portOffset: 0
  infrastructure:
    clickhouse:
      xatu:
        mode: local
      cbt:
        mode: local
    redis:
      port: 6380
  ports:
    labBackend: 8080
    labFrontend: 5173
    cbtBase: 8081
    cbtApiBase: 8091
    cbtFrontendBase: 8085
`), 0644))

	diag, err := buildLabDiagnosticContext(context.Background(), configPath, "diag-one")
	require.NoError(t, err)

	require.Equal(t, "diag-one", diag.Runtime.InstanceID)
	require.Equal(t, configPath, diag.Runtime.Manifest.ConfigPath)
	require.Equal(t, instance.InstanceStateDir(rootDir, "diag-one"), diag.Runtime.Manifest.StateDir)
	require.Equal(t, 8080, diag.Runtime.Ports.LabBackend)
	require.False(t, diag.ManifestLoaded)

	traps := strings.Join(diag.Traps, "\n")
	require.Contains(t, traps, "No persisted manifest")
	require.Contains(t, traps, "Repo cbt path does not exist")
}
