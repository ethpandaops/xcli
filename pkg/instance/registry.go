package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const manifestFileMode = 0644

// Registry persists lab instance manifests in the user-global registry.
type Registry struct {
	dir string
}

// NewRegistry creates a registry rooted at dir.
func NewRegistry(dir string) *Registry {
	return &Registry{dir: dir}
}

// DefaultRegistry returns the user-global lab instance registry.
func DefaultRegistry() (*Registry, error) {
	dir, err := DefaultRegistryDir()
	if err != nil {
		return nil, err
	}

	return NewRegistry(dir), nil
}

// DefaultRegistryDir returns ~/.xcli/lab/instances.
func DefaultRegistryDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".xcli", "lab", "instances"), nil
}

// ManifestPath returns the global registry file for an instance.
func (r *Registry) ManifestPath(instanceID string) string {
	return filepath.Join(r.dir, instanceID+".json")
}

// Save writes the global manifest and the root-local manifest copy.
func (r *Registry) Save(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is required")
	}
	if manifest.InstanceID == "" {
		return fmt.Errorf("manifest instance id is required")
	}
	if err := r.checkManifestConflict(manifest); err != nil {
		return err
	}

	now := time.Now().UTC()
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = now
	}
	manifest.UpdatedAt = now

	if err := writeJSONAtomic(r.ManifestPath(manifest.InstanceID), manifest); err != nil {
		return fmt.Errorf("failed to write global manifest: %w", err)
	}

	localPath := LocalManifestPath(manifest.RootDir, manifest.InstanceID)
	if err := writeJSONAtomic(localPath, manifest); err != nil {
		return fmt.Errorf("failed to write local manifest copy: %w", err)
	}

	return nil
}

func (r *Registry) checkManifestConflict(manifest *Manifest) error {
	existing, err := r.Load(manifest.InstanceID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("failed to inspect existing manifest %q: %w", manifest.InstanceID, err)
	}
	if existing == nil || existing.ConfigPath == "" || manifest.ConfigPath == "" {
		return nil
	}
	if sameConfigPath(existing.ConfigPath, manifest.ConfigPath) {
		return nil
	}

	return fmt.Errorf(
		"instance id %q is already registered for config %s; refusing to overwrite with config %s",
		manifest.InstanceID,
		existing.ConfigPath,
		manifest.ConfigPath,
	)
}

// Load reads one manifest by id from the global registry.
func (r *Registry) Load(instanceID string) (*Manifest, error) {
	data, err := os.ReadFile(r.ManifestPath(instanceID))
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest %q: %w", instanceID, err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest %q: %w", instanceID, err)
	}

	return &manifest, nil
}

// LoadAll reads all manifests from the global registry.
func (r *Registry) LoadAll() ([]*Manifest, error) {
	entries, err := os.ReadDir(r.dir)
	if os.IsNotExist(err) {
		return []*Manifest{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read registry dir: %w", err)
	}

	manifests := make([]*Manifest, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		instanceID := strings.TrimSuffix(entry.Name(), ".json")
		manifest, loadErr := r.Load(instanceID)
		if loadErr != nil {
			return nil, loadErr
		}

		manifests = append(manifests, manifest)
	}

	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].InstanceID < manifests[j].InstanceID
	})

	return manifests, nil
}

// Delete removes the global manifest and root-local manifest copy.
func (r *Registry) Delete(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is required")
	}
	if manifest.InstanceID == "" {
		return fmt.Errorf("manifest instance id is required")
	}

	paths := []string{r.ManifestPath(manifest.InstanceID)}
	if manifest.RootDir != "" {
		paths = append(paths, LocalManifestPath(manifest.RootDir, manifest.InstanceID))
	}

	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove manifest %s: %w", path, err)
		}
	}

	return nil
}

func writeJSONAtomic(path string, value any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmp.Chmod(manifestFileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}

	return nil
}

func sameConfigPath(a, b string) bool {
	aAbs, aErr := filepath.Abs(a)
	bAbs, bErr := filepath.Abs(b)
	if aErr == nil {
		a = aAbs
	}
	if bErr == nil {
		b = bAbs
	}

	return filepath.Clean(a) == filepath.Clean(b)
}
