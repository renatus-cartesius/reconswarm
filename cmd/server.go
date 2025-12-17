package cmd

import (
	"reconswarm/internal/config"
	"reconswarm/internal/logging"
	"reconswarm/internal/server"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the ReconSwarm server",
	Long:  `Start the ReconSwarm gRPC server to manage pipelines and workers. All settings are read from the config file.`,
	Run: func(cmd *cobra.Command, args []string) {
		logging.Logger().Info("Starting ReconSwarm server")

		// Load configuration (all settings from config file)
		cfg, err := config.Load()
		if err != nil {
			logging.Logger().Fatal("Failed to load configuration", zap.Error(err))
		}

		logging.Logger().Info("Configuration loaded",
			zap.Int("port", cfg.Server.Port),
			zap.Strings("etcd_endpoints", cfg.Etcd.Endpoints),
			zap.String("provisioner_type", string(cfg.Provisioner.Type)),
			zap.Int("max_workers", cfg.Workers.MaxWorkers),
		)

		// Create and start server
		srv, err := server.NewServer(*cfg)
		if err != nil {
			logging.Logger().Fatal("Failed to create server", zap.Error(err))
		}

		// Start server on configured port
		if err := srv.Start(); err != nil {
			logging.Logger().Fatal("Server failed", zap.Error(err))
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
