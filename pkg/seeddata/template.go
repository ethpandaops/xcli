package seeddata

import (
	"bytes"
	"fmt"
	"sort"
	"text/template"

	"gopkg.in/yaml.v3"
)

// TestYAMLTemplate is the template for generating xatu-cbt test YAML files.
const TestYAMLTemplate = `model: {{ .Model }}
network: {{ .Network }}
external_data:
    {{ .Model }}:
        url: {{ .URL }}
        network_column: meta_network_name
assertions:
    - name: total count
      sql: |
        SELECT COUNT(*) AS count
        FROM cluster('{remote_cluster}', default.{{ .Model }})
      expected:
        count: {{ .RowCount }}
`

// TemplateData contains the data for generating a test YAML template.
type TemplateData struct {
	Model    string // Model/table name
	Network  string // Network name (e.g., "mainnet", "sepolia")
	URL      string // URL to the parquet file
	RowCount int64  // Number of rows in the parquet file
}

// ExternalDataEntry represents an external data entry in the test YAML.
type ExternalDataEntry struct {
	URL           string `yaml:"url"`
	NetworkColumn string `yaml:"network_column"` //nolint:tagliatelle // xatu-cbt uses snake_case
}

// TransformationTestYAML represents the complete test YAML structure.
type TransformationTestYAML struct {
	Model        string                       `yaml:"model"`
	Network      string                       `yaml:"network"`
	ExternalData map[string]ExternalDataEntry `yaml:"external_data"` //nolint:tagliatelle // xatu-cbt uses snake_case
	Assertions   []Assertion                  `yaml:"assertions"`
}

// TransformationTemplateData contains the data for generating transformation test YAML.
type TransformationTemplateData struct {
	Model          string            // Transformation model name
	Network        string            // Network name
	ExternalModels []string          // List of external model names
	URLs           map[string]string // model name -> parquet URL
	Assertions     []Assertion       // Generated assertions
}

// GenerateTestYAML generates a test YAML string from the template data.
func GenerateTestYAML(data TemplateData) (string, error) {
	tmpl, err := template.New("test").Parse(TestYAMLTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer

	if execErr := tmpl.Execute(&buf, data); execErr != nil {
		return "", fmt.Errorf("failed to execute template: %w", execErr)
	}

	return buf.String(), nil
}

// GenerateTransformationTestYAML generates a complete test YAML for transformation models.
func GenerateTransformationTestYAML(data TransformationTemplateData) (string, error) {
	testYAML := TransformationTestYAML{
		Model:        data.Model,
		Network:      data.Network,
		ExternalData: make(map[string]ExternalDataEntry, len(data.ExternalModels)),
		Assertions:   data.Assertions,
	}

	// Sort external models for consistent output
	sortedModels := make([]string, len(data.ExternalModels))
	copy(sortedModels, data.ExternalModels)
	sort.Strings(sortedModels)

	for _, model := range sortedModels {
		url, ok := data.URLs[model]
		if !ok {
			return "", fmt.Errorf("missing URL for external model: %s", model)
		}

		testYAML.ExternalData[model] = ExternalDataEntry{
			URL:           url,
			NetworkColumn: "meta_network_name",
		}
	}

	// If no assertions provided, use default
	if len(testYAML.Assertions) == 0 {
		testYAML.Assertions = GetDefaultAssertions(data.Model)
	}

	var buf bytes.Buffer

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(4)

	if err := encoder.Encode(testYAML); err != nil {
		return "", fmt.Errorf("failed to encode YAML: %w", err)
	}

	return buf.String(), nil
}
