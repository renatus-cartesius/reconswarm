/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"reconswarm/internal/config"
	"reconswarm/internal/logging"
	"reconswarm/internal/recon"

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

		ctx := context.Background()
		if err := recon.Run(ctx, *cfg); err != nil {
			logging.Logger().Fatal("pipeline failed", zap.Error(err))
		}

		logging.Logger().Info("manual pipeline finished")
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
