package cmd

import (
	"context"
	"fmt"
	"os"
	"reconswarm/api"
	"reconswarm/internal/logging"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	runPipelineFile string
	runServerAddr   string
)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run [pipeline file]",
	Short: "Run a reconnaissance pipeline",
	Long:  `Submit a pipeline YAML file to the ReconSwarm server for execution.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if runPipelineFile == "" {
			if len(args) > 0 {
				runPipelineFile = args[0]
			} else {
				logging.Logger().Fatal("Pipeline file is required")
			}
		}

		runPipeline(runServerAddr, runPipelineFile)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVarP(&runPipelineFile, "pipeline", "f", "", "Path to pipeline YAML file")
	runCmd.Flags().StringVarP(&runServerAddr, "server", "s", "localhost:50051", "Server address")
}

func runPipeline(serverAddr, pipelineFile string) {
	// Read pipeline file
	content, err := os.ReadFile(pipelineFile)
	if err != nil {
		logging.Logger().Fatal("Failed to read pipeline file", zap.Error(err))
	}

	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logging.Logger().Fatal("Did not connect", zap.Error(err))
	}
	defer conn.Close()
	c := api.NewReconSwarmClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	r, err := c.RunPipeline(ctx, &api.RunPipelineRequest{PipelineYaml: string(content)})
	if err != nil {
		logging.Logger().Fatal("Could not run pipeline", zap.Error(err))
	}
	fmt.Printf("Pipeline submitted successfully. ID: %s\n", r.PipelineId)
}
