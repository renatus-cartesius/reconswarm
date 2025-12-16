package cmd

import (
	"reconswarm/internal/config"
	"reconswarm/internal/logging"
	"reconswarm/internal/server"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var serverPort int

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the ReconSwarm server",
	Long:  `Start the ReconSwarm gRPC server to manage pipelines and workers.`,
	Run: func(cmd *cobra.Command, args []string) {
		logging.Logger().Info("Starting ReconSwarm server")

		// Load configuration
		cfg, err := config.Load()
		if err != nil {
			logging.Logger().Fatal("Failed to load configuration", zap.Error(err))
		}

		// Create server
		srv, err := server.NewServer(*cfg)
		if err != nil {
			logging.Logger().Fatal("Failed to create server", zap.Error(err))
		}

		// Start server
		if err := srv.Start(serverPort); err != nil {
			logging.Logger().Fatal("Server failed", zap.Error(err))
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().IntVarP(&serverPort, "port", "p", 50051, "Port to listen on")
}
