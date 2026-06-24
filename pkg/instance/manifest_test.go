package instance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/workspace"
	"github.com/stretchr/testify/require"
)

func TestNewManifestCapturesWorkspaceReposAndDockerPlan(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	rootDir := t.TempDir()
	configPath := filepath.Join(rootDir, config.DefaultConfigFileName)
	repoDir := initTestRepo(t)

	ws := &workspace.Workspace{
		RootDir:       rootDir,
		ConfigPath:    configPath,
		OverridesPath: filepath.Join(rootDir, constants.CBTOverridesFile),
		StateDir:      filepath.Join(rootDir, ".xcli"),
		ConfigExists:  true,
	}
	labCfg := &config.LabConfig{
		Instance: config.LabInstanceConfig{ID: "Example One"},
		Mode:     constants.ModeHybrid,
		Repos: config.LabReposConfig{
			CBT:        repoDir,
			XatuCBT:    repoDir,
			CBTAPI:     repoDir,
			LabBackend: repoDir,
			Lab:        repoDir,
		},
	}

	manifest, err := NewManifest(context.Background(), ws, labCfg, "")
	require.NoError(t, err)

	require.Equal(t, SchemaVersion, manifest.SchemaVersion)
	require.Equal(t, "example-one", manifest.InstanceID)
	require.Equal(t, StatusCreated, manifest.Status)
	require.Equal(t, rootDir, manifest.RootDir)
	require.Equal(t, configPath, manifest.ConfigPath)
	require.Equal(t, InstanceStateDir(rootDir, "example-one"), manifest.StateDir)
	require.Equal(t, constants.ModeHybrid, manifest.Mode)
	require.Equal(t, "xcli-example-one", manifest.Docker.ProjectName)
	require.Equal(t, "example-one", manifest.Docker.Labels["com.ethpandaops.xcli.instance"])
	require.NotEmpty(t, manifest.Repos[constants.RepoCBT].Commit)
	require.False(t, manifest.CreatedAt.IsZero())
	require.False(t, manifest.UpdatedAt.IsZero())
	require.Empty(t, manifest.LastError)
}

func initTestRepo(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	runInstanceGit(t, repoDir, "init")
	runInstanceGit(t, repoDir, "config", "user.email", "test@example.com")
	runInstanceGit(t, repoDir, "config", "user.name", "Test User")
	runInstanceGit(t, repoDir, "config", "commit.gpgsign", "false")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0644))
	runInstanceGit(t, repoDir, "add", "README.md")
	runInstanceGit(t, repoDir, "commit", "-m", "initial")

	return repoDir
}

func runInstanceGit(t *testing.T, repoDir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(out)))
}
