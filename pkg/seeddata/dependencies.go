// Package seeddata provides functionality to generate seed data parquet files
// for xatu-cbt tests by extracting data from external ClickHouse.
package seeddata

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// DependencyType represents the type of a dependency.
type DependencyType string

const (
	// DependencyTypeExternal represents an external model dependency.
	DependencyTypeExternal DependencyType = "external"
	// DependencyTypeTransformation represents a transformation model dependency.
	DependencyTypeTransformation DependencyType = "transformation"
)

// Dependency represents a single model dependency.
type Dependency struct {
	Type DependencyType
	Name string
}

// DependencyTree represents a model and its dependencies.
type DependencyTree struct {
	Model        string
	Type         DependencyType
	Dependencies []Dependency
	Children     map[string]*DependencyTree // For transformation deps (recursive)
}

// dependencyPattern matches dependency strings like "{{transformation}}.model_name" or "{{external}}.model_name".
var dependencyPattern = regexp.MustCompile(`^\{\{(transformation|external)\}\}\.(.+)$`)

// sqlFrontmatter represents the YAML frontmatter in SQL files.
type sqlFrontmatter struct {
	Table        string   `yaml:"table"`
	Dependencies []string `yaml:"dependencies"`
}

// ParseDependencies parses the dependencies from a SQL file's YAML frontmatter.
func ParseDependencies(sqlPath string) ([]Dependency, error) {
	frontmatter, err := parseFrontmatter(sqlPath)
	if err != nil {
		return nil, err
	}

	deps := make([]Dependency, 0, len(frontmatter.Dependencies))

	for _, depStr := range frontmatter.Dependencies {
		dep, err := parseDependencyString(depStr)
		if err != nil {
			return nil, fmt.Errorf("invalid dependency '%s': %w", depStr, err)
		}

		deps = append(deps, dep)
	}

	return deps, nil
}

// ResolveDependencyTree recursively resolves all dependencies for a transformation model.
// It returns a tree structure with all dependencies, where external models are leaf nodes.
func ResolveDependencyTree(model string, xatuCBTPath string, visited map[string]bool) (*DependencyTree, error) {
	// Initialize visited map if nil (first call)
	if visited == nil {
		visited = make(map[string]bool, 16)
	}

	// Check for circular dependencies
	if visited[model] {
		return nil, fmt.Errorf("circular dependency detected: %s", model)
	}

	visited[model] = true

	defer func() { visited[model] = false }()

	// First, try to find as transformation model
	transformationPath := filepath.Join(xatuCBTPath, "models", "transformations", model+".sql")

	if _, err := os.Stat(transformationPath); err == nil {
		return resolveTransformationTree(model, transformationPath, xatuCBTPath, visited)
	}

	// If not found as transformation, check if it's an external model
	externalPath := filepath.Join(xatuCBTPath, "models", "external", model+".sql")
	if _, err := os.Stat(externalPath); err == nil {
		return &DependencyTree{
			Model:        model,
			Type:         DependencyTypeExternal,
			Dependencies: nil,
			Children:     nil,
		}, nil
	}

	return nil, fmt.Errorf("model '%s' not found in transformations or external models", model)
}

// resolveTransformationTree resolves a transformation model's dependency tree.
func resolveTransformationTree(model, sqlPath, xatuCBTPath string, visited map[string]bool) (*DependencyTree, error) {
	deps, err := ParseDependencies(sqlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dependencies for %s: %w", model, err)
	}

	tree := &DependencyTree{
		Model:        model,
		Type:         DependencyTypeTransformation,
		Dependencies: deps,
		Children:     make(map[string]*DependencyTree, len(deps)),
	}

	// Recursively resolve transformation dependencies
	for _, dep := range deps {
		if dep.Type == DependencyTypeTransformation {
			childTree, err := ResolveDependencyTree(dep.Name, xatuCBTPath, visited)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve dependency %s: %w", dep.Name, err)
			}

			tree.Children[dep.Name] = childTree
		} else {
			// External dependencies are leaf nodes
			tree.Children[dep.Name] = &DependencyTree{
				Model:        dep.Name,
				Type:         DependencyTypeExternal,
				Dependencies: nil,
				Children:     nil,
			}
		}
	}

	return tree, nil
}

// GetExternalDependencies returns all external model names from the dependency tree (leaf nodes).
// The result is deduplicated.
func (t *DependencyTree) GetExternalDependencies() []string {
	seen := make(map[string]bool, 8)
	externals := make([]string, 0, 8)

	t.collectExternalDeps(seen, &externals)

	return externals
}

// collectExternalDeps recursively collects external dependencies.
func (t *DependencyTree) collectExternalDeps(seen map[string]bool, result *[]string) {
	if t.Type == DependencyTypeExternal {
		if !seen[t.Model] {
			seen[t.Model] = true
			*result = append(*result, t.Model)
		}

		return
	}

	for _, child := range t.Children {
		child.collectExternalDeps(seen, result)
	}
}

// PrintTree returns a string representation of the dependency tree.
func (t *DependencyTree) PrintTree(indent string) string {
	var sb strings.Builder

	typeStr := "transformation"
	if t.Type == DependencyTypeExternal {
		typeStr = "external"
	}

	sb.WriteString(fmt.Sprintf("%s%s ({{%s}})\n", indent, t.Model, typeStr))

	childIndent := indent + "  "

	for _, dep := range t.Dependencies {
		if child, ok := t.Children[dep.Name]; ok {
			sb.WriteString(child.PrintTree(childIndent))
		}
	}

	return sb.String()
}

// ListTransformationModels returns a list of available transformation models from the xatu-cbt repo.
func ListTransformationModels(xatuCBTPath string) ([]string, error) {
	modelsDir := filepath.Join(xatuCBTPath, "models", "transformations")

	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read transformations directory: %w", err)
	}

	models := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".sql") {
			// Remove .sql extension to get model name
			models = append(models, strings.TrimSuffix(name, ".sql"))
		}
	}

	return models, nil
}

// parseFrontmatter extracts and parses the YAML frontmatter from a SQL file.
func parseFrontmatter(sqlPath string) (*sqlFrontmatter, error) {
	file, err := os.Open(sqlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	var yamlContent strings.Builder

	inFrontmatter := false
	foundStart := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.TrimSpace(line) == "---" {
			if !foundStart {
				foundStart = true
				inFrontmatter = true

				continue
			}
			// Found end of frontmatter
			break
		}

		if inFrontmatter {
			yamlContent.WriteString(line)
			yamlContent.WriteString("\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	if !foundStart {
		return nil, fmt.Errorf("no YAML frontmatter found in file")
	}

	var fm sqlFrontmatter

	if err := yaml.Unmarshal([]byte(yamlContent.String()), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	return &fm, nil
}

// parseDependencyString parses a dependency string like "{{transformation}}.model_name".
func parseDependencyString(depStr string) (Dependency, error) {
	matches := dependencyPattern.FindStringSubmatch(depStr)
	if matches == nil {
		return Dependency{}, fmt.Errorf("does not match expected pattern {{type}}.name")
	}

	depType := DependencyType(matches[1])
	if depType != DependencyTypeExternal && depType != DependencyTypeTransformation {
		return Dependency{}, fmt.Errorf("unknown dependency type: %s", matches[1])
	}

	return Dependency{
		Type: depType,
		Name: matches[2],
	}, nil
}
