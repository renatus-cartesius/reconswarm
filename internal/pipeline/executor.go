package pipeline

import (
	"context"
	"fmt"
	"os"
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
		zap.Int("targets_count", len(workerTargets)),
		zap.Strings("targets_sample", logging.TruncateSlice(workerTargets, 10)))

	// Ensure /opt/recon directory exists and is writable
	logging.Logger().Debug("ensuring /opt/recon directory exists")
	if err := controller.Run("sudo mkdir -p /opt/recon && sudo chmod 777 /opt/recon"); err != nil {
		return fmt.Errorf("failed to create /opt/recon directory: %w", err)
	}

	// Create targets file with timestamp
	timestamp := time.Now().Unix()
	targetsFile := fmt.Sprintf("/opt/recon/targets-%d.txt", timestamp)

	logging.Logger().Info("creating targets file", zap.String("file", targetsFile))

	// Write targets to file using SFTP
	if err := writeTargetsFile(controller, targetsFile, workerTargets); err != nil {
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

// writeTargetsFile writes targets to a file on the remote host using SFTP
func writeTargetsFile(controller control.Controller, path string, targets []string) error {
	f, err := controller.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
	if err != nil {
		return err
	}
	defer f.Close()

	content := strings.Join(targets, "\n") + "\n"
	_, err = f.Write([]byte(content))
	return err
}
