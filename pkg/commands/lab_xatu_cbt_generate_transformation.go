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

// NewLabXatuCBTGenerateTransformationTestCommand creates the command.
func NewLabXatuCBTGenerateTransformationTestCommand(log logrus.FieldLogger, configPath string) *cobra.Command {
	var (
		model         string
		network       string
		spec          string
		rangeColumn   string
		from          string
		to            string
		limit         int
		upload        bool
		aiAssertions  bool
		skipExisting  bool
		noSanitizeIPs bool
	)

	cmd := &cobra.Command{
		Use:   "generate-transformation-test",
		Short: "Generate test YAML for transformation models",
		Long: `Generate complete test YAML files for xatu-cbt transformation models.

This command:
1. Resolves the full dependency tree for a transformation model
2. Identifies all external model dependencies (leaf nodes)
3. Queries external ClickHouse for available data ranges
4. Finds the intersecting range across all dependencies
5. Generates seed data parquet files for all external models
6. Optionally uses Claude to generate meaningful assertions
7. Writes the complete test YAML to xatu-cbt

This command requires hybrid mode to be enabled.

Interactive mode:
  xcli lab xatu-cbt generate-transformation-test

Scripted mode:
  xcli lab xatu-cbt generate-transformation-test \
    --model fct_data_column_availability_by_slot \
    --network sepolia \
    --spec fusaka \
    --range-column slot_start_date_time \
    --from "2025-10-27 00:26:00" \
    --to "2025-10-27 00:30:00" \
    --upload \
    --ai-assertions

S3 Upload Configuration (defaults to Cloudflare R2):
  AWS_ACCESS_KEY_ID      R2 API token Access Key ID
  AWS_SECRET_ACCESS_KEY  R2 API token Secret Access Key
  S3_ENDPOINT            Override endpoint (default: ethpandaops R2)
  S3_BUCKET              Override bucket (default: ethpandaops-platform-production-public)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerateTransformationTest(cmd.Context(), log, configPath,
				model, network, spec, rangeColumn, from, to, limit, upload, aiAssertions, skipExisting, !noSanitizeIPs)
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "Transformation model name")
	cmd.Flags().StringVar(&network, "network", "", "Network name (mainnet, sepolia, etc.)")
	cmd.Flags().StringVar(&spec, "spec", "", "Fork spec (pectra, fusaka, etc.)")
	cmd.Flags().StringVar(&rangeColumn, "range-column", "", "Override detected range column")
	cmd.Flags().StringVar(&from, "from", "", "Range start value")
	cmd.Flags().StringVar(&to, "to", "", "Range end value")
	cmd.Flags().IntVar(&limit, "limit", defaultRowLimit, "Max rows per external model (0 = unlimited)")
	cmd.Flags().BoolVar(&upload, "upload", false, "Upload parquets to S3 after generation")
	cmd.Flags().BoolVar(&aiAssertions, "ai-assertions", false, "Use Claude to generate assertions")
	cmd.Flags().BoolVar(&skipExisting, "skip-existing", false, "Skip generating seed data for existing S3 files")
	cmd.Flags().BoolVar(&noSanitizeIPs, "no-sanitize-ips", false, "Disable IP address sanitization (IPs are sanitized by default)")

	return cmd
}

//nolint:funlen,cyclop,gocyclo,gocognit // Command handler with interactive flow
func runGenerateTransformationTest(
	ctx context.Context,
	log logrus.FieldLogger,
	configPath string,
	model, network, spec, rangeColumn, from, to string,
	limit int,
	upload, aiAssertions, skipExisting, sanitizeIPs bool,
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

	// Interactive mode: prompt for model
	var promptErr error

	if model == "" {
		model, promptErr = promptForTransformationModel(labCfg.Repos.XatuCBT)
		if promptErr != nil {
			return promptErr
		}
	}

	// Resolve dependency tree
	ui.Header("Resolving dependencies")

	depSpinner := ui.NewSpinner(fmt.Sprintf("Analyzing %s", model))

	tree, err := seeddata.ResolveDependencyTree(model, labCfg.Repos.XatuCBT, nil)
	if err != nil {
		depSpinner.Fail("Failed to resolve dependencies")

		return fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	depSpinner.Success("Dependency tree resolved")

	// Display dependency tree
	ui.Blank()
	fmt.Println(tree.PrintTree("  "))

	// Get external dependencies
	externalModels := tree.GetExternalDependencies()
	if len(externalModels) == 0 {
		return fmt.Errorf("no external dependencies found for %s", model)
	}

	ui.Info(fmt.Sprintf("External models needed (%d):", len(externalModels)))

	for _, m := range externalModels {
		fmt.Printf("  • %s\n", m)
	}

	ui.Blank()

	// Prompt for network
	if network == "" {
		network, promptErr = promptForNetwork(labCfg)
		if promptErr != nil {
			return promptErr
		}
	}

	// Prompt for spec
	if spec == "" {
		spec, promptErr = promptForSpec()
		if promptErr != nil {
			return promptErr
		}
	}

	// Detect range columns for external models
	ui.Header("Detecting range columns")

	rangeInfos, err := seeddata.DetectRangeColumnsForModels(externalModels, labCfg.Repos.XatuCBT)
	if err != nil {
		return fmt.Errorf("failed to detect range columns: %w", err)
	}

	// Track detection status for prompting
	anyDefault := false
	detectedColumns := make(map[string]bool)

	for _, info := range rangeInfos {
		status := "detected"
		if !info.Detected {
			status = "default"
			anyDefault = true
		}

		detectedColumns[info.RangeColumn] = true
		ui.Info(fmt.Sprintf("  • %s: %s (%s)", info.Model, info.RangeColumn, status))
	}

	// Use common range column or override
	if rangeColumn == "" {
		rangeColumn = seeddata.FindCommonRangeColumn(rangeInfos)

		// Prompt user if detection used defaults or found mismatches
		shouldPrompt := anyDefault || len(detectedColumns) > 1

		if shouldPrompt {
			if len(detectedColumns) > 1 {
				ui.Warning("Different range columns detected across models")
			}

			rangeColumn, promptErr = promptForRangeColumn(rangeColumn)
			if promptErr != nil {
				return promptErr
			}
		}

		ui.Info(fmt.Sprintf("Using range column: %s", rangeColumn))
	}

	// Query available ranges
	ui.Blank()
	ui.Header("Querying available data ranges")

	rangeSpinner := ui.NewSpinner("Querying external ClickHouse")

	ranges, err := gen.QueryModelRanges(ctx, externalModels, network, rangeInfos, rangeColumn)
	if err != nil {
		rangeSpinner.Fail("Failed to query ranges")

		return fmt.Errorf("failed to query ranges: %w", err)
	}

	rangeSpinner.Success("Range data retrieved")

	for _, r := range ranges {
		ui.Info(fmt.Sprintf("  • %s: %s", r.Model, r.FormatRange()))
	}

	// Find intersection
	intersection, err := seeddata.FindIntersection(ranges)
	if err != nil {
		ui.Error("No intersecting range found across all models")

		return fmt.Errorf("range intersection failed: %w", err)
	}

	ui.Blank()
	ui.Success(fmt.Sprintf("Intersecting range: %s", intersection.FormatRange()))

	// Prompt for range within intersection
	if from == "" || to == "" {
		from, to, promptErr = promptForRangeWithinIntersection(intersection)
		if promptErr != nil {
			return promptErr
		}
	}

	// Prompt for limit
	if limit == defaultRowLimit {
		limit, promptErr = promptForLimit()
		if promptErr != nil {
			return promptErr
		}
	}

	// Prompt for upload
	if !upload {
		upload, promptErr = ui.Confirm("Upload to S3?")
		if promptErr != nil {
			return promptErr
		}
	}

	// S3 preflight check if uploading
	var uploader *seeddata.S3Uploader

	if upload {
		s3Spinner := ui.NewSpinner("Checking S3 access")

		uploader, err = seeddata.NewS3Uploader(ctx, log)
		if err != nil {
			s3Spinner.Fail("Failed to initialize S3 client")

			return fmt.Errorf("failed to create S3 uploader: %w", err)
		}

		if accessErr := uploader.CheckAccess(ctx); accessErr != nil {
			s3Spinner.Fail("S3 access check failed")

			return fmt.Errorf("S3 preflight check failed: %w", accessErr)
		}

		s3Spinner.Success("S3 access verified")
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

	// Generate seed data for all external models
	ui.Blank()
	ui.Header("Generating seed data")

	urls := make(map[string]string, len(externalModels))

	for _, extModel := range externalModels {
		filename := seeddata.GetParquetFilename(model, extModel)
		outputPath := fmt.Sprintf("./%s", filename)

		// Check if we should skip existing
		if upload && skipExisting && uploader != nil {
			exists, existsErr := uploader.ObjectExists(ctx, network, spec, filename[:len(filename)-8]) // Remove .parquet
			if existsErr == nil && exists {
				ui.Info(fmt.Sprintf("  ⏭ Skipping %s (already exists)", extModel))

				urls[extModel] = uploader.GetPublicURL(network, spec, filename[:len(filename)-8])

				continue
			}
		}

		genSpinner := ui.NewSpinner(fmt.Sprintf("Generating %s", extModel))

		result, genErr := gen.Generate(ctx, seeddata.GenerateOptions{
			Model:       extModel,
			Network:     network,
			Spec:        spec,
			RangeColumn: rangeColumn,
			From:        from,
			To:          to,
			Limit:       limit,
			OutputPath:  outputPath,
			SanitizeIPs: sanitizeIPs,
			Salt:        salt,
		})
		if genErr != nil {
			genSpinner.Fail(fmt.Sprintf("Failed to generate %s", extModel))

			return fmt.Errorf("failed to generate seed data for %s: %w", extModel, genErr)
		}

		genSpinner.Success(fmt.Sprintf("%s (%s)", extModel, formatFileSize(result.FileSize)))

		// Display sanitized columns if any
		if len(result.SanitizedColumns) > 0 {
			ui.Info(fmt.Sprintf("  Sanitized IP columns: %v", result.SanitizedColumns))
		}

		// Upload if requested
		if upload && uploader != nil {
			uploadSpinner := ui.NewSpinner(fmt.Sprintf("Uploading %s", extModel))

			uploadResult, uploadErr := uploader.Upload(ctx, seeddata.UploadOptions{
				LocalPath: outputPath,
				Network:   network,
				Spec:      spec,
				Model:     extModel,
				Filename:  filename[:len(filename)-8], // Remove .parquet extension
			})
			if uploadErr != nil {
				uploadSpinner.Fail(fmt.Sprintf("Failed to upload %s", extModel))

				return fmt.Errorf("failed to upload %s: %w", extModel, uploadErr)
			}

			uploadSpinner.Success(fmt.Sprintf("Uploaded %s", extModel))
			ui.Info(fmt.Sprintf("  → %s", uploadResult.PublicURL))

			urls[extModel] = uploadResult.PublicURL

			// Clean up local file
			if removeErr := os.Remove(outputPath); removeErr != nil {
				ui.Warning(fmt.Sprintf("Could not remove local file: %v", removeErr))
			}
		} else {
			// Use placeholder URL
			urls[extModel] = fmt.Sprintf("https://%s/%s/%s/%s/%s",
				seeddata.DefaultS3PublicDomain, seeddata.DefaultS3Prefix, network, spec, filename)
		}
	}

	// Generate assertions
	var assertions []seeddata.Assertion

	if aiAssertions {
		assertions, err = generateAIAssertions(ctx, log, model, externalModels, labCfg.Repos.XatuCBT)
		if err != nil {
			ui.Warning(fmt.Sprintf("AI assertion generation failed: %v", err))
			ui.Info("Using default assertions instead")

			assertions = seeddata.GetDefaultAssertions(model)
		}
	} else {
		// Prompt for AI assertions
		useAI, confirmErr := ui.Confirm("Generate assertions with Claude?")
		if confirmErr != nil {
			return confirmErr
		}

		if useAI {
			assertions, err = generateAIAssertions(ctx, log, model, externalModels, labCfg.Repos.XatuCBT)
			if err != nil {
				ui.Warning(fmt.Sprintf("AI assertion generation failed: %v", err))

				assertions = seeddata.GetDefaultAssertions(model)
			}
		} else {
			assertions = seeddata.GetDefaultAssertions(model)
		}
	}

	// Generate test YAML
	ui.Blank()
	ui.Header("Generating test YAML")

	yamlContent, err := seeddata.GenerateTransformationTestYAML(seeddata.TransformationTemplateData{
		Model:          model,
		Network:        network,
		Spec:           spec,
		ExternalModels: externalModels,
		URLs:           urls,
		Assertions:     assertions,
	})
	if err != nil {
		return fmt.Errorf("failed to generate YAML: %w", err)
	}

	// Prompt to write YAML to xatu-cbt
	writeYAML, writeErr := ui.Confirm("Write test YAML to xatu-cbt?")
	if writeErr != nil {
		return writeErr
	}

	if writeYAML {
		yamlPath := filepath.Join(labCfg.Repos.XatuCBT, "tests", network, spec, "models", model+".yaml")

		if yamlWriteErr := writeTestYAML(yamlPath, yamlContent); yamlWriteErr != nil {
			return yamlWriteErr
		}
	}

	// Display test command
	ui.Blank()
	ui.Header("Test Command")

	testCmd := fmt.Sprintf("./bin/xatu-cbt test models %s --spec %s --network %s --verbose --force-rebuild",
		model, spec, network)
	fmt.Println(testCmd)

	// Display test YAML
	ui.Blank()
	ui.Header("Test YAML")

	fmt.Println(yamlContent)

	if !upload {
		ui.Blank()
		ui.Warning("Files were not uploaded. Update the URLs in the YAML after uploading manually.")
	}

	return nil
}

func promptForTransformationModel(xatuCBTPath string) (string, error) {
	models, err := seeddata.ListTransformationModels(xatuCBTPath)
	if err != nil {
		return "", fmt.Errorf("failed to list transformation models: %w", err)
	}

	options := make([]ui.SelectOption, 0, len(models))

	for _, m := range models {
		options = append(options, ui.SelectOption{
			Label: m,
			Value: m,
		})
	}

	return ui.Select("Select transformation model", options)
}

func promptForRangeColumn(defaultColumn string) (string, error) {
	// Don't pre-fill input - pterm concatenates instead of replacing
	// Show default in prompt, user can press enter to accept
	column, err := ui.TextInput(
		fmt.Sprintf("Range column [%s]", defaultColumn),
		"")
	if err != nil {
		return "", err
	}

	if column == "" {
		return defaultColumn, nil
	}

	return column, nil
}

func promptForRangeWithinIntersection(intersection *seeddata.ModelRange) (string, string, error) {
	ui.Info(fmt.Sprintf("Enter range within intersection (%s to %s)",
		intersection.Min.Format("2006-01-02 15:04:05"),
		intersection.Max.Format("2006-01-02 15:04:05")))

	defaultFrom := intersection.Min.Format("2006-01-02 15:04:05")
	defaultTo := intersection.Max.Format("2006-01-02 15:04:05")

	// Don't pre-fill input - pterm concatenates instead of replacing
	// Show default in prompt, user can press enter to accept
	from, err := ui.TextInput(
		fmt.Sprintf("From [%s]", defaultFrom),
		"")
	if err != nil {
		return "", "", err
	}

	if from == "" {
		from = defaultFrom
	}

	to, err := ui.TextInput(
		fmt.Sprintf("To [%s]", defaultTo),
		"")
	if err != nil {
		return "", "", err
	}

	if to == "" {
		to = defaultTo
	}

	return from, to, nil
}

func generateAIAssertions(ctx context.Context, log logrus.FieldLogger, model string, externalModels []string, xatuCBTPath string) ([]seeddata.Assertion, error) {
	aiSpinner := ui.NewSpinner("Analyzing transformation SQL with Claude")

	client, err := seeddata.NewClaudeAssertionClient(log)
	if err != nil {
		aiSpinner.Fail("Claude CLI not available")

		return nil, fmt.Errorf("claude CLI not available: %w", err)
	}

	// Read transformation SQL
	sqlPath := filepath.Join(xatuCBTPath, "models", "transformations", model+".sql")

	sqlContent, err := os.ReadFile(sqlPath)
	if err != nil {
		aiSpinner.Fail("Failed to read transformation SQL")

		return nil, fmt.Errorf("failed to read SQL file: %w", err)
	}

	assertions, err := client.GenerateAssertions(ctx, string(sqlContent), externalModels, model)
	if err != nil {
		aiSpinner.Fail("Claude assertion generation failed")

		return nil, err
	}

	aiSpinner.Success(fmt.Sprintf("Generated %d assertions", len(assertions)))

	return assertions, nil
}
