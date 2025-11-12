package discovery

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/sirupsen/logrus"
)

// Discovery handles repository discovery.
type Discovery struct {
	log      logrus.FieldLogger
	basePath string
}

// NewDiscovery creates a new Discovery instance.
func NewDiscovery(log logrus.FieldLogger, basePath string) *Discovery {
	return &Discovery{
		log:      log.WithField("component", "discovery"),
		basePath: basePath,
	}
}

// DiscoverRepos attempts to find all required lab repositories.
func (d *Discovery) DiscoverRepos() (*config.LabReposConfig, error) {
	d.log.Info("discovering lab repositories")

	repos := &config.LabReposConfig{
		CBT:        filepath.Join(d.basePath, "cbt"),
		XatuCBT:    filepath.Join(d.basePath, "xatu-cbt"),
		CBTAPI:     filepath.Join(d.basePath, "cbt-api"),
		LabBackend: filepath.Join(d.basePath, "lab-backend"),
		Lab:        filepath.Join(d.basePath, "lab"),
	}

	// Validate each repository
	repoMap := map[string]*string{
		"cbt":         &repos.CBT,
		"xatu-cbt":    &repos.XatuCBT,
		"cbt-api":     &repos.CBTAPI,
		"lab-backend": &repos.LabBackend,
		"lab":         &repos.Lab,
	}

	for name, path := range repoMap {
		if err := d.validateRepo(name, *path); err != nil {
			return nil, fmt.Errorf("failed to validate %s: %w", name, err)
		}

		d.log.WithFields(logrus.Fields{
			"repo": name,
			"path": *path,
		}).Info("found repository")
	}

	return repos, nil
}

// validateRepo checks if a repository exists and has expected structure.
func (d *Discovery) validateRepo(name, path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check if directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("repository not found at %s", absPath)
		}

		return fmt.Errorf("failed to stat directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Check for expected files based on repo type
	switch name {
	case "cbt", "xatu-cbt", "cbt-api", "lab-backend":
		// Go repositories - check for go.mod
		if !d.fileExists(filepath.Join(absPath, "go.mod")) {
			return fmt.Errorf("go.mod not found (not a Go project)")
		}
	case "lab":
		// Frontend repository - check for package.json
		if !d.fileExists(filepath.Join(absPath, "package.json")) {
			return fmt.Errorf("package.json not found (not a Node.js project)")
		}
	}

	// Additional validation for specific repos
	switch name {
	case "xatu-cbt":
		// Check for models directory
		if !d.dirExists(filepath.Join(absPath, "models")) {
			return fmt.Errorf("models directory not found")
		}
	case "lab":
		// Check for src directory
		if !d.dirExists(filepath.Join(absPath, "src")) {
			return fmt.Errorf("src directory not found")
		}
	}

	return nil
}

// fileExists checks if a file exists.
func (d *Discovery) fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

// dirExists checks if a directory exists.
func (d *Discovery) dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.IsDir()
}
