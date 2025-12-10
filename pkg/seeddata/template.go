package seeddata

import (
	"bytes"
	"fmt"
	"text/template"
)

// TestYAMLTemplate is the template for generating xatu-cbt test YAML files.
const TestYAMLTemplate = `model: {{ .Model }}
network: {{ .Network }}
spec: {{ .Spec }}
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
	Spec     string // Fork spec (e.g., "pectra", "fusaka")
	URL      string // URL to the parquet file
	RowCount int64  // Number of rows in the parquet file
}

// GenerateTestYAML generates a test YAML string from the template data.
func GenerateTestYAML(data TemplateData) (string, error) {
	tmpl, err := template.New("test").Parse(TestYAMLTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
