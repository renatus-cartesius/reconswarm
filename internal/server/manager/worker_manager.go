package manager

import (
	"context"
	"fmt"
	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/logging"
	"reconswarm/internal/provisioning"
	"reconswarm/internal/ssh"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// WorkerStatus represents the status of a worker
type WorkerStatus string

const (
	WorkerStatusIdle WorkerStatus = "Idle"
	WorkerStatusBusy WorkerStatus = "Busy"
)

// Worker represents a worker node
type Worker struct {
	ID          string
	Name        string
	IP          string
	Status      WorkerStatus
	CurrentTask string
	InstanceID  string
	Controller  control.Controller       `json:"-"`
	Provisioner provisioning.Provisioner `json:"-"`
}

// WorkerManager manages the worker pool
type WorkerManager struct {
	mu          sync.Mutex
	workers     map[string]*Worker
	maxWorkers  int
	provisioner provisioning.Provisioner
	sshKeyPair  *ssh.KeyPair
	vmDefaults  provisioning.VMDefaults
	setupCmds   []string
	ctrlFactory ControllerFactory
}

// ControllerFactory creates a new controller
type ControllerFactory func(config control.Config) (control.Controller, error)

// NewWorkerManager creates a new WorkerManager
func NewWorkerManager(cfg config.Config, sm StateManager, prov provisioning.Provisioner, cf ControllerFactory, keyProvider ssh.KeyProvider) (*WorkerManager, error) {
	ctx := context.Background()

	// Get or create SSH keys using the key provider
	keyPair, err := keyProvider.GetOrCreate(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get SSH keys: %w", err)
	}

	// Extract VM defaults from provisioner config
	vmDefaults := provisioning.GetVMDefaults(cfg.Provisioner)

	return &WorkerManager{
		workers:     make(map[string]*Worker),
		maxWorkers:  cfg.Workers.MaxWorkers,
		provisioner: prov,
		sshKeyPair:  keyPair,
		vmDefaults:  vmDefaults,
		setupCmds:   cfg.Workers.SetupCommands,
		ctrlFactory: cf,
	}, nil
}

// GetStatus returns the status of all workers
func (wm *WorkerManager) GetStatus() []*Worker {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	var status []*Worker
	for _, w := range wm.workers {
		// Return a copy to avoid race conditions
		wCopy := *w
		status = append(status, &wCopy)
	}
	return status
}

// CreateEphemeralWorker creates a single worker that is not tracked in the pool.
// The caller is responsible for calling DeleteEphemeralWorker when done.
func (wm *WorkerManager) CreateEphemeralWorker(ctx context.Context) (*Worker, error) {
	name := fmt.Sprintf("reconswarm-%v", uuid.NewString())
	spec := provisioning.InstanceSpec{
		Name:         name,
		Cores:        wm.vmDefaults.Cores,
		Memory:       wm.vmDefaults.Memory,
		DiskSize:     wm.vmDefaults.DiskSize,
		ImageID:      wm.vmDefaults.Image,
		Zone:         wm.vmDefaults.Zone,
		SSHPublicKey: wm.sshKeyPair.PublicKey,
		Username:     wm.vmDefaults.Username,
	}

	logging.Logger().Info("creating ephemeral worker node",
		zap.String("name", spec.Name),
		zap.String("zone", spec.Zone),
		zap.Int("cores", int(spec.Cores)),
		zap.Int64("memory_gb", spec.Memory))

	instance, err := wm.provisioner.Create(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	// Create controller
	controlConfig := control.Config{
		Host:         instance.IP,
		User:         wm.vmDefaults.Username,
		PrivateKey:   wm.sshKeyPair.PrivateKey,
		Timeout:      5 * time.Minute,
		SSHTimeout:   30 * time.Second,
		InstanceName: instance.Name,
	}

	controller, err := wm.ctrlFactory(controlConfig)
	if err != nil {
		// Cleanup failed instance
		logging.Logger().Error("failed to get controller from factory", zap.Error(err))
		if delErr := wm.provisioner.Delete(ctx, instance.ID); delErr != nil {
			logging.Logger().Error("failed to delete instance during cleanup", zap.String("instance_id", instance.ID), zap.Error(delErr))
		}
		return nil, fmt.Errorf("failed to create controller: %w", err)
	}

	// Setup VM
	if err := wm.setupWorker(controller); err != nil {
		controller.Close()
		logging.Logger().Error("failed to setup worker", zap.Error(err))
		if delErr := wm.provisioner.Delete(ctx, instance.ID); delErr != nil {
			logging.Logger().Error("failed to delete instance during cleanup", zap.String("instance_id", instance.ID), zap.Error(delErr))
		}
		return nil, fmt.Errorf("failed to setup worker: %w", err)
	}

	return &Worker{
		ID:          uuid.NewString(),
		Name:        instance.Name,
		IP:          instance.IP,
		Status:      WorkerStatusBusy,
		InstanceID:  instance.ID,
		Controller:  controller,
		Provisioner: wm.provisioner,
	}, nil
}

// DeleteEphemeralWorker deletes a worker created by CreateEphemeralWorker.
func (wm *WorkerManager) DeleteEphemeralWorker(ctx context.Context, w *Worker) error {
	logging.Logger().Info("deleting ephemeral worker node", zap.String("name", w.Name))

	if w.Controller != nil {
		if err := w.Controller.Close(); err != nil {
			logging.Logger().Warn("failed to close controller", zap.String("name", w.Name), zap.Error(err))
		}
	}

	if err := wm.provisioner.Delete(ctx, w.InstanceID); err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}

	return nil
}

// MaxWorkers returns the maximum number of workers allowed
func (wm *WorkerManager) MaxWorkers() int {
	return wm.maxWorkers
}

func (wm *WorkerManager) setupWorker(controller control.Controller) error {
	for _, cmd := range wm.setupCmds {
		if err := controller.Run(cmd); err != nil {
			return fmt.Errorf("failed to run setup command '%s': %w", cmd, err)
		}
	}
	return nil
}
