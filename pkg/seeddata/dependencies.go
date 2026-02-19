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

// IntervalType represents the interval type from external model frontmatter.
type IntervalType string

const (
	// IntervalTypeSlot is slot-based interval (time ranges via slot_start_date_time).
	IntervalTypeSlot IntervalType = "slot"
	// IntervalTypeBlock is block number based interval.
	IntervalTypeBlock IntervalType = "block"
	// IntervalTypeEntity is for dimension/reference tables with no time range.
	IntervalTypeEntity IntervalType = "entity"
)

// intervalConfig represents the interval configuration in model frontmatter.
type intervalConfig struct {
	Type IntervalType `yaml:"type"`
}

// sqlFrontmatter represents the YAML frontmatter in SQL files.
type sqlFrontmatter struct {
	Database     string         `yaml:"database"`
	Table        string         `yaml:"table"`
	Dependencies []string       `yaml:"dependencies"`
	Interval     intervalConfig `yaml:"interval"`
}

// ResolveExternalTableRef returns the fully qualified "database.table" for an external model.
// If the model's frontmatter specifies a database, it uses "database.table".
// Otherwise it falls back to "default.modelName" for backward compatibility.
func ResolveExternalTableRef(model, xatuCBTPath string) string {
	modelPath := findModelFile(xatuCBTPath, "external", model)
	if modelPath == "" {
		return "default." + model
	}

	fm, err := parseFrontmatter(modelPath)
	if err != nil {
		return "default." + model
	}

	if fm.Database != "" && fm.Table != "" {
		return fm.Database + "." + fm.Table
	}

	return "default." + model
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

	// Try to find as transformation model (supports .sql and .yml extensions)
	transformationPath := findModelFile(xatuCBTPath, "transformations", model)

	if transformationPath != "" {
		return resolveTransformationTree(model, transformationPath, xatuCBTPath, visited)
	}

	// If not found as transformation, check if it's an external model
	externalPath := findModelFile(xatuCBTPath, "external", model)
	if externalPath != "" {
		return &DependencyTree{
			Model:        model,
			Type:         DependencyTypeExternal,
			Dependencies: nil,
			Children:     nil,
		}, nil
	}

	return nil, fmt.Errorf("model '%s' not found in transformations or external models", model)
}

// findModelFile looks for a model file with supported extensions (.sql, .yml, .yaml).
func findModelFile(xatuCBTPath, folder, model string) string {
	extensions := []string{".sql", ".yml", ".yaml"}

	for _, ext := range extensions {
		path := filepath.Join(xatuCBTPath, "models", folder, model+ext)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
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

	fmt.Fprintf(&sb, "%s%s ({{%s}})\n", indent, t.Model, typeStr)

	childIndent := indent + "  "

	for _, dep := range t.Dependencies {
		if child, ok := t.Children[dep.Name]; ok {
			sb.WriteString(child.PrintTree(childIndent))
		}
	}

	return sb.String()
}

// GetIntermediateDependencies returns all intermediate (transformation) model names from the
// dependency tree, excluding the root model. These are non-leaf nodes that transform external data.
// The result is deduplicated.
func (t *DependencyTree) GetIntermediateDependencies() []string {
	seen := make(map[string]bool, 8)
	intermediates := make([]string, 0, 8)

	t.collectIntermediateDeps(seen, &intermediates, true)

	return intermediates
}

// collectIntermediateDeps recursively collects intermediate (transformation) dependencies.
func (t *DependencyTree) collectIntermediateDeps(seen map[string]bool, result *[]string, isRoot bool) {
	// Skip external models (leaf nodes)
	if t.Type == DependencyTypeExternal {
		return
	}

	// Add this model if it's not the root and not already seen
	if !isRoot && !seen[t.Model] {
		seen[t.Model] = true
		*result = append(*result, t.Model)
	}

	// Recurse into children
	for _, child := range t.Children {
		child.collectIntermediateDeps(seen, result, false)
	}
}

// ReadIntermediateSQL reads the SQL content for all intermediate dependencies.
// Returns a map of model name to SQL content.
// Note: YAML script models (.yml/.yaml) are skipped as they don't contain SQL to analyze.
func ReadIntermediateSQL(tree *DependencyTree, xatuCBTPath string) (map[string]string, error) {
	intermediates := tree.GetIntermediateDependencies()
	result := make(map[string]string, len(intermediates))

	for _, model := range intermediates {
		modelPath := findModelFile(xatuCBTPath, "transformations", model)
		if modelPath == "" {
			// Model file not found, skip
			continue
		}

		// Only read SQL files - YAML script models don't have SQL to analyze
		if !strings.HasSuffix(modelPath, ".sql") {
			continue
		}

		content, err := os.ReadFile(modelPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read SQL for %s: %w", model, err)
		}

		result[model] = string(content)
	}

	return result, nil
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

		// Support .sql, .yml, and .yaml extensions
		for _, ext := range []string{".sql", ".yml", ".yaml"} {
			if strings.HasSuffix(name, ext) {
				models = append(models, strings.TrimSuffix(name, ext))

				break
			}
		}
	}

	return models, nil
}

// parseFrontmatter extracts and parses the YAML frontmatter from a SQL file,
// or parses a pure YAML file (.yml/.yaml) directly.
func parseFrontmatter(modelPath string) (*sqlFrontmatter, error) {
	// Check if this is a pure YAML file (not SQL with frontmatter)
	if strings.HasSuffix(modelPath, ".yml") || strings.HasSuffix(modelPath, ".yaml") {
		return parseYAMLFile(modelPath)
	}

	// Parse SQL file with YAML frontmatter
	return parseSQLFrontmatter(modelPath)
}

// parseYAMLFile parses a pure YAML model file (.yml or .yaml).
func parseYAMLFile(yamlPath string) (*sqlFrontmatter, error) {
	content, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var fm sqlFrontmatter

	if err := yaml.Unmarshal(content, &fm); err != nil {
		return nil, fmt.Errorf("failed to parse YAML file: %w", err)
	}

	return &fm, nil
}

// parseSQLFrontmatter extracts and parses the YAML frontmatter from a SQL file.
func parseSQLFrontmatter(sqlPath string) (*sqlFrontmatter, error) {
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

// GetExternalModelIntervalType returns the interval type for an external model.
// Returns IntervalTypeEntity for dimension tables, or the actual type (slot, block, etc.).
func GetExternalModelIntervalType(model, xatuCBTPath string) (IntervalType, error) {
	modelPath := findModelFile(xatuCBTPath, "external", model)
	if modelPath == "" {
		return "", fmt.Errorf("external model '%s' not found", model)
	}

	fm, err := parseFrontmatter(modelPath)
	if err != nil {
		return "", fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	if fm.Interval.Type == "" {
		// Default to slot if not specified
		return IntervalTypeSlot, nil
	}

	return fm.Interval.Type, nil
}

// IsEntityModel checks if an external model is an entity/dimension table.
func IsEntityModel(model, xatuCBTPath string) bool {
	intervalType, err := GetExternalModelIntervalType(model, xatuCBTPath)
	if err != nil {
		return false
	}

	return intervalType == IntervalTypeEntity
}

// GetExternalModelIntervalTypes returns interval types for multiple external models.
func GetExternalModelIntervalTypes(models []string, xatuCBTPath string) (map[string]IntervalType, error) {
	result := make(map[string]IntervalType, len(models))

	for _, model := range models {
		intervalType, err := GetExternalModelIntervalType(model, xatuCBTPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get interval type for %s: %w", model, err)
		}

		result[model] = intervalType
	}

	return result, nil
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
