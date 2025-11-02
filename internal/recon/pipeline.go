package recon

import (
	"context"
	"fmt"
	"math/rand"
	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/logging"
	"reconswarm/internal/provisioning"
	"reconswarm/internal/ssh"
	"slices"
	"strings"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PrepareTargets returns all targets from the pipeline
func PrepareTargets(cfg config.Config) []string {
	var targets []string

	logging.Logger().Info("preparing initial target list")

	for _, target := range cfg.Pipeline().Targets {
		switch target.Type {
		case "crtsh":
			targets = append(targets, target.Value.(string))
			t, err := GetCrtshClient().Dump(target.Value.(string))
			if err != nil {
				logging.Logger().Error("error on preparing crtsh targets", zap.Error(err))
				continue
			}
			targets = append(targets, t...)
			logging.Logger().Info("adding crtsh resolved targets", zap.Strings("targets", t))
		case "list":
			list, ok := target.Value.([]any)
			if !ok {
				logging.Logger().Error("invalid type for list targets", zap.Any("value", target.Value))
				continue
			}
			for _, v := range list {
				if s, ok := v.(string); ok {
					targets = append(targets, s)
				} else {
					logging.Logger().Error("non-string entry in target list", zap.Any("entry", v))
				}
			}
		default:
			logging.Logger().Error("found unknown targets type", zap.String("type", target.Type))
		}
	}

	logging.Logger().Info("prepared targets list for recon", zap.Strings("targets", targets))

	return targets
}

func Run(ctx context.Context, cfg config.Config) error {
	// Preparing final targets list
	targets := PrepareTargets(cfg)

	// Get or generate SSH key pair
	logging.Logger().Info("Getting or generating SSH key pair")
	keyDir := "/tmp/reconswarm"
	keyPair, err := ssh.GetOrGenerateKeyPair(keyDir)
	if err != nil {
		logging.Logger().Fatal("Failed to get or generate SSH key pair", zap.Error(err))
	}

	logging.Logger().Info("Using SSH key pair",
		zap.String("private_key", keyPair.PrivateKeyPath),
		zap.String("public_key", keyPair.PublicKeyPath))

	// Create provisioner
	logging.Logger().Info("Creating Yandex Cloud provisioner")
	provisioner, err := provisioning.NewYcProvisioner(cfg.IAMToken, cfg.FolderID)
	if err != nil {
		logging.Logger().Fatal("Failed to create provisioner", zap.Error(err))
	}

	workersCount := min(cfg.MaxWorkers, len(targets))

	// Shuffle targets to distribute them evenly across workers
	logging.Logger().Info("shuffling targets to ensure even distribution across workers")
	rand.Shuffle(len(targets), func(i, j int) {
		targets[i], targets[j] = targets[j], targets[i]
	})

	pool := pond.NewPool(workersCount)

	for c := range slices.Chunk(targets, (len(targets)+workersCount-1)/workersCount) {
		pool.Submit(func() {

			logging.Logger().Info("started worker", zap.Strings("targets", c), zap.Any("provisioner", provisioner))

			// Get VM parameters from environment variables or use defaults
			name := fmt.Sprintf("reconswarm-demo-%v", uuid.NewString())

			// Create worker node specification
			spec := provisioning.InstanceSpec{
				Name:         name,
				Cores:        cfg.DefaultCores,
				Memory:       cfg.DefaultMemory,
				DiskSize:     cfg.DefaultDiskSize,
				ImageID:      cfg.DefaultImage,
				Zone:         cfg.DefaultZone,
				SSHPublicKey: keyPair.PublicKey,
				Username:     cfg.DefaultUsername,
			}

			// Create worker node
			logging.Logger().Info("creating worker node",
				zap.String("name", spec.Name),
				zap.String("zone", spec.Zone),
				zap.Int("cores", int(spec.Cores)),
				zap.Int64("memory_gb", spec.Memory))

			instance, err := provisioner.Create(ctx, spec)
			if err != nil {
				logging.Logger().Error("failed to create worker node", zap.Error(err))
				return
			}

			// Create control configuration
			controlConfig := control.Config{
				Host:         instance.IP,
				User:         cfg.DefaultUsername,
				PrivateKey:   keyPair.PrivateKeyPath,
				Timeout:      5 * time.Minute,
				SSHTimeout:   30 * time.Second,
				InstanceName: instance.Name,
			}

			// Create controller
			logging.Logger().Debug("creating controller")
			controller, err := control.NewController(controlConfig)
			if err != nil {
				logging.Logger().Error("failed to create controller", zap.Error(err))
				return
			}
			defer func() {
				if err := controller.Close(); err != nil {
					logging.Logger().Warn("failed to close controller",
						zap.String("instance", instance.Name),
						zap.Error(err))
				}
			}()

			defer func() {
				// Delete VM
				logging.Logger().Info("deleting worker node after pipeline finish",
					zap.String("worker node", name))
				err = provisioner.Delete(ctx, instance.ID)
				if err != nil {
					logging.Logger().Error("failed to delete worker node", zap.Error(err))
				}
			}()

			// Setup vm
			logging.Logger().Info("starting worker node setup",
				zap.String("ip", instance.IP),
				zap.Int("command_count", len(cfg.SetupCommands)))

			if err := setupVm(controller, cfg); err != nil {
				logging.Logger().Error("failed to setup worker node", zap.Error(err))
				return
			}

			// Run recon stages
			if err := runStages(controller, cfg, c); err != nil {
				logging.Logger().Error("failed to run recon pipeline", zap.Error(err))
				return
			}

			logging.Logger().Info("worker node created successfully",
				zap.String("id", instance.ID),
				zap.String("name", instance.Name),
				zap.String("ip", instance.IP),
				zap.String("zone", instance.Zone),
				zap.String("status", instance.Status))

		})
	}

	pool.StopAndWait()

	return nil
}

func runStages(controller control.Controller, cfg config.Config, targets []string) error {
	logging.Logger().Info("starting pipeline stages execution",
		zap.Int("stages_count", len(cfg.Pipeline().Stages)),
		zap.Strings("targets", targets))

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
	targetsContent := strings.Join(targets, "\n")

	// Write targets to file using echo command
	writeCmd := fmt.Sprintf("echo '%s' | sudo tee %s > /dev/null", targetsContent, targetsFile)
	if err := controller.Run(writeCmd); err != nil {
		return fmt.Errorf("failed to write targets file: %w", err)
	}

	// Execute each stage sequentially
	for stageIndex, stage := range cfg.Pipeline().Stages {
		logging.Logger().Info("executing pipeline stage",
			zap.Int("stage_index", stageIndex+1),
			zap.String("stage_name", stage.GetName()),
			zap.String("stage_type", stage.GetType()))

		// Execute the stage using the interface
		if err := stage.Execute(controller, targets, targetsFile); err != nil {
			return fmt.Errorf("failed to execute stage '%s': %w", stage.GetName(), err)
		}

		logging.Logger().Info("stage completed successfully", zap.String("stage_name", stage.GetName()))
	}

	logging.Logger().Info("all pipeline stages completed successfully")
	return nil
}

func setupVm(controller control.Controller, cfg config.Config) error {
	// Run setup commands from configuration
	for i, cmd := range cfg.SetupCommands {
		logging.Logger().Debug("executing setup command",
			zap.Int("step", i+1),
			zap.Int("total", len(cfg.SetupCommands)),
			zap.String("command", cmd))

		if err := controller.Run(cmd); err != nil {
			return err
		}
	}

	return nil
}
