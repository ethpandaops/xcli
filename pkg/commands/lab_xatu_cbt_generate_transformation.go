package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		duration      string
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
				model, network, spec, rangeColumn, from, to, limit, upload, aiAssertions, skipExisting, !noSanitizeIPs, duration)
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
	cmd.Flags().StringVar(&duration, "duration", "", "Time range duration (e.g., 1m, 5m, 10m, 30m)")

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
	duration string,
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

	// AI-assisted range discovery
	ui.Blank()
	ui.Header("Analyzing range strategies")

	// Prompt for duration if not specified
	if duration == "" {
		durationOpts := []ui.SelectOption{
			{Label: "5m", Description: "recommended", Value: "5m"},
			{Label: "30s", Description: "minimal test", Value: "30s"},
			{Label: "1m", Description: "quick test", Value: "1m"},
			{Label: "10m", Description: "", Value: "10m"},
			{Label: "30m", Description: "", Value: "30m"},
			{Label: "1h", Description: "large dataset", Value: "1h"},
		}

		selectedDuration, durationErr := ui.Select("Time range duration", durationOpts)
		if durationErr != nil {
			return durationErr
		}

		duration = selectedDuration
	}

	ui.Info(fmt.Sprintf("Using %s time range", duration))
	ui.Info("This may take a few minutes for models with many dependencies - grab a coffee ☕")

	var discoveryResult *seeddata.DiscoveryResult

	// Try AI discovery first
	discoveryClient, discoveryErr := seeddata.NewClaudeDiscoveryClient(log, gen)
	if discoveryErr != nil {
		ui.Warning(fmt.Sprintf("Claude CLI not available: %v", discoveryErr))
		ui.Info("Falling back to heuristic range detection")

		// Fallback to heuristic detection
		var rangeInfos map[string]*seeddata.RangeColumnInfo

		rangeInfos, err = seeddata.DetectRangeColumnsForModels(externalModels, labCfg.Repos.XatuCBT)
		if err != nil {
			return fmt.Errorf("failed to detect range columns: %w", err)
		}

		discoveryResult, err = seeddata.FallbackRangeDiscovery(ctx, gen, externalModels, network, rangeInfos, duration)
		if err != nil {
			return fmt.Errorf("fallback range discovery failed: %w", err)
		}
	} else {
		// Gather schema information
		schemaSpinner := ui.NewSpinner("Gathering schema information")

		schemaInfo, schemaErr := discoveryClient.GatherSchemaInfo(ctx, externalModels, network, labCfg.Repos.XatuCBT)
		if schemaErr != nil {
			schemaSpinner.Fail("Failed to gather schema info")

			return fmt.Errorf("failed to gather schema info: %w", schemaErr)
		}

		schemaSpinner.Success(fmt.Sprintf("Schema info gathered for %d models", len(schemaInfo)))

		// Display detected range info
		for _, schema := range schemaInfo {
			if schema.RangeInfo != nil {
				status := "detected"
				if !schema.RangeInfo.Detected {
					status = "default"
				}

				rangeStr := ""
				if schema.RangeInfo.MinValue != "" && schema.RangeInfo.MaxValue != "" {
					rangeStr = fmt.Sprintf(" [%s → %s]", schema.RangeInfo.MinValue, schema.RangeInfo.MaxValue)
				}

				ui.Info(fmt.Sprintf("  • %s: %s (%s)%s", schema.Model, schema.RangeInfo.Column, status, rangeStr))
			}
		}

		// Read transformation SQL
		transformationSQL, sqlErr := seeddata.ReadTransformationSQL(model, labCfg.Repos.XatuCBT)
		if sqlErr != nil {
			return fmt.Errorf("failed to read transformation SQL: %w", sqlErr)
		}

		// Invoke Claude for analysis
		ui.Blank()

		analysisSpinner := ui.NewSpinner("Analyzing correlation strategy with Claude")

		discoveryResult, err = discoveryClient.AnalyzeRanges(ctx, seeddata.DiscoveryInput{
			TransformationModel: model,
			TransformationSQL:   transformationSQL,
			Network:             network,
			Duration:            duration,
			ExternalModels:      schemaInfo,
		})
		if err != nil {
			analysisSpinner.Fail("AI analysis failed")
			ui.Warning(fmt.Sprintf("Claude analysis failed: %v", err))
			ui.Info("Falling back to heuristic range detection")

			// Fallback to heuristic detection
			rangeInfos, rangeErr := seeddata.DetectRangeColumnsForModels(externalModels, labCfg.Repos.XatuCBT)
			if rangeErr != nil {
				return fmt.Errorf("failed to detect range columns: %w", rangeErr)
			}

			discoveryResult, err = seeddata.FallbackRangeDiscovery(ctx, gen, externalModels, network, rangeInfos, duration)
			if err != nil {
				return fmt.Errorf("fallback range discovery failed: %w", err)
			}
		} else {
			analysisSpinner.Success(fmt.Sprintf("Strategy generated (confidence: %.0f%%)", discoveryResult.OverallConfidence*100))
		}
	}

	// Validate that Claude's strategies cover all expected models
	// This catches cases where Claude named a model differently
	var missingModels []string

	for _, extModel := range externalModels {
		if discoveryResult.GetStrategy(extModel) == nil {
			missingModels = append(missingModels, extModel)
		}
	}

	if len(missingModels) > 0 {
		ui.Blank()
		ui.Warning("The following models are NOT covered by Claude's strategy:")

		for _, m := range missingModels {
			ui.Warning(fmt.Sprintf("  • %s", m))
		}

		ui.Warning("These will use the primary range column, which may be incorrect.")
		ui.Info("Claude's strategies cover these models:")

		for _, s := range discoveryResult.Strategies {
			ui.Info(fmt.Sprintf("  • %s", s.Model))
		}

		ui.Blank()

		proceedMissing, missErr := ui.Confirm("Proceed anyway?")
		if missErr != nil {
			return missErr
		}

		if !proceedMissing {
			ui.Info("Aborted. Try regenerating with clearer model names.")

			return nil
		}
	}

	// Display the proposed strategy
	ui.Blank()
	ui.Header("Proposed Strategy")
	ui.Info(fmt.Sprintf("Summary: %s", discoveryResult.Summary))
	ui.Blank()
	ui.Info(fmt.Sprintf("Primary Range: %s (%s)", discoveryResult.PrimaryRangeColumn, discoveryResult.PrimaryRangeType))
	ui.Info(fmt.Sprintf("  From: %s", discoveryResult.FromValue))
	ui.Info(fmt.Sprintf("  To:   %s", discoveryResult.ToValue))
	ui.Blank()

	ui.Info("Per-Table Strategies:")

	for _, strategy := range discoveryResult.Strategies {
		confidence := fmt.Sprintf("%.0f%%", strategy.Confidence*100)
		bridgeInfo := ""

		if strategy.RequiresBridge {
			bridgeInfo = fmt.Sprintf(" (via %s)", strategy.BridgeTable)
		}

		ui.Info(fmt.Sprintf("  • %s: %s [%s → %s] %s%s",
			strategy.Model,
			strategy.RangeColumn,
			strategy.FromValue,
			strategy.ToValue,
			confidence,
			bridgeInfo,
		))
	}

	// Display warnings
	if len(discoveryResult.Warnings) > 0 {
		ui.Blank()

		for _, warning := range discoveryResult.Warnings {
			ui.Warning(warning)
		}
	}

	// Warn if low confidence
	if discoveryResult.OverallConfidence < 0.5 {
		ui.Blank()
		ui.Warning("Low confidence score - manual review recommended")
	}

	// Validate that each model has data in the proposed range
	ui.Blank()
	ui.Header("Validating data availability")

	validationSpinner := ui.NewSpinner("Checking row counts for each model")

	validation, validationErr := gen.ValidateStrategyHasData(ctx, discoveryResult, network)
	if validationErr != nil {
		validationSpinner.Fail("Validation failed")

		return fmt.Errorf("failed to validate strategy: %w", validationErr)
	}

	validationSpinner.Success("Validation complete")

	// Display row counts
	ui.Blank()
	ui.Info("Data availability per model:")

	for _, count := range validation.Counts {
		status := "✓"
		if !count.HasData {
			status = "✗"
		}

		if count.Error != nil {
			ui.Warning(fmt.Sprintf("  %s %s: error - %v", status, count.Model, count.Error))
		} else {
			ui.Info(fmt.Sprintf("  %s %s: %d rows", status, count.Model, count.RowCount))
		}
	}

	ui.Blank()
	ui.Info(fmt.Sprintf("Total rows across all models: %d", validation.TotalRows))

	if validation.MinRowModel != "" {
		ui.Info(fmt.Sprintf("Model with fewest rows: %s (%d rows)", validation.MinRowModel, validation.MinRowCount))
	}

	// Handle errored models (timeouts, etc.)
	if len(validation.ErroredModels) > 0 {
		ui.Blank()
		ui.Error("The following models FAILED to query (timeout or error):")

		for _, model := range validation.ErroredModels {
			ui.Error(fmt.Sprintf("  • %s", model))
		}

		ui.Blank()
		ui.Warning("These queries timed out - the tables may be too large or the range too wide.")
		ui.Warning("Consider narrowing the block/time range, or proceed if you believe the data exists.")

		proceedWithErrors, errErr := ui.Confirm("Proceed anyway (assuming data exists)?")
		if errErr != nil {
			return errErr
		}

		if !proceedWithErrors {
			ui.Info("Aborted by user. Try a narrower range.")

			return nil
		}
	}

	// Handle empty models (zero rows)
	if len(validation.EmptyModels) > 0 {
		ui.Blank()
		ui.Error("The following models have NO DATA in the proposed range:")

		for _, model := range validation.EmptyModels {
			ui.Error(fmt.Sprintf("  • %s", model))
		}

		ui.Blank()
		ui.Warning("Empty parquets will be generated for these models, which may cause test failures.")

		expandWindow, expandErr := ui.Confirm("Would you like to expand the time window and retry?")
		if expandErr != nil {
			return expandErr
		}

		if expandWindow {
			ui.Info("Please re-run the command with a larger time window or different range.")
			ui.Info("Tip: Some tables (like canonical_execution_contracts) may have sparse data.")

			return nil
		}

		// Let user proceed anyway if they want
		proceedAnyway, proceedErr := ui.Confirm("Proceed anyway with potentially empty data?")
		if proceedErr != nil {
			return proceedErr
		}

		if !proceedAnyway {
			ui.Info("Aborted by user")

			return nil
		}
	}

	// User confirmation
	ui.Blank()

	proceed, confirmErr := ui.Confirm("Proceed with this strategy?")
	if confirmErr != nil {
		return confirmErr
	}

	if !proceed {
		ui.Info("Aborted by user")

		return nil
	}

	// Row limit handling:
	// - With AI discovery: use unlimited (0) since Claude already picked sensible ranges
	// - Manual/fallback: prompt for limit to avoid accidentally pulling too much data
	// - Explicit --limit flag always respected
	if discoveryResult != nil && limit == defaultRowLimit {
		// AI discovery mode: no limit needed, Claude picked appropriate ranges
		limit = 0

		ui.Info("Using unlimited rows (AI discovery already optimized the range)")
	} else if limit == defaultRowLimit {
		// Fallback/manual mode: prompt for safety
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
		// Get the strategy for this model
		strategy := discoveryResult.GetStrategy(extModel)
		if strategy == nil {
			ui.Warning(fmt.Sprintf("No strategy found for %s, using defaults", extModel))

			// Detect the correct range column for this model instead of blindly using primary
			detectedCol, detectErr := seeddata.DetectRangeColumnForModel(extModel, labCfg.Repos.XatuCBT)
			if detectErr != nil {
				ui.Warning(fmt.Sprintf("Could not detect range column for %s: %v", extModel, detectErr))

				detectedCol = seeddata.DefaultRangeColumn
			}

			// Check if detected column type matches primary range type
			colLower := strings.ToLower(detectedCol)
			isTimeColumn := strings.Contains(colLower, "date") || strings.Contains(colLower, "time")
			primaryIsTime := discoveryResult.PrimaryRangeType == seeddata.RangeColumnTypeTime

			if isTimeColumn != primaryIsTime {
				// Column types don't match - we can't use primary range values
				ui.Error(fmt.Sprintf("  %s uses %s but primary range is %s - cannot convert automatically",
					extModel, detectedCol, discoveryResult.PrimaryRangeColumn))
				ui.Error("  Please re-run with Claude to get proper correlation, or use --range-column to override")

				return fmt.Errorf("cannot generate %s: range column type mismatch", extModel)
			}

			strategy = &seeddata.TableRangeStrategy{
				Model:       extModel,
				RangeColumn: detectedCol,
				FromValue:   discoveryResult.FromValue,
				ToValue:     discoveryResult.ToValue,
			}
		}

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

		// Show query parameters (helps debug empty parquets)
		ui.Info(fmt.Sprintf("  %s: %s [%s → %s]", extModel, strategy.RangeColumn, strategy.FromValue, strategy.ToValue))

		genSpinner := ui.NewSpinner(fmt.Sprintf("Generating %s", extModel))

		result, genErr := gen.Generate(ctx, seeddata.GenerateOptions{
			Model:       extModel,
			Network:     network,
			Spec:        spec,
			RangeColumn: strategy.RangeColumn,
			From:        strategy.FromValue,
			To:          strategy.ToValue,
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

		// Warn if file is too large for comfortable test imports
		const largeFileThreshold = 15 * 1024 * 1024 // 15MB
		if result.FileSize > largeFileThreshold {
			ui.Warning(fmt.Sprintf("  Large file (%s) - may slow down tests on low-powered machines. Consider using a shorter duration.",
				formatFileSize(result.FileSize)))
		}

		// Show query for first model to help debug empty parquets
		if extModel == externalModels[0] {
			ui.Info(fmt.Sprintf("  Query: %s", result.Query))
		}

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
