package instance

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReconcilerMarksStaleAndDiscoversDockerOrphans(t *testing.T) {
	rootDir := t.TempDir()
	registry := NewRegistry(filepath.Join(t.TempDir(), "instances"))

	running := &Manifest{
		SchemaVersion: SchemaVersion,
		InstanceID:    "dead",
		Status:        StatusRunning,
		RootDir:       rootDir,
		ConfigPath:    filepath.Join(rootDir, ".xcli.yaml"),
		StateDir:      InstanceStateDir(rootDir, "dead"),
		Docker:        NewDockerPlan("dead", filepath.Join(rootDir, ".xcli.yaml")),
		PIDs:          map[string]int{"lab-backend": 999999},
		Ports: PortPlan{
			LabBackend:  18080,
			LabFrontend: 15173,
		},
		URLs: map[string]string{},
	}
	require.NoError(t, registry.Save(running))

	reconciler := &Reconciler{
		Registry: registry,
		Docker: fakeDockerResources{resources: []DockerResource{
			orphanDockerResource("orphan"),
			composeProjectDockerResource("composeonly"),
		}},
		PIDAlive:  func(int) bool { return false },
		PortBound: func(int) bool { return false },
	}

	result, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)
	require.NoError(t, result.DockerError)
	require.Len(t, result.Instances, 3)

	byID := map[string]*ReconciledInstance{}
	for _, item := range result.Instances {
		byID[item.InstanceID] = item
	}

	require.Equal(t, StatusStale, byID["dead"].Status)
	require.True(t, byID["dead"].RegistryPresent)
	require.False(t, byID["dead"].Live.BackingExists)

	require.Equal(t, StatusOrphan, byID["orphan"].Status)
	require.False(t, byID["orphan"].RegistryPresent)
	require.True(t, byID["orphan"].Orphan)
	require.True(t, byID["orphan"].Live.BackingExists)
	require.Len(t, byID["orphan"].Live.DockerResources, 1)

	require.Equal(t, StatusOrphan, byID["composeonly"].Status)
	require.False(t, byID["composeonly"].RegistryPresent)
	require.True(t, byID["composeonly"].Orphan)
	require.True(t, byID["composeonly"].Live.BackingExists)
	require.Len(t, byID["composeonly"].Live.DockerResources, 1)
}

func TestReconcilerMarksReservedWithoutBackingStale(t *testing.T) {
	rootDir := t.TempDir()
	registry := NewRegistry(filepath.Join(t.TempDir(), "instances"))

	reserved := &Manifest{
		SchemaVersion: SchemaVersion,
		InstanceID:    "reserved",
		Status:        StatusReserved,
		RootDir:       rootDir,
		ConfigPath:    filepath.Join(rootDir, ".xcli.yaml"),
		StateDir:      InstanceStateDir(rootDir, "reserved"),
		Docker:        NewDockerPlan("reserved", filepath.Join(rootDir, ".xcli.yaml")),
		PIDs:          map[string]int{},
		Ports:         PortPlan{LabBackend: 18080},
		URLs:          map[string]string{},
	}
	require.NoError(t, registry.Save(reserved))

	reconciler := &Reconciler{
		Registry:  registry,
		Docker:    fakeDockerResources{},
		PIDAlive:  func(int) bool { return false },
		PortBound: func(int) bool { return false },
	}

	result, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)
	require.Len(t, result.Instances, 1)
	require.Equal(t, StatusStale, result.Instances[0].Status)
}

type fakeDockerResources struct {
	resources []DockerResource
}

func (f fakeDockerResources) ListXCLIResources(context.Context) ([]DockerResource, error) {
	return f.resources, nil
}

func orphanDockerResource(instanceID string) DockerResource {
	return DockerResource{
		Kind:  "container",
		ID:    "container-id",
		Name:  "xcli-" + instanceID + "-prometheus",
		State: "running",
		Labels: map[string]string{
			DockerLabelInstance: instanceID,
			DockerLabelConfig:   filepath.Join("/tmp", instanceID, ".xcli.yaml"),
		},
	}
}

func composeProjectDockerResource(instanceID string) DockerResource {
	return DockerResource{
		Kind:  "container",
		ID:    "compose-container-id",
		Name:  "xcli-" + instanceID + "-redis-1",
		State: "running",
		Labels: map[string]string{
			DockerComposeProjectLabel: "xcli-" + instanceID,
		},
	}
}
