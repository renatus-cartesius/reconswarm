package recon

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/logging"
	"reconswarm/internal/provisioning"
	"reconswarm/internal/ssh"
	"reconswarm/internal/state"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const StateFileName = ".reconswarm.json"

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
	// Load or initialize state
	s, err := state.Load(StateFileName)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Logger().Info("No existing state found, starting fresh")
			s = state.New()
		} else {
			return fmt.Errorf("failed to load state: %w", err)
		}
	} else {
		logging.Logger().Info("Resuming from existing state")
	}

	// If no workers in state, initialize them
	if len(s.Workers) == 0 {
		// Preparing final targets list
		targets := PrepareTargets(cfg)

		workersCount := min(cfg.MaxWorkers, len(targets))

		// Shuffle targets to distribute them evenly across workers
		rand.Shuffle(len(targets), func(i, j int) {
			targets[i], targets[j] = targets[j], targets[i]
		})

		// Distribute targets and create worker entries in state
		chunks := slices.Chunk(targets, (len(targets)+workersCount-1)/workersCount)
		for c := range chunks {
			workerID := uuid.NewString()
			s.AddWorker(state.WorkerState{
				ID:      workerID,
				Targets: c,
				Status:  "pending",
			})
		}

		if err := s.Save(StateFileName); err != nil {
			return fmt.Errorf("failed to save initial state: %w", err)
		}
	}

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

	pool := pond.NewPool(cfg.MaxWorkers)
	var wg sync.WaitGroup

	// Iterate over workers from state
	for id, workerState := range s.Workers {
		if workerState.Status == "completed" {
			logging.Logger().Info("Skipping completed worker", zap.String("worker_id", id))
			continue
		}

		wg.Add(1)
		workerID := id // Capture for closure
		pool.Submit(func() {
			defer wg.Done()
			processWorker(ctx, cfg, s, workerID, keyPair, provisioner)
		})
	}

	pool.StopAndWait()
	wg.Wait()

	return nil
}

func processWorker(ctx context.Context, cfg config.Config, s *state.State, workerID string, keyPair *ssh.KeyPair, provisioner provisioning.Provisioner) {
	worker, _ := s.GetWorker(workerID)
	logging.Logger().Info("processing worker", zap.String("worker_id", workerID), zap.Strings("targets", worker.Targets))

	// Update status to running
	s.UpdateWorker(workerID, func(w *state.WorkerState) {
		w.Status = "running"
	})
	s.Save(StateFileName)

	// Check if we have an existing instance
	var instance *provisioning.InstanceInfo
	var err error

	if worker.InstanceID != "" {
		logging.Logger().Info("Checking existing instance", zap.String("instance_id", worker.InstanceID))
		// TODO: Add Get method to provisioner to check status. For now, assume it might exist or we need to recreate if not found.
		// Since the current provisioner interface doesn't have Get, we might need to rely on Create being idempotent or handle errors.
		// Ideally, we should check if it exists. If the provisioner doesn't support Get, we might have to just try to create a new one if we can't connect.
		// For this implementation, let's assume we need to create if we don't have a valid IP or if connection fails.
		// But to be safe and simple given current interfaces:
		// If we have an InstanceID, we assume it's running. If we can't connect, we might fail this run and let user decide.
		// OR, we can try to create a new one if the old one is gone.
		// Let's try to reuse the IP if we have it.
	}

	// If no instance ID or we decide to recreate:
	if worker.InstanceID == "" {
		name := fmt.Sprintf("reconswarm-%s", workerID)
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

		logging.Logger().Info("creating worker node", zap.String("name", spec.Name))
		instance, err = provisioner.Create(ctx, spec)
		if err != nil {
			logging.Logger().Error("failed to create worker node", zap.Error(err))
			s.UpdateWorker(workerID, func(w *state.WorkerState) {
				w.Error = err.Error()
				w.Status = "failed"
			})
			s.Save(StateFileName)
			return
		}

		// Update state with new instance info
		s.UpdateWorker(workerID, func(w *state.WorkerState) {
			w.InstanceID = instance.ID
			w.InstanceIP = instance.IP
			w.InstanceName = instance.Name
		})
		s.Save(StateFileName)
	} else {
		// Reconstruct instance struct from state
		instance = &provisioning.InstanceInfo{
			ID:   worker.InstanceID,
			IP:   worker.InstanceIP,
			Name: worker.InstanceName,
		}
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

	controller, err := control.NewController(controlConfig)
	if err != nil {
		logging.Logger().Error("failed to create controller", zap.Error(err))
		return
	}
	defer func() {
		if err := controller.Close(); err != nil {
			logging.Logger().Warn("failed to close controller", zap.Error(err))
		}
	}()

	// Cleanup logic
	defer func() {
		// Only delete if context was NOT canceled (graceful shutdown)
		// OR if we want to enforce cleanup on interrupt?
		// Requirement: "graceful shutdown... keeping VMs alive for resumption"
		// So if ctx.Err() != nil, we do NOT delete.
		if ctx.Err() != nil {
			logging.Logger().Info("Context canceled, keeping worker node for resumption", zap.String("worker_node", instance.Name))
			return
		}

		// Also check if worker actually completed successfully
		w, _ := s.GetWorker(workerID)
		if w.Status == "completed" {
			logging.Logger().Info("deleting worker node after pipeline finish", zap.String("worker_node", instance.Name))
			err = provisioner.Delete(context.Background(), instance.ID) // Use background context for cleanup
			if err != nil {
				logging.Logger().Error("failed to delete worker node", zap.Error(err))
			}

			// Clear instance info from state so we don't try to reuse deleted VM
			s.UpdateWorker(workerID, func(w *state.WorkerState) {
				w.InstanceID = ""
				w.InstanceIP = ""
				w.InstanceName = ""
			})
			s.Save(StateFileName)
		}
	}()

	// Setup VM (idempotent-ish, or we can track setup status too)
	// For now, let's run setup every time to be safe, or we could add a "SetupCompleted" flag to state.
	// Let's assume setup commands are safe to re-run (e.g. apt update).
	if err := setupVm(controller, cfg); err != nil {
		logging.Logger().Error("failed to setup worker node", zap.Error(err))
		return
	}

	// Run recon stages
	if err := runStages(ctx, controller, cfg, s, workerID, worker.Targets); err != nil {
		logging.Logger().Error("failed to run recon pipeline", zap.Error(err))
		s.UpdateWorker(workerID, func(w *state.WorkerState) {
			w.Error = err.Error()
			w.Status = "failed"
		})
		s.Save(StateFileName)
		return
	}

	// Mark as completed
	s.UpdateWorker(workerID, func(w *state.WorkerState) {
		w.Status = "completed"
		w.Error = ""
	})
	s.Save(StateFileName)

	logging.Logger().Info("worker completed successfully", zap.String("worker_id", workerID))
}

func runStages(ctx context.Context, controller control.Controller, cfg config.Config, s *state.State, workerID string, targets []string) error {
	logging.Logger().Info("starting pipeline stages execution",
		zap.Int("stages_count", len(cfg.Pipeline().Stages)),
		zap.Strings("targets", targets))

	// Ensure /opt/recon directory exists
	if err := controller.Run("sudo mkdir -p /opt/recon"); err != nil {
		return fmt.Errorf("failed to create /opt/recon directory: %w", err)
	}

	// Create targets file
	timestamp := time.Now().Unix()
	targetsFile := fmt.Sprintf("/opt/recon/targets-%d.txt", timestamp)
	targetsContent := strings.Join(targets, "\n")
	writeCmd := fmt.Sprintf("echo '%s' | sudo tee %s > /dev/null", targetsContent, targetsFile)
	if err := controller.Run(writeCmd); err != nil {
		return fmt.Errorf("failed to write targets file: %w", err)
	}

	// Execute each stage
	for stageIndex, stage := range cfg.Pipeline().Stages {
		// Check if stage is already completed
		worker, _ := s.GetWorker(workerID)
		if slices.Contains(worker.CompletedStages, stage.GetName()) {
			logging.Logger().Info("Skipping completed stage", zap.String("stage_name", stage.GetName()))
			continue
		}

		// Check context before starting stage
		if ctx.Err() != nil {
			return ctx.Err()
		}

		logging.Logger().Info("executing pipeline stage",
			zap.Int("stage_index", stageIndex+1),
			zap.String("stage_name", stage.GetName()),
			zap.String("stage_type", stage.GetType()))

		if err := stage.Execute(controller, targets, targetsFile); err != nil {
			return fmt.Errorf("failed to execute stage '%s': %w", stage.GetName(), err)
		}

		// Mark stage as completed
		s.UpdateWorker(workerID, func(w *state.WorkerState) {
			w.CompletedStages = append(w.CompletedStages, stage.GetName())
		})
		s.Save(StateFileName)

		logging.Logger().Info("stage completed successfully", zap.String("stage_name", stage.GetName()))
	}

	return nil
}

func setupVm(controller control.Controller, cfg config.Config) error {
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
