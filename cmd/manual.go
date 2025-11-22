/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"os"
	"os/signal"
	"reconswarm/internal/config"
	"reconswarm/internal/logging"
	"reconswarm/internal/recon"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// manualCmd represents the manual command
var manualCmd = &cobra.Command{
	Use:   "manual",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load configuration
		logging.Logger().Info("Loading configuration")
		cfg, err := config.Load()
		if err != nil {
			logging.Logger().Fatal("Failed to load configuration", zap.Error(err))
		}

		// Create context that listens for signals
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			logging.Logger().Info("Received interrupt signal, shutting down gracefully...")
			cancel()
		}()

		if err := recon.Run(ctx, *cfg); err != nil {
			if ctx.Err() != nil {
				logging.Logger().Info("Pipeline interrupted by user")
			} else {
				logging.Logger().Fatal("pipeline failed", zap.Error(err))
			}
		} else {
			logging.Logger().Info("manual pipeline finished")
		}
	},
}

func init() {
	rootCmd.AddCommand(manualCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// manualCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// manualCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
