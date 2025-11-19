package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ethpandaops/xcli/pkg/autoupgrade"
	"github.com/ethpandaops/xcli/pkg/commands"
	"github.com/ethpandaops/xcli/pkg/config"
	"github.com/ethpandaops/xcli/pkg/ui"
	"github.com/ethpandaops/xcli/pkg/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Build-time variables set via ldflags.
var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

func init() {
	// Set package-level version variables from build flags
	version.Version = buildVersion
	version.Commit = buildCommit
	version.Date = buildDate
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	// Setup logger with conditional writer
	// Logs are hidden by default and only shown when --verbose is enabled
	logWriter := ui.NewConditionalWriter(os.Stdout, false)
	log := logrus.New()
	log.SetOutput(logWriter)
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Check for updates and auto-upgrade if needed (runs quickly due to caching)
	if repoPath := findRepoPath(); repoPath != "" {
		upgrader := autoupgrade.NewService(log, repoPath, version.Commit)
		if err := upgrader.CheckAndUpgrade(ctx); err != nil {
			log.WithError(err).Debug("Auto-upgrade check failed")
		}
	}

	// Create root command
	rootCmd := &cobra.Command{
		Use:     "xcli",
		Short:   "Local development orchestration tool for ethPandaOps",
		Long:    `xcli orchestrates local development environments for ethPandaOps projects.`,
		Version: version.GetFullVersion(),
	}

	// Global flags
	var (
		configPath string
		logLevel   string
		verbose    bool
	)

	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", ".xcli.yaml", "Path to config file")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output (show all logs)")

	// Parse log level and configure verbose mode
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		level, err := logrus.ParseLevel(logLevel)
		if err != nil {
			return fmt.Errorf("invalid log level: %w", err)
		}

		log.SetLevel(level)

		// Enable log writer based on verbose flag
		logWriter.SetEnabled(verbose)

		return nil
	}

	// Add root-level commands
	rootCmd.AddCommand(commands.NewInitCommand(log, configPath))
	rootCmd.AddCommand(commands.NewConfigCommand(log, configPath))

	// Add stack commands
	rootCmd.AddCommand(commands.NewLabCommand(log, configPath))

	// Execute
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.WithError(err).Error("Command failed")
		os.Exit(1)
	}
}

// findRepoPath attempts to locate the xcli repository for auto-updates.
func findRepoPath() string {
	// Load global config to get xcli path
	globalCfg, err := config.LoadGlobalConfig()
	if err != nil {
		return ""
	}

	// Verify the path has a .git directory
	if globalCfg.XCLIPath != "" {
		gitDir := filepath.Join(globalCfg.XCLIPath, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return globalCfg.XCLIPath
		}
	}

	return ""
}
