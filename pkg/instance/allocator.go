package instance

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/ethpandaops/xcli/pkg/config"
)

const defaultMaxAllocationSlots = 100

const reservationGracePeriod = 10 * time.Minute

// Allocator assigns preferred port slots under the global registry lock.
type Allocator struct {
	registry   *Registry
	lockPath   string
	probePorts bool
	maxSlots   int
}

// NewAllocator returns an allocator using the given registry.
func NewAllocator(registry *Registry, probePorts bool) *Allocator {
	lockPath := ""
	if registry != nil && registry.dir != "" {
		lockPath = filepath.Join(filepath.Dir(registry.dir), "registry.lock")
	}

	return &Allocator{
		registry:   registry,
		lockPath:   lockPath,
		probePorts: probePorts,
		maxSlots:   defaultMaxAllocationSlots,
	}
}

// AllocationRequest describes one port allocation.
type AllocationRequest struct {
	InstanceID string
	LabConfig  *config.LabConfig
	Manifest   *Manifest
	Claim      bool
}

// Allocate fills a PortPlan. If Claim is true, it writes a reserved manifest
// while still holding the registry lock so concurrent allocators see the slot.
func (a *Allocator) Allocate(ctx context.Context, req AllocationRequest) (PortPlan, error) {
	if req.InstanceID == "" {
		return PortPlan{}, fmt.Errorf("instance id is required")
	}
	if req.LabConfig == nil {
		return PortPlan{}, fmt.Errorf("lab config is required")
	}
	if a.registry == nil {
		var err error
		a.registry, err = DefaultRegistry()
		if err != nil {
			return PortPlan{}, err
		}
	}
	if a.lockPath == "" {
		lockPath, err := DefaultRegistryLockPath()
		if err != nil {
			return PortPlan{}, err
		}
		a.lockPath = lockPath
	}

	unlock, err := lockFile(a.lockPath)
	if err != nil {
		return PortPlan{}, err
	}
	defer unlock()

	activePorts, err := a.activeRegistryPorts(req.InstanceID)
	if err != nil {
		return PortPlan{}, err
	}

	var rejected []string
	for slot := 0; slot < a.maxSlots; slot++ {
		if err := ctx.Err(); err != nil {
			return PortPlan{}, err
		}

		plan, planErr := BuildPortPlan(req.LabConfig, slot)
		if planErr != nil {
			return PortPlan{}, planErr
		}

		if overlap := overlapWithSet(plan.AllPorts(), activePorts); len(overlap) > 0 {
			rejected = append(rejected, fmt.Sprintf("slot %d registry ports in use: %v", slot, overlap))
			continue
		}

		if a.probePorts {
			boundPorts := boundPorts(plan.AllPorts())
			if len(boundPorts) > 0 {
				rejected = append(rejected, fmt.Sprintf("slot %d bound ports in use: %v", slot, boundPorts))
				continue
			}
		}

		if req.Claim {
			if req.Manifest == nil {
				return PortPlan{}, fmt.Errorf("manifest is required to claim allocated ports")
			}

			req.Manifest.Ports = plan
			req.Manifest.Status = StatusReserved
			req.Manifest.Docker = NewDockerPlan(req.InstanceID, req.Manifest.ConfigPath)
			if err := a.registry.Save(req.Manifest); err != nil {
				return PortPlan{}, err
			}
		}

		return plan, nil
	}

	return PortPlan{}, fmt.Errorf("no available port slot found after %d slots (%v)", a.maxSlots, rejected)
}

// DefaultRegistryLockPath returns ~/.xcli/lab/registry.lock.
func DefaultRegistryLockPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".xcli", "lab", "registry.lock"), nil
}

func (a *Allocator) activeRegistryPorts(instanceID string) (map[int]bool, error) {
	manifests, err := a.registry.LoadAll()
	if err != nil {
		return nil, err
	}

	ports := make(map[int]bool)
	for _, manifest := range manifests {
		if manifest.InstanceID == instanceID || !isActiveManifest(manifest) {
			continue
		}

		for _, port := range manifest.Ports.AllPorts() {
			ports[port] = true
		}
	}

	return ports, nil
}

func isActiveManifest(manifest *Manifest) bool {
	if manifest == nil {
		return false
	}

	switch manifest.Status {
	case StatusRunning:
		return true
	case StatusReserved:
		if reservationHasBacking(manifest) {
			return true
		}
		if manifest.UpdatedAt.IsZero() {
			return false
		}

		return time.Since(manifest.UpdatedAt) <= reservationGracePeriod
	default:
		return false
	}
}

func reservationHasBacking(manifest *Manifest) bool {
	for _, pid := range manifest.PIDs {
		if pid > 0 && defaultPIDAlive(pid) {
			return true
		}
	}
	for _, port := range manifest.Ports.AllPorts() {
		if port > 0 && !isPortFree(port) {
			return true
		}
	}

	return false
}

func lockFile(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to lock registry: %w", err)
	}

	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func boundPorts(ports []int) []int {
	bound := make([]int, 0)
	for _, port := range ports {
		if !isPortFree(port) {
			bound = append(bound, port)
		}
	}
	sort.Ints(bound)

	return bound
}

func isPortFree(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}

	_ = listener.Close()

	return true
}

func overlapWithSet(ports []int, used map[int]bool) []int {
	overlap := make([]int, 0)
	for _, port := range ports {
		if used[port] {
			overlap = append(overlap, port)
		}
	}
	sort.Ints(overlap)

	return overlap
}
