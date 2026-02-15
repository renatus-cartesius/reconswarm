package cmd

import (
	"context"
	"fmt"
	"os"
	"reconswarm/api"
	"reconswarm/internal/logging"
	"reconswarm/internal/pipeline"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
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

// pipelineWrapper is used to parse YAML files with "pipeline:" root key
type pipelineWrapper struct {
	Pipeline pipeline.Pipeline `yaml:"pipeline"`
}

// parsePipelineYAML parses a pipeline YAML file into a domain Pipeline.
// Supports both "pipeline:" wrapper and direct format.
func parsePipelineYAML(data []byte) (pipeline.Pipeline, error) {
	// Try to parse with "pipeline:" wrapper first
	var wrapper pipelineWrapper
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return pipeline.Pipeline{}, fmt.Errorf("failed to parse pipeline YAML: %w", err)
	}

	if len(wrapper.Pipeline.Targets) > 0 || len(wrapper.Pipeline.Stages) > 0 {
		return wrapper.Pipeline, nil
	}

	// Fallback: try parsing without wrapper
	var p pipeline.Pipeline
	if err := yaml.Unmarshal(data, &p); err != nil {
		return pipeline.Pipeline{}, fmt.Errorf("failed to parse pipeline YAML (direct): %w", err)
	}
	return p, nil
}

// pipelineToProto converts a domain pipeline.Pipeline to an api.Pipeline proto message.
func pipelineToProto(p pipeline.Pipeline) *api.Pipeline {
	proto := &api.Pipeline{}

	for _, t := range p.Targets {
		pt := &api.Target{Type: t.Type}
		switch v := t.Value.(type) {
		case string:
			pt.StringValue = v
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					pt.ListValue = append(pt.ListValue, s)
				}
			}
		case []string:
			pt.ListValue = v
		}
		proto.Targets = append(proto.Targets, pt)
	}

	for _, s := range p.Stages {
		switch stage := s.(type) {
		case *pipeline.ExecStage:
			proto.Stages = append(proto.Stages, &api.Stage{
				Name: stage.Name,
				Config: &api.Stage_Exec{
					Exec: &api.ExecStage{Steps: stage.Steps},
				},
			})
		case *pipeline.SyncStage:
			proto.Stages = append(proto.Stages, &api.Stage{
				Name: stage.Name,
				Config: &api.Stage_Sync{
					Sync: &api.SyncStage{Src: stage.Src, Dest: stage.Dest},
				},
			})
		}
	}

	return proto
}

func runPipeline(serverAddr, pipelineFile string) {
	// Read pipeline file
	content, err := os.ReadFile(pipelineFile)
	if err != nil {
		logging.Logger().Fatal("Failed to read pipeline file", zap.Error(err))
	}

	// Parse YAML into domain pipeline
	p, err := parsePipelineYAML(content)
	if err != nil {
		logging.Logger().Fatal("Failed to parse pipeline YAML", zap.Error(err))
	}

	logging.Logger().Info("Pipeline parsed",
		zap.Int("targets", len(p.Targets)),
		zap.Int("stages", len(p.Stages)))

	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logging.Logger().Fatal("Did not connect", zap.Error(err))
	}
	defer conn.Close()
	c := api.NewReconSwarmClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Convert domain pipeline to proto and send
	r, err := c.RunPipeline(ctx, &api.RunPipelineRequest{Pipeline: pipelineToProto(p)})
	if err != nil {
		logging.Logger().Fatal("Could not run pipeline", zap.Error(err))
	}
	fmt.Printf("Pipeline submitted successfully. ID: %s\n", r.PipelineId)
}
