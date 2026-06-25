package instance

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	xcligit "github.com/ethpandaops/xcli/pkg/git"
	"github.com/stretchr/testify/require"
)

func TestRegistrySaveWritesGlobalAndLocalManifest(t *testing.T) {
	rootDir := t.TempDir()
	registry := NewRegistry(filepath.Join(t.TempDir(), "instances"))

	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		InstanceID:    "alpha",
		Status:        StatusCreated,
		RootDir:       rootDir,
		ConfigPath:    filepath.Join(rootDir, ".xcli.yaml"),
		OverridesPath: filepath.Join(rootDir, ".cbt-overrides.yaml"),
		StateDir:      InstanceStateDir(rootDir, "alpha"),
		Mode:          "hybrid",
		Repos: map[string]xcligit.RepoVersion{
			"cbt": {Path: "/repos/cbt", Branch: "main", Commit: "abcdef", Dirty: true},
		},
		Ports:     PortPlan{},
		Docker:    NewDockerPlan("alpha", filepath.Join(rootDir, ".xcli.yaml")),
		PIDs:      map[string]int{},
		URLs:      map[string]string{},
		CreatedAt: time.Now().UTC().Add(-time.Minute),
	}

	require.NoError(t, registry.Save(manifest))

	globalPath := registry.ManifestPath("alpha")
	localPath := LocalManifestPath(rootDir, "alpha")

	require.FileExists(t, globalPath)
	require.FileExists(t, localPath)

	globalData, err := os.ReadFile(globalPath)
	require.NoError(t, err)

	localData, err := os.ReadFile(localPath)
	require.NoError(t, err)
	require.Equal(t, string(globalData), string(localData))

	loaded, err := registry.Load("alpha")
	require.NoError(t, err)
	require.Equal(t, "alpha", loaded.InstanceID)
	require.Equal(t, "main", loaded.Repos["cbt"].Branch)
	require.False(t, loaded.UpdatedAt.IsZero())
}

func TestRegistryLoadAllSorted(t *testing.T) {
	rootDir := t.TempDir()
	registry := NewRegistry(filepath.Join(t.TempDir(), "instances"))

	for _, id := range []string{"bravo", "alpha"} {
		require.NoError(t, registry.Save(&Manifest{
			SchemaVersion: SchemaVersion,
			InstanceID:    id,
			Status:        StatusCreated,
			RootDir:       rootDir,
			ConfigPath:    filepath.Join(rootDir, ".xcli.yaml"),
			OverridesPath: filepath.Join(rootDir, ".cbt-overrides.yaml"),
			StateDir:      InstanceStateDir(rootDir, id),
			Repos:         map[string]xcligit.RepoVersion{},
			Docker:        NewDockerPlan(id, filepath.Join(rootDir, ".xcli.yaml")),
			PIDs:          map[string]int{},
			URLs:          map[string]string{},
		}))
	}

	manifests, err := registry.LoadAll()
	require.NoError(t, err)
	require.Len(t, manifests, 2)
	require.Equal(t, "alpha", manifests[0].InstanceID)
	require.Equal(t, "bravo", manifests[1].InstanceID)
}

func TestRegistryDeleteRemovesGlobalAndLocalManifest(t *testing.T) {
	rootDir := t.TempDir()
	registry := NewRegistry(filepath.Join(t.TempDir(), "instances"))
	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		InstanceID:    "delete-me",
		Status:        StatusStopped,
		RootDir:       rootDir,
		ConfigPath:    filepath.Join(rootDir, ".xcli.yaml"),
		OverridesPath: filepath.Join(rootDir, ".cbt-overrides.yaml"),
		StateDir:      InstanceStateDir(rootDir, "delete-me"),
		Repos:         map[string]xcligit.RepoVersion{},
		Docker:        NewDockerPlan("delete-me", filepath.Join(rootDir, ".xcli.yaml")),
		PIDs:          map[string]int{},
		URLs:          map[string]string{},
	}

	require.NoError(t, registry.Save(manifest))
	require.FileExists(t, registry.ManifestPath("delete-me"))
	require.FileExists(t, LocalManifestPath(rootDir, "delete-me"))

	require.NoError(t, registry.Delete(manifest))
	require.NoFileExists(t, registry.ManifestPath("delete-me"))
	require.NoFileExists(t, LocalManifestPath(rootDir, "delete-me"))
}

func TestRegistryRejectsSameIDWithDifferentConfigPath(t *testing.T) {
	rootDir := t.TempDir()
	registry := NewRegistry(filepath.Join(t.TempDir(), "instances"))

	configOne := filepath.Join(rootDir, "one", ".xcli.yaml")
	configTwo := filepath.Join(rootDir, "two", ".xcli.yaml")
	first := &Manifest{
		SchemaVersion: SchemaVersion,
		InstanceID:    "same-id",
		Status:        StatusStopped,
		RootDir:       rootDir,
		ConfigPath:    configOne,
		StateDir:      InstanceStateDir(rootDir, "same-id"),
		Repos:         map[string]xcligit.RepoVersion{},
		Docker:        NewDockerPlan("same-id", configOne),
		PIDs:          map[string]int{},
		URLs:          map[string]string{},
	}
	second := *first
	second.ConfigPath = configTwo
	second.Docker = NewDockerPlan("same-id", configTwo)

	require.NoError(t, registry.Save(first))
	err := registry.Save(&second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already registered")
}

func TestDefaultRegistryDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	got, err := DefaultRegistryDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(homeDir, ".xcli", "lab", "instances"), got)
}
