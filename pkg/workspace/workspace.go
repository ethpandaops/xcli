// Package workspace resolves the authoritative xcli workspace root and the
// paths derived from it.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
)

// Workspace describes the authoritative workspace selected for a command.
type Workspace struct {
	RootDir       string
	ConfigPath    string
	OverridesPath string
	StateDir      string
	ConfigExists  bool
}

// Resolve selects the authoritative workspace for a command.
//
// The default config path searches upward only from the current directory. An
// explicit config path is resolved directly and always wins.
func Resolve(configPath string, requireConfig bool, checkCWDOverrides bool) (*Workspace, error) {
	path := config.EffectiveConfigPath(configPath)
	if path == "" {
		path = config.DefaultConfigFileName
	}

	var (
		resolvedPath string
		exists       bool
		err          error
	)

	defaultPath := isDefaultConfigPath(path)
	if defaultPath {
		resolvedPath, exists, err = resolveDefaultConfig()
	} else {
		resolvedPath, exists, err = resolveExplicitConfig(path)
	}

	if err != nil {
		return nil, err
	}

	if requireConfig && !exists {
		if !defaultPath {
			return nil, fmt.Errorf("config file not found: %s", resolvedPath)
		}

		return nil, fmt.Errorf(
			"no %s found searching upward from current directory; pass --config <path> or run xcli lab init",
			config.DefaultConfigFileName,
		)
	}

	rootDir := filepath.Dir(resolvedPath)
	ws := &Workspace{
		RootDir:       rootDir,
		ConfigPath:    resolvedPath,
		OverridesPath: filepath.Join(rootDir, constants.CBTOverridesFile),
		StateDir:      filepath.Join(rootDir, ".xcli"),
		ConfigExists:  exists,
	}

	if checkCWDOverrides {
		if err := ws.CheckCWDOverrides(); err != nil {
			return nil, err
		}
	}

	return ws, nil
}

// LoadConfig resolves and loads the root config from the authoritative
// workspace.
func LoadConfig(configPath string, requireConfig bool, checkCWDOverrides bool) (*config.Config, *Workspace, error) {
	ws, err := Resolve(configPath, requireConfig, checkCWDOverrides)
	if err != nil {
		return nil, nil, err
	}

	result, err := config.Load(ws.ConfigPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	return result.Config, ws, nil
}

// LoadLabConfig resolves the workspace, loads the lab config, and converts
// repository paths to absolute paths relative to the config file directory.
func LoadLabConfig(configPath string, checkCWDOverrides bool) (*config.LabConfig, *Workspace, error) {
	rootCfg, ws, err := LoadConfig(configPath, true, checkCWDOverrides)
	if err != nil {
		return nil, nil, err
	}

	if rootCfg.Lab == nil {
		return nil, nil, fmt.Errorf("lab configuration not found - run 'xcli lab init' first")
	}

	ResolveLabRepoPaths(rootCfg.Lab, ws.RootDir)

	return rootCfg.Lab, ws, nil
}

// ResolveLabRepoPaths resolves lab repository paths relative to rootDir.
func ResolveLabRepoPaths(labCfg *config.LabConfig, rootDir string) {
	if labCfg == nil {
		return
	}

	labCfg.Repos.CBT = resolvePath(rootDir, labCfg.Repos.CBT)
	labCfg.Repos.XatuCBT = resolvePath(rootDir, labCfg.Repos.XatuCBT)
	labCfg.Repos.CBTAPI = resolvePath(rootDir, labCfg.Repos.CBTAPI)
	labCfg.Repos.LabBackend = resolvePath(rootDir, labCfg.Repos.LabBackend)
	labCfg.Repos.Lab = resolvePath(rootDir, labCfg.Repos.Lab)
}

// CheckCWDOverrides prevents a .cbt-overrides.yaml in the current working
// directory from being mistaken for the authoritative workspace override file.
func (w *Workspace) CheckCWDOverrides() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	cwdOverrides := filepath.Join(cwd, constants.CBTOverridesFile)

	cwdOverrides, err = filepath.Abs(cwdOverrides)
	if err != nil {
		return fmt.Errorf("failed to resolve current overrides path: %w", err)
	}

	if samePath(cwdOverrides, w.OverridesPath) {
		return nil
	}

	if _, statErr := os.Stat(cwdOverrides); statErr == nil {
		return fmt.Errorf(
			"refusing to continue: current directory contains %s at %s, but the authoritative overrides path for config %s is %s",
			constants.CBTOverridesFile,
			cwdOverrides,
			w.ConfigPath,
			w.OverridesPath,
		)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("failed to inspect current overrides path %s: %w", cwdOverrides, statErr)
	}

	return nil
}

func resolveDefaultConfig() (string, bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("failed to get current directory: %w", err)
	}

	if found := searchUpward(cwd, config.DefaultConfigFileName); found != "" {
		return found, true, nil
	}

	fallback, err := filepath.Abs(filepath.Join(cwd, config.DefaultConfigFileName))
	if err != nil {
		return "", false, fmt.Errorf("failed to resolve default config path: %w", err)
	}

	return fallback, false, nil
}

func resolveExplicitConfig(path string) (string, bool, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", false, fmt.Errorf("failed to resolve config path %q: %w", path, err)
	}

	_, statErr := os.Stat(absPath)
	switch {
	case statErr == nil:
		return absPath, true, nil
	case os.IsNotExist(statErr):
		return absPath, false, nil
	default:
		return "", false, fmt.Errorf("failed to inspect config path %s: %w", absPath, statErr)
	}
}

func searchUpward(startDir, fileName string) string {
	currentDir := startDir

	for {
		configPath := filepath.Join(currentDir, fileName)
		if _, err := os.Stat(configPath); err == nil {
			absPath, absErr := filepath.Abs(configPath)
			if absErr != nil {
				return configPath
			}

			return absPath
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return ""
		}

		currentDir = parentDir
	}
}

func resolvePath(rootDir, path string) string {
	if path == "" {
		return ""
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	return filepath.Clean(filepath.Join(rootDir, path))
}

func isDefaultConfigPath(path string) bool {
	return path == config.DefaultConfigFileName ||
		path == "."+string(filepath.Separator)+config.DefaultConfigFileName
}

func samePath(a, b string) bool {
	return filepath.Clean(normalizePath(a)) == filepath.Clean(normalizePath(b))
}

func normalizePath(path string) string {
	if evaluated, err := filepath.EvalSymlinks(path); err == nil {
		return evaluated
	}

	if absPath, err := filepath.Abs(path); err == nil {
		return absPath
	}

	return path
}
