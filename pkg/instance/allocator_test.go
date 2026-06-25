package instance

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
	xcligit "github.com/ethpandaops/xcli/pkg/git"
	"github.com/stretchr/testify/require"
)

func TestAllocatorClaimsDistinctNonOverlappingSlots(t *testing.T) {
	registry := NewRegistry(filepath.Join(t.TempDir(), "lab", "instances"))
	allocator := NewAllocator(registry, false)
	labCfg := config.DefaultLab()

	first := testManifest(t, "first")
	firstPlan, err := allocator.Allocate(context.Background(), AllocationRequest{
		InstanceID: first.InstanceID,
		LabConfig:  labCfg,
		Manifest:   first,
		Claim:      true,
	})
	require.NoError(t, err)
	require.Equal(t, 0, firstPlan.Slot)

	second := testManifest(t, "second")
	secondPlan, err := allocator.Allocate(context.Background(), AllocationRequest{
		InstanceID: second.InstanceID,
		LabConfig:  labCfg,
		Manifest:   second,
		Claim:      true,
	})
	require.NoError(t, err)
	require.Equal(t, 1, secondPlan.Slot)
	require.Empty(t, firstPlan.Overlaps(secondPlan))

	loaded, err := registry.Load("first")
	require.NoError(t, err)
	require.Equal(t, StatusReserved, loaded.Status)
	require.Equal(t, firstPlan, loaded.Ports)
}

func TestAllocatorStoppedManifestDoesNotHogSlot(t *testing.T) {
	registry := NewRegistry(filepath.Join(t.TempDir(), "lab", "instances"))
	allocator := NewAllocator(registry, false)
	labCfg := config.DefaultLab()

	stopped := testManifest(t, "stopped")
	stoppedPlan, err := BuildPortPlan(labCfg, 0)
	require.NoError(t, err)

	stopped.Ports = stoppedPlan
	stopped.Status = StatusStopped
	require.NoError(t, registry.Save(stopped))

	next := testManifest(t, "next")
	nextPlan, err := allocator.Allocate(context.Background(), AllocationRequest{
		InstanceID: next.InstanceID,
		LabConfig:  labCfg,
		Manifest:   next,
		Claim:      true,
	})
	require.NoError(t, err)
	require.Equal(t, 0, nextPlan.Slot)
}

func TestAllocatorOldReservedManifestDoesNotHogSlot(t *testing.T) {
	registry := NewRegistry(filepath.Join(t.TempDir(), "lab", "instances"))
	allocator := NewAllocator(registry, false)
	labCfg := config.DefaultLab()

	reserved := testManifest(t, "reserved")
	reserved.Ports = PortPlan{LabBackend: 35080}
	reserved.Status = StatusReserved
	reserved.UpdatedAt = time.Now().Add(-reservationGracePeriod - time.Minute)
	require.NoError(t, writeJSONAtomic(registry.ManifestPath(reserved.InstanceID), reserved))

	next := testManifest(t, "next")
	nextPlan, err := allocator.Allocate(context.Background(), AllocationRequest{
		InstanceID: next.InstanceID,
		LabConfig:  labCfg,
		Manifest:   next,
		Claim:      true,
	})
	require.NoError(t, err)
	require.Equal(t, 0, nextPlan.Slot)
}

func TestAllocatorSkipsBoundPorts(t *testing.T) {
	port, closeListener := listenOnFreePort(t, 18080, 19080)
	if port == 0 {
		t.Skip("localhost bind is not permitted in this environment")
	}
	defer closeListener()

	registry := NewRegistry(filepath.Join(t.TempDir(), "lab", "instances"))
	allocator := NewAllocator(registry, true)
	labCfg := config.DefaultLab()
	labCfg.Ports.LabBackend = port

	manifest := testManifest(t, "bound")
	plan, err := allocator.Allocate(context.Background(), AllocationRequest{
		InstanceID: manifest.InstanceID,
		LabConfig:  labCfg,
		Manifest:   manifest,
		Claim:      true,
	})
	require.NoError(t, err)
	require.NotZero(t, plan.Slot)
}

func testManifest(t *testing.T, id string) *Manifest {
	t.Helper()

	rootDir := t.TempDir()
	configPath := filepath.Join(rootDir, config.DefaultConfigFileName)

	return &Manifest{
		SchemaVersion: SchemaVersion,
		InstanceID:    id,
		Status:        StatusCreated,
		RootDir:       rootDir,
		ConfigPath:    configPath,
		OverridesPath: filepath.Join(rootDir, ".cbt-overrides.yaml"),
		StateDir:      InstanceStateDir(rootDir, id),
		Repos:         map[string]xcligit.RepoVersion{},
		Docker:        NewDockerPlan(id, configPath),
		PIDs:          map[string]int{},
		URLs:          map[string]string{},
	}
}

func listenOnFreePort(t *testing.T, start int, end int) (int, func()) {
	t.Helper()

	for port := start; port < end; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}

		return port, func() {
			_ = listener.Close()
		}
	}

	return 0, func() {}
}
