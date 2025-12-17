/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"os"
	"reconswarm/internal/config"
	"reconswarm/internal/logging"
	"reconswarm/internal/pipeline"
	"reconswarm/internal/recon"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

var manualPipelineFile string

// manualCmd represents the manual command
var manualCmd = &cobra.Command{
	Use:   "manual",
	Short: "Run a pipeline manually (without gRPC server)",
	Long: `Run a reconnaissance pipeline directly without going through the gRPC server.
This is useful for testing and debugging pipelines locally.

Example:
  reconswarm manual -f examples/pipelines/nuclei.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		if manualPipelineFile == "" {
			logging.Logger().Fatal("Pipeline file is required. Use -f flag.")
		}

		// Load server configuration
		logging.Logger().Info("Loading configuration")
		cfg, err := config.Load()
		if err != nil {
			logging.Logger().Fatal("Failed to load configuration", zap.Error(err))
		}

		// Load pipeline from file
		logging.Logger().Info("Loading pipeline", zap.String("file", manualPipelineFile))
		pipelineData, err := os.ReadFile(manualPipelineFile)
		if err != nil {
			logging.Logger().Fatal("Failed to read pipeline file", zap.Error(err))
		}

		var pipelineRaw pipeline.PipelineRaw
		if err := yaml.Unmarshal(pipelineData, &pipelineRaw); err != nil {
			logging.Logger().Fatal("Failed to parse pipeline YAML", zap.Error(err))
		}

		ctx := context.Background()
		if err := recon.Run(ctx, *cfg, pipelineRaw.ToPipeline()); err != nil {
			logging.Logger().Fatal("Pipeline failed", zap.Error(err))
		}

		logging.Logger().Info("Manual pipeline finished")
	},
}

func init() {
	rootCmd.AddCommand(manualCmd)

	manualCmd.Flags().StringVarP(&manualPipelineFile, "file", "f", "", "Path to pipeline YAML file (required)")
	manualCmd.MarkFlagRequired("file")
}
