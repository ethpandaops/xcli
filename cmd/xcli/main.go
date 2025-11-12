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
		Short:   "Local development orchestration tool for ethPandaOps lab stack",
		Long:    `xcli orchestrates the complete local development environment for the lab stack, including ClickHouse, CBT, cbt-api, lab-backend, and lab frontend.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	}

	// Global flags
	var configPath string
	var logLevel string

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

	// Add commands
	rootCmd.AddCommand(commands.NewInitCommand(log))
	rootCmd.AddCommand(commands.NewBuildCommand(log, configPath))
	rootCmd.AddCommand(commands.NewUpCommand(log, configPath))
	rootCmd.AddCommand(commands.NewDownCommand(log, configPath))
	rootCmd.AddCommand(commands.NewPsCommand(log, configPath))
	rootCmd.AddCommand(commands.NewLogsCommand(log, configPath))
	rootCmd.AddCommand(commands.NewRestartCommand(log, configPath))
	rootCmd.AddCommand(commands.NewStatusCommand(log, configPath))
	rootCmd.AddCommand(commands.NewModeCommand(log, configPath))
	rootCmd.AddCommand(commands.NewConfigCommand(log, configPath))

	// Execute
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.WithError(err).Error("Command failed")
		os.Exit(1)
	}
}
