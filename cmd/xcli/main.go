package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ethpandaops/xcli/pkg/commands"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

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

	// Setup logger
	log := logrus.New()
	log.SetOutput(os.Stdout)
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Create root command
	rootCmd := &cobra.Command{
		Use:     "xcli",
		Short:   "Local development orchestration tool for ethPandaOps",
		Long:    `xcli orchestrates local development environments for ethPandaOps projects.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	}

	// Global flags
	var (
		configPath string
		logLevel   string
	)

	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", ".xcli.yaml", "Path to config file")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")

	// Parse log level
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		level, err := logrus.ParseLevel(logLevel)
		if err != nil {
			return fmt.Errorf("invalid log level: %w", err)
		}

		log.SetLevel(level)

		return nil
	}

	// Add stack commands
	rootCmd.AddCommand(commands.NewLabCommand(log, configPath))

	// Add global config command (shows all stack configs)
	rootCmd.AddCommand(commands.NewConfigCommand(log, configPath))

	// Execute
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.WithError(err).Error("Command failed")
		os.Exit(1)
	}
}
