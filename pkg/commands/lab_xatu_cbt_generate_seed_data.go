package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/constants"
	"github.com/ethpandaops/xcli/pkg/seeddata"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	defaultRowLimit = 10000
)

// NewLabXatuCBTGenerateSeedDataCommand creates the lab xatu-cbt generate-seed-data command.
func NewLabXatuCBTGenerateSeedDataCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var (
		model         string
		network       string
		spec          string
		rangeColumn   string
		from          string
		to            string
		filters       []string
		limit         int
		output        string
		upload        bool
		noSanitizeIPs bool
	)

	cmd := &cobra.Command{
		Use:   "generate-seed-data",
		Short: "Generate seed data parquet files for xatu-cbt tests",
		Long: `Generate seed data parquet files for xatu-cbt tests by extracting data
from the external ClickHouse cluster.

This command requires hybrid mode to be enabled, as it needs access to
the external ClickHouse cluster containing production xatu data.

Interactive mode (prompts for all required inputs):
  xcli lab xatu-cbt generate-seed-data

Scripted mode (all flags provided):
  xcli lab xatu-cbt generate-seed-data \
    --model beacon_api_eth_v1_events_block \
    --network mainnet \
    --spec pectra \
    --range-column slot \
    --from 1000000 \
    --to 1001000 \
    --filter "status = VALID" \
    --filter "proposer_index > 100" \
    --upload

The command outputs a test YAML template that can be used directly in xatu-cbt tests.

S3 Upload Configuration (defaults to Cloudflare R2):
  AWS_ACCESS_KEY_ID      R2 API token Access Key ID
  AWS_SECRET_ACCESS_KEY  R2 API token Secret Access Key
  S3_ENDPOINT            Override endpoint (default: ethpandaops R2)
  S3_BUCKET              Override bucket (default: ethpandaops-platform-production-public)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerateSeedData(cmd.Context(), log, configPath,
				model, network, spec, rangeColumn, from, to, filters, limit, output, upload, !noSanitizeIPs)
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "Table name from xatu-cbt external models")
	cmd.Flags().StringVar(&network, "network", "", "Network name (mainnet, sepolia, etc.)")
	cmd.Flags().StringVar(&spec, "spec", "", "Fork spec (pectra, fusaka, etc.)")
	cmd.Flags().StringVar(&rangeColumn, "range-column", "", "Column to filter on (e.g., slot, epoch)")
	cmd.Flags().StringVar(&from, "from", "", "Range start value")
	cmd.Flags().StringVar(&to, "to", "", "Range end value")
	cmd.Flags().StringArrayVar(&filters, "filter", nil, "Additional filter (format: 'column operator value', e.g., 'status = VALID')")
	cmd.Flags().IntVar(&limit, "limit", defaultRowLimit, "Max rows (0 = unlimited)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: ./{model}.parquet)")
	cmd.Flags().BoolVar(&upload, "upload", false, "Upload to S3 after generation")
	cmd.Flags().BoolVar(&noSanitizeIPs, "no-sanitize-ips", false, "Disable IP address sanitization (IPs are sanitized by default)")

	return cmd
}

//nolint:funlen,cyclop,gocyclo,gocognit // Command handler with interactive flow
func runGenerateSeedData(
	ctx context.Context,
	log logrus.FieldLogger,
	configPath string,
	model, network, spec, rangeColumn, from, to string,
	filterStrings []string,
	limit int,
	output string,
	upload bool,
	sanitizeIPs bool,
) error {
	// Load configuration
	labCfg, _, err := config.LoadLabConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate hybrid mode
	if labCfg.Mode != constants.ModeHybrid {
		return fmt.Errorf("this command requires hybrid mode (current mode: %s)\n"+
			"Run 'xcli lab mode hybrid' to switch to hybrid mode", labCfg.Mode)
	}

	// Create generator
	gen := seeddata.NewGenerator(log, labCfg)

	// Interactive mode: prompt for missing values
	var promptErr error

	if model == "" {
		model, promptErr = promptForModel(gen)
		if promptErr != nil {
			return promptErr
		}
	} else {
		// Validate provided model
		if validateErr := gen.ValidateModel(model); validateErr != nil {
			return validateErr
		}
	}

	if network == "" {
		network, promptErr = promptForNetwork(labCfg)
		if promptErr != nil {
			return promptErr
		}
	}

	if spec == "" {
		spec, promptErr = promptForSpec()
		if promptErr != nil {
			return promptErr
		}
	}

	// Prompt for range (optional)
	if rangeColumn == "" {
		rangeColumn, from, to, promptErr = promptForRange()
		if promptErr != nil {
			return promptErr
		}
	}

	// Parse filter strings into Filter structs
	filters, parseErr := parseFilters(filterStrings)
	if parseErr != nil {
		return parseErr
	}

	// Prompt for additional filters (interactive mode)
	if len(filterStrings) == 0 {
		additionalFilters, filterErr := promptForFilters()
		if filterErr != nil {
			return filterErr
		}

		filters = append(filters, additionalFilters...)
	}

	// Prompt for limit if not specified via flag
	if limit == defaultRowLimit {
		limit, promptErr = promptForLimit()
		if promptErr != nil {
			return promptErr
		}
	}

	// Prompt for upload if not specified
	if !upload {
		upload, promptErr = promptForUpload()
		if promptErr != nil {
			return promptErr
		}
	}

	// Prompt for S3 filename if upload is enabled
	var s3Filename string

	if upload {
		s3Filename, promptErr = promptForS3Filename(model)
		if promptErr != nil {
			return promptErr
		}

		s3Spinner := ui.NewSpinner("Checking S3 access")

		uploader, uploaderErr := seeddata.NewS3Uploader(ctx, log)
		if uploaderErr != nil {
			s3Spinner.Fail("Failed to initialize S3 client")

			return fmt.Errorf("failed to create S3 uploader: %w", uploaderErr)
		}

		if accessErr := uploader.CheckAccess(ctx); accessErr != nil {
			s3Spinner.Fail("S3 access check failed")

			return fmt.Errorf("S3 preflight check failed: %w", accessErr)
		}

		s3Spinner.Success("S3 access verified")

		// Check if object already exists
		exists, existsErr := uploader.ObjectExists(ctx, network, spec, s3Filename)
		if existsErr != nil {
			ui.Warning(fmt.Sprintf("Could not check if file exists: %v", existsErr))
		} else if exists {
			existingURL := uploader.GetPublicURL(network, spec, s3Filename)
			ui.Warning(fmt.Sprintf("File already exists: %s", existingURL))

			overwrite, confirmErr := ui.Confirm("Overwrite existing file?")
			if confirmErr != nil {
				return confirmErr
			}

			if !overwrite {
				return fmt.Errorf("upload cancelled - file already exists")
			}
		}
	}

	// Set default output path
	if output == "" {
		output = fmt.Sprintf("./%s.parquet", model)
	}

	// Generate salt for IP sanitization if enabled
	var salt string

	if sanitizeIPs {
		var saltErr error

		salt, saltErr = seeddata.GenerateSalt()
		if saltErr != nil {
			return fmt.Errorf("failed to generate salt for IP sanitization: %w", saltErr)
		}
	}

	// Generate seed data
	ui.Header("Generating seed data")

	spinner := ui.NewSpinner(fmt.Sprintf("Extracting data for %s", model))

	result, err := gen.Generate(ctx, seeddata.GenerateOptions{
		Model:       model,
		Network:     network,
		Spec:        spec,
		RangeColumn: rangeColumn,
		From:        from,
		To:          to,
		Filters:     filters,
		Limit:       limit,
		OutputPath:  output,
		SanitizeIPs: sanitizeIPs,
		Salt:        salt,
	})
	if err != nil {
		spinner.Fail("Failed to generate seed data")

		return fmt.Errorf("failed to generate seed data: %w", err)
	}

	spinner.Success(fmt.Sprintf("Written to: %s (%s)", result.OutputPath, formatFileSize(result.FileSize)))

	// Upload to S3 if requested
	var publicURL string

	if upload {
		publicURL, err = uploadToS3(ctx, log, result.OutputPath, network, spec, model, s3Filename)
		if err != nil {
			return err
		}

		// Clean up local file after successful upload
		if removeErr := os.Remove(result.OutputPath); removeErr != nil {
			ui.Warning(fmt.Sprintf("Could not remove local file: %v", removeErr))
		} else {
			ui.Info(fmt.Sprintf("Cleaned up local file: %s", result.OutputPath))
		}
	} else {
		// Use placeholder URL for template
		publicURL = fmt.Sprintf("https://%s/%s/%s/%s/%s.parquet",
			seeddata.DefaultS3PublicDomain, seeddata.DefaultS3Prefix, network, spec, model)
	}

	// Generate test YAML
	yamlFilename := s3Filename
	if yamlFilename == "" {
		yamlFilename = model
	}

	yamlContent, err := seeddata.GenerateTestYAML(seeddata.TemplateData{
		Model:    model,
		Network:  network,
		Spec:     spec,
		URL:      publicURL,
		RowCount: estimateRowCount(result.FileSize),
	})
	if err != nil {
		return fmt.Errorf("failed to generate YAML template: %w", err)
	}

	// Prompt to write YAML to xatu-cbt
	writeYAML, writeErr := ui.Confirm("Write test YAML to xatu-cbt?")
	if writeErr != nil {
		return writeErr
	}

	if writeYAML {
		yamlPath := fmt.Sprintf("%s/tests/%s/%s/models/%s.yaml",
			labCfg.Repos.XatuCBT, network, spec, yamlFilename)

		if yamlWriteErr := writeTestYAML(yamlPath, yamlContent); yamlWriteErr != nil {
			return yamlWriteErr
		}
	}

	// Display test YAML
	ui.Blank()
	ui.Header("Test YAML")

	fmt.Println(yamlContent)

	if !upload {
		ui.Blank()
		ui.Warning("File was not uploaded. Update the URL in the YAML after uploading manually.")
	}

	return nil
}

func promptForModel(gen *seeddata.Generator) (string, error) {
	models, err := gen.ListExternalModels()
	if err != nil {
		return "", fmt.Errorf("failed to list models: %w", err)
	}

	options := make([]ui.SelectOption, 0, len(models))
	for _, m := range models {
		options = append(options, ui.SelectOption{
			Label: m,
			Value: m,
		})
	}

	return ui.Select("Select a model", options)
}

func promptForNetwork(labCfg *config.LabConfig) (string, error) {
	options := make([]ui.SelectOption, 0, len(labCfg.Networks))

	for _, net := range labCfg.Networks {
		options = append(options, ui.SelectOption{
			Label: net.Name,
			Value: net.Name,
		})
	}

	return ui.Select("Select network", options)
}

func promptForSpec() (string, error) {
	options := []ui.SelectOption{
		{Label: "pectra", Value: "pectra"},
		{Label: "fusaka", Value: "fusaka"},
	}

	return ui.Select("Select spec", options)
}

func promptForRange() (column, from, to string, err error) {
	column, err = ui.TextInput("Filter by column (leave empty to skip)", "")
	if err != nil {
		return "", "", "", err
	}

	if column == "" {
		return "", "", "", nil
	}

	from, err = ui.TextInput("From value", "")
	if err != nil {
		return "", "", "", err
	}

	to, err = ui.TextInput("To value", "")
	if err != nil {
		return "", "", "", err
	}

	return column, from, to, nil
}

func promptForLimit() (int, error) {
	limitStr, err := ui.TextInput(fmt.Sprintf("Row limit [%d]", defaultRowLimit), "")
	if err != nil {
		return 0, err
	}

	if limitStr == "" {
		return defaultRowLimit, nil
	}

	var limit int

	_, err = fmt.Sscanf(limitStr, "%d", &limit)
	if err != nil {
		return 0, fmt.Errorf("invalid limit: %w", err)
	}

	return limit, nil
}

func promptForUpload() (bool, error) {
	return ui.Confirm("Upload to S3?")
}

func writeTestYAML(path, content string) error {
	spinner := ui.NewSpinner(fmt.Sprintf("Writing YAML to %s", path))

	// Ensure directory exists
	dir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(dir, 0755); mkdirErr != nil {
		spinner.Fail("Failed to create directory")

		return fmt.Errorf("failed to create directory %s: %w", dir, mkdirErr)
	}

	// Write the file (0644 is intentional - this is a config file to be committed to git)
	if writeErr := os.WriteFile(path, []byte(content), 0644); writeErr != nil { //nolint:gosec // G306: config file needs to be readable
		spinner.Fail("Failed to write YAML file")

		return fmt.Errorf("failed to write YAML to %s: %w", path, writeErr)
	}

	spinner.Success(fmt.Sprintf("Written to: %s", path))

	return nil
}

func promptForS3Filename(defaultName string) (string, error) {
	filename, err := ui.TextInput(fmt.Sprintf("S3 filename (without .parquet) [%s]", defaultName), "")
	if err != nil {
		return "", err
	}

	if filename == "" {
		return defaultName, nil
	}

	return filename, nil
}

func uploadToS3(ctx context.Context, log logrus.FieldLogger, localPath, network, spec, model, filename string) (string, error) {
	spinner := ui.NewSpinner("Uploading to S3")

	uploader, err := seeddata.NewS3Uploader(ctx, log)
	if err != nil {
		spinner.Fail("Failed to create S3 uploader")

		return "", fmt.Errorf("failed to create S3 uploader: %w", err)
	}

	result, err := uploader.Upload(ctx, seeddata.UploadOptions{
		LocalPath: localPath,
		Network:   network,
		Spec:      spec,
		Model:     model,
		Filename:  filename,
	})
	if err != nil {
		spinner.Fail("Failed to upload to S3")

		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	spinner.Success(fmt.Sprintf("Uploaded to: %s", result.PublicURL))

	return result.PublicURL, nil
}

func formatFileSize(bytes int64) string {
	const unit = 1024

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func estimateRowCount(fileSize int64) int64 {
	// Rough estimate: average 100 bytes per row in compressed parquet
	// This is a placeholder - in reality we'd need to read the parquet metadata
	return fileSize / 100
}

// parseFilters parses filter strings into Filter structs.
// Format: "column operator value" (e.g., "status = VALID").
func parseFilters(filterStrings []string) ([]seeddata.Filter, error) {
	if len(filterStrings) == 0 {
		return nil, nil
	}

	filters := make([]seeddata.Filter, 0, len(filterStrings))

	for _, s := range filterStrings {
		filter, err := parseFilterString(s)
		if err != nil {
			return nil, err
		}

		filters = append(filters, filter)
	}

	return filters, nil
}

// parseFilterString parses a single filter string.
// Supports operators: =, !=, <>, >, <, >=, <=, LIKE, NOT LIKE, IN, NOT IN.
func parseFilterString(s string) (seeddata.Filter, error) {
	// List of operators to check (longer ones first to avoid partial matches)
	operators := []string{
		"NOT LIKE", "NOT IN",
		">=", "<=", "!=", "<>",
		"LIKE", "IN",
		"=", ">", "<",
	}

	for _, op := range operators {
		idx := findOperatorIndex(s, op)
		if idx != -1 {
			column := trimSpace(s[:idx])
			value := trimSpace(s[idx+len(op):])

			if column == "" || value == "" {
				return seeddata.Filter{}, fmt.Errorf("invalid filter format: %q (expected 'column %s value')", s, op)
			}

			return seeddata.Filter{
				Column:   column,
				Operator: op,
				Value:    value,
			}, nil
		}
	}

	return seeddata.Filter{}, fmt.Errorf("invalid filter format: %q (no valid operator found)", s)
}

// findOperatorIndex finds the index of an operator in a string, case-insensitive.
func findOperatorIndex(s, op string) int {
	upper := toUpper(s)
	opUpper := toUpper(op)

	for i := 0; i <= len(upper)-len(opUpper); i++ {
		if upper[i:i+len(opUpper)] == opUpper {
			return i
		}
	}

	return -1
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}

	return s[start:end]
}

func toUpper(s string) string {
	b := make([]byte, len(s))

	for i := range len(s) {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}

		b[i] = c
	}

	return string(b)
}

// promptForFilters prompts the user to add additional filters interactively.
func promptForFilters() ([]seeddata.Filter, error) {
	var filters []seeddata.Filter

	for {
		addMore, err := ui.Confirm("Add a filter?")
		if err != nil {
			return nil, err
		}

		if !addMore {
			break
		}

		column, err := ui.TextInput("Column name", "")
		if err != nil {
			return nil, err
		}

		if column == "" {
			continue
		}

		operator, err := promptForOperator()
		if err != nil {
			return nil, err
		}

		value, err := ui.TextInput("Value", "")
		if err != nil {
			return nil, err
		}

		filters = append(filters, seeddata.Filter{
			Column:   column,
			Operator: operator,
			Value:    value,
		})
	}

	return filters, nil
}

func promptForOperator() (string, error) {
	options := []ui.SelectOption{
		{Label: "= (equals)", Value: "="},
		{Label: "!= (not equals)", Value: "!="},
		{Label: "> (greater than)", Value: ">"},
		{Label: "< (less than)", Value: "<"},
		{Label: ">= (greater or equal)", Value: ">="},
		{Label: "<= (less or equal)", Value: "<="},
		{Label: "LIKE (pattern match)", Value: "LIKE"},
		{Label: "IN (in list)", Value: "IN"},
	}

	return ui.Select("Select operator", options)
}
