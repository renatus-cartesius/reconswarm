package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"reconswarm/internal/control"
	"reconswarm/internal/logging"

	"go.uber.org/zap"
)

// ExecuteOnWorker executes all pipeline stages on a worker with the given targets.
func ExecuteOnWorker(ctx context.Context, controller control.Controller, p Pipeline, workerTargets []string) error {
	logging.Logger().Info("starting pipeline stages execution",
		zap.Int("stages_count", len(p.Stages)),
		zap.Strings("targets", workerTargets))

	// Ensure /opt/recon directory exists
	logging.Logger().Debug("ensuring /opt/recon directory exists")
	if err := controller.Run("sudo mkdir -p /opt/recon"); err != nil {
		return fmt.Errorf("failed to create /opt/recon directory: %w", err)
	}

	// Create targets file with timestamp
	timestamp := time.Now().Unix()
	targetsFile := fmt.Sprintf("/opt/recon/targets-%d.txt", timestamp)

	logging.Logger().Info("creating targets file", zap.String("file", targetsFile))

	// Create targets content
	targetsContent := strings.Join(workerTargets, "\n")

	// Write targets to file using echo command
	writeCmd := fmt.Sprintf("echo '%s' | sudo tee %s > /dev/null", targetsContent, targetsFile)
	if err := controller.Run(writeCmd); err != nil {
		return fmt.Errorf("failed to write targets file: %w", err)
	}

	// Execute each stage sequentially
	for stageIndex, stage := range p.Stages {
		logging.Logger().Info("executing pipeline stage",
			zap.Int("stage_index", stageIndex+1),
			zap.String("stage_name", stage.GetName()),
			zap.String("stage_type", stage.GetType()))

		// Execute the stage using the interface
		if err := stage.Execute(ctx, controller, workerTargets, targetsFile); err != nil {
			return fmt.Errorf("failed to execute stage '%s': %w", stage.GetName(), err)
		}

		logging.Logger().Info("stage completed successfully", zap.String("stage_name", stage.GetName()))
	}

	logging.Logger().Info("all pipeline stages completed successfully")
	return nil
}
