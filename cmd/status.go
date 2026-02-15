package cmd

import (
	"context"
	"fmt"
	"reconswarm/api"
	"reconswarm/internal/logging"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	statusPipelineID string
	statusServerAddr string
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check pipeline status",
	Long:  `Retrieve the status of a running or completed pipeline from the ReconSwarm server.`,
	Run: func(cmd *cobra.Command, args []string) {
		if statusPipelineID == "" {
			logging.Logger().Fatal("Pipeline ID is required")
		}

		getStatus(statusServerAddr, statusPipelineID)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)

	statusCmd.Flags().StringVar(&statusPipelineID, "id", "", "Pipeline ID (required)")
	statusCmd.Flags().StringVarP(&statusServerAddr, "server", "s", "localhost:50051", "Server address")
	if err := statusCmd.MarkFlagRequired("id"); err != nil {
		panic(fmt.Sprintf("failed to mark flag as required: %v", err))
	}
}

func getStatus(serverAddr, pipelineID string) {
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logging.Logger().Fatal("Did not connect", zap.Error(err))
	}
	defer conn.Close()
	c := api.NewReconSwarmClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	r, err := c.GetPipeline(ctx, &api.GetPipelineRequest{PipelineId: pipelineID})
	if err != nil {
		logging.Logger().Fatal("Could not get status", zap.Error(err))
	}

	fmt.Printf("Pipeline ID: %s\n", r.PipelineId)
	fmt.Printf("Status: %s\n", r.Status)
	if r.Error != "" {
		fmt.Printf("Error: %s\n", r.Error)
	}
	fmt.Printf("Progress: %d/%d stages completed\n", r.CompletedStages, r.TotalStages)

	if p := r.Pipeline; p != nil {
		fmt.Printf("Targets: %d\n", len(p.Targets))
		fmt.Printf("Stages: %d\n", len(p.Stages))
		for i, s := range p.Stages {
			fmt.Printf("  %d. %s\n", i+1, s.Name)
		}
	}

	if len(r.Workers) > 0 {
		fmt.Println("\nWorkers:")
		for _, w := range r.Workers {
			fmt.Printf("- [%s] %s (%s): %s\n", w.WorkerId, w.Name, w.Status, w.CurrentTask)
		}
	}
}
