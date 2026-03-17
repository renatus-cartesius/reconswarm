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
	return &api.Pipeline{
		Targets:      convertTargetsToProto(p.Targets),
		Stages:       convertStagesToProto(p.Stages),
		PreCommands:  p.PreCommands,
		PostCommands: p.PostCommands,
	}
}

// convertTargetsToProto converts domain targets to proto targets.
func convertTargetsToProto(targets []pipeline.Target) []*api.Target {
	return mapSlice(targets, convertTargetToProto)
}

// convertTargetToProto is a helper function that converts a single target.
func convertTargetToProto(target pipeline.Target) *api.Target {
	pt := &api.Target{Type: target.Type}

	// Handle target value directly based on its type
	if target.Value != nil {
		switch v := target.Value.(type) {
		case string:
			pt.StringValue = v
		case []string:
			pt.ListValue = v
		case []interface{}:
			pt.ListValue = make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					pt.ListValue = append(pt.ListValue, s)
				}
			}
		default:
			logging.Logger().Warn("Unsupported target value type", zap.String("type", fmt.Sprintf("%T", v)))
		}
	}

	return pt
}

// convertStagesToProto converts domain stages to proto stages.
func convertStagesToProto(stages []pipeline.Stage) []*api.Stage {
	return mapSlice(stages, convertStageToProto)
}

// convertStageToProto is a helper function that converts a single stage.
func convertStageToProto(s pipeline.Stage) *api.Stage {
	switch stage := s.(type) {
	case *pipeline.ExecStage:
		return &api.Stage{
			Name: stage.Name,
			Config: &api.Stage_Exec{
				Exec: &api.ExecStage{Steps: stage.Steps},
			},
		}
	case *pipeline.SyncStage:
		return &api.Stage{
			Name: stage.Name,
			Config: &api.Stage_Sync{
				Sync: &api.SyncStage{Src: stage.Src, Dest: stage.Dest},
			},
		}
	}
	return nil
}

// mapSlice is a generic helper function for transforming slices.
func mapSlice[T any, R any](slice []T, fn func(T) R) []R {
	result := make([]R, 0, len(slice))
	for _, item := range slice {
		mapped := fn(item)
		result = append(result, mapped)
	}
	return result
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
