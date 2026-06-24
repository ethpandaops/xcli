package instance

import (
	"context"
	"fmt"
	"os"

	"github.com/ethpandaops/xcli/pkg/config"
	xcligit "github.com/ethpandaops/xcli/pkg/git"
	"github.com/ethpandaops/xcli/pkg/workspace"
)

// Runtime is the fully resolved per-instance bundle used by later stack phases.
type Runtime struct {
	Workspace  *workspace.Workspace
	LabConfig  *config.LabConfig
	Registry   *Registry
	InstanceID string
	Manifest   *Manifest
	Ports      PortPlan
	Docker     DockerPlan
	Repos      map[string]xcligit.RepoVersion
}

// EffectiveDockerPlan returns the runtime Docker plan with manifest/id fallbacks.
func (r *Runtime) EffectiveDockerPlan() DockerPlan {
	if r == nil {
		return DockerPlan{}
	}

	fallback := DockerPlan{}
	if r.Manifest != nil {
		fallback = r.Manifest.EffectiveDockerPlan()
	}
	if fallback.ProjectName == "" && r.InstanceID != "" {
		configPath := ""
		if r.Manifest != nil {
			configPath = r.Manifest.ConfigPath
		}
		fallback = NewDockerPlan(r.InstanceID, configPath)
	}

	return r.Docker.WithDefaults(fallback)
}

// RuntimeOptions configures runtime assembly.
type RuntimeOptions struct {
	Registry   *Registry
	Allocator  *Allocator
	ClaimPorts bool
	ProbePorts bool
}

// ResolveRuntime loads a persisted runtime for the selected instance when one
// exists, otherwise assembles an unfinalized runtime with the supplied options.
func ResolveRuntime(
	ctx context.Context,
	configPath string,
	cliInstanceID string,
	checkCWDOverrides bool,
	opts RuntimeOptions,
) (*Runtime, error) {
	labCfg, ws, err := workspace.LoadLabConfig(configPath, checkCWDOverrides)
	if err != nil {
		return nil, err
	}

	return ResolveRuntimeFromWorkspace(ctx, ws, labCfg, cliInstanceID, opts)
}

// ResolveRuntimeFromWorkspace is ResolveRuntime for callers that already
// loaded the authoritative workspace.
func ResolveRuntimeFromWorkspace(
	ctx context.Context,
	ws *workspace.Workspace,
	labCfg *config.LabConfig,
	cliInstanceID string,
	opts RuntimeOptions,
) (*Runtime, error) {
	instanceID, err := ResolveID(ws, labCfg, cliInstanceID)
	if err != nil {
		return nil, err
	}

	registry := opts.Registry
	if registry == nil {
		registry, err = DefaultRegistry()
		if err != nil {
			return nil, err
		}
		opts.Registry = registry
	}

	if _, statErr := os.Stat(registry.ManifestPath(instanceID)); statErr == nil {
		manifest, loadErr := registry.Load(instanceID)
		if loadErr != nil {
			return nil, loadErr
		}

		if cliInstanceID != "" {
			return NewRuntimeFromManifestConfig(ctx, manifest, registry)
		}
		if manifest.ConfigPath != "" && ws.ConfigPath != "" && !sameConfigPath(manifest.ConfigPath, ws.ConfigPath) {
			return nil, fmt.Errorf(
				"instance id %q is registered for config %s, not current config %s",
				instanceID,
				manifest.ConfigPath,
				ws.ConfigPath,
			)
		}

		return NewRuntimeFromManifest(manifest, labCfg, ws, registry)
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("failed to inspect instance manifest %q: %w", instanceID, statErr)
	}

	return NewRuntimeFromWorkspace(ctx, ws, labCfg, cliInstanceID, opts)
}

// NewRuntimeFromWorkspace assembles a runtime from an already resolved workspace.
func NewRuntimeFromWorkspace(
	ctx context.Context,
	ws *workspace.Workspace,
	labCfg *config.LabConfig,
	cliInstanceID string,
	opts RuntimeOptions,
) (*Runtime, error) {
	manifest, err := NewManifest(ctx, ws, labCfg, cliInstanceID)
	if err != nil {
		return nil, err
	}

	registry := opts.Registry
	if registry == nil {
		registry, err = DefaultRegistry()
		if err != nil {
			return nil, err
		}
	}

	allocator := opts.Allocator
	if allocator == nil {
		allocator = NewAllocator(registry, opts.ProbePorts)
	}

	ports, err := allocator.Allocate(ctx, AllocationRequest{
		InstanceID: manifest.InstanceID,
		LabConfig:  labCfg,
		Manifest:   manifest,
		Claim:      opts.ClaimPorts,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to allocate ports: %w", err)
	}

	manifest.Ports = ports
	manifest.Docker = NewDockerPlan(manifest.InstanceID, manifest.ConfigPath)

	return &Runtime{
		Workspace:  ws,
		LabConfig:  labCfg,
		Registry:   registry,
		InstanceID: manifest.InstanceID,
		Manifest:   manifest,
		Ports:      ports,
		Docker:     manifest.Docker,
		Repos:      manifest.Repos,
	}, nil
}

// NewRuntimeFromManifestConfig reloads the workspace and lab config recorded
// in a persisted manifest before assembling the runtime.
func NewRuntimeFromManifestConfig(
	_ context.Context,
	manifest *Manifest,
	registry *Registry,
) (*Runtime, error) {
	if manifest == nil {
		return nil, fmt.Errorf("manifest is required")
	}
	if manifest.ConfigPath == "" {
		return nil, fmt.Errorf("manifest %q has no config path", manifest.InstanceID)
	}

	labCfg, ws, err := workspace.LoadLabConfig(manifest.ConfigPath, false)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to load config for instance %q from %s: %w",
			manifest.InstanceID,
			manifest.ConfigPath,
			err,
		)
	}

	return NewRuntimeFromManifest(manifest, labCfg, ws, registry)
}

// NewRuntimeFromManifest assembles a runtime from persisted instance state.
func NewRuntimeFromManifest(
	manifest *Manifest,
	labCfg *config.LabConfig,
	ws *workspace.Workspace,
	registry *Registry,
) (*Runtime, error) {
	if manifest == nil {
		return nil, fmt.Errorf("manifest is required")
	}
	if labCfg == nil {
		return nil, fmt.Errorf("lab config is required")
	}
	if ws == nil {
		return nil, fmt.Errorf("workspace is required")
	}

	if manifest.Docker.ProjectName == "" {
		manifest.Docker = NewDockerPlan(manifest.InstanceID, manifest.ConfigPath)
	}

	return &Runtime{
		Workspace:  ws,
		LabConfig:  labCfg,
		Registry:   registry,
		InstanceID: manifest.InstanceID,
		Manifest:   manifest,
		Ports:      manifest.Ports,
		Docker:     manifest.Docker,
		Repos:      manifest.Repos,
	}, nil
}
