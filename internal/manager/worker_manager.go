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
	WorkerStatusProvisioning WorkerStatus = "Provisioning"
	WorkerStatusIdle         WorkerStatus = "Idle"
	WorkerStatusBusy         WorkerStatus = "Busy"
	WorkerStatusTerminating  WorkerStatus = "Terminating"
)

// Worker represents a worker node
type Worker struct {
	ID          string
	Name        string
	IP          string
	Status      WorkerStatus
	CurrentTask string // PipelineID or StageName
	InstanceID  string // Cloud Provider Instance ID
	Controller  control.Controller
	Provisioner provisioning.Provisioner
	LastUsed    time.Time
}

// WorkerManager manages the worker pool
type WorkerManager struct {
	mu           sync.Mutex
	workers      map[string]*Worker
	maxWorkers   int
	provisioner  provisioning.Provisioner
	stateManager StateManager
	keyProvider  ssh.KeyProvider
	sshKeyPair   *ssh.KeyPair
	vmDefaults   provisioning.VMDefaults
	setupCmds    []string
	ctrlFactory  ControllerFactory
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
		workers:      make(map[string]*Worker),
		maxWorkers:   cfg.Workers.MaxWorkers,
		provisioner:  prov,
		stateManager: sm,
		keyProvider:  keyProvider,
		sshKeyPair:   keyPair,
		vmDefaults:   vmDefaults,
		setupCmds:    cfg.Workers.SetupCommands,
		ctrlFactory:  cf,
	}, nil
}

// RequestWorkers requests a number of workers for a task
func (wm *WorkerManager) RequestWorkers(ctx context.Context, count int, taskID string) ([]*Worker, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	var allocated []*Worker

	// 1. Reuse idle workers THAT BELONG TO THIS TASK
	for _, w := range wm.workers {
		if w.Status == WorkerStatusIdle && w.CurrentTask == taskID {
			w.Status = WorkerStatusBusy
			w.LastUsed = time.Now()
			allocated = append(allocated, w)
			if len(allocated) == count {
				return allocated, nil
			}
		}
	}

	// 2. Create new workers if limit not reached
	needed := count - len(allocated)
	availableSlots := wm.maxWorkers - len(wm.workers)
	toCreate := min(needed, availableSlots)

	if toCreate > 0 {
		logging.Logger().Info("Creating new workers", zap.Int("count", toCreate), zap.String("task_id", taskID))
		newWorkers, err := wm.createWorkers(ctx, toCreate, taskID)
		if err != nil {
			// If we failed to create some, return what we have (including reused ones)
			logging.Logger().Error("Failed to create some workers", zap.Error(err))
		}
		allocated = append(allocated, newWorkers...)
	}

	return allocated, nil
}

// ReleaseWorker releases a worker back to the pool
func (wm *WorkerManager) ReleaseWorker(workerID string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if w, exists := wm.workers[workerID]; exists {
		w.Status = WorkerStatusIdle
		w.LastUsed = time.Now()
		logging.Logger().Info("Worker released", zap.String("worker_id", workerID))

		// Update state in Etcd
		if err := wm.stateManager.SaveWorker(context.Background(), w.ID, w); err != nil {
			logging.Logger().Error("Failed to save worker state", zap.Error(err))
		}
	}
}

// DeallocateWorkers deallocates all workers for a specific task
func (wm *WorkerManager) DeallocateWorkers(ctx context.Context, taskID string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	logging.Logger().Info("Deallocating workers for task", zap.String("task_id", taskID))

	for id, w := range wm.workers {
		if w.CurrentTask == taskID {
			if err := w.Provisioner.Delete(ctx, w.InstanceID); err != nil {
				logging.Logger().Error("Failed to delete worker instance", zap.String("instance_id", w.InstanceID), zap.Error(err))
			}
			if w.Controller != nil {
				w.Controller.Close()
			}
			if err := wm.stateManager.DeleteWorker(ctx, id); err != nil {
				logging.Logger().Error("Failed to delete worker state", zap.Error(err))
			}
			delete(wm.workers, id)
		}
	}
	return nil
}

// DeallocateAll deallocates all workers
func (wm *WorkerManager) DeallocateAll(ctx context.Context) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	logging.Logger().Info("Deallocating all workers")

	for id, w := range wm.workers {
		if err := w.Provisioner.Delete(ctx, w.InstanceID); err != nil {
			logging.Logger().Error("Failed to delete worker instance", zap.String("instance_id", w.InstanceID), zap.Error(err))
		}
		if w.Controller != nil {
			w.Controller.Close()
		}
		if err := wm.stateManager.DeleteWorker(ctx, id); err != nil {
			logging.Logger().Error("Failed to delete worker state", zap.Error(err))
		}
		delete(wm.workers, id)
	}
	return nil
}

// createWorkers creates new workers
func (wm *WorkerManager) createWorkers(ctx context.Context, count int, taskID string) ([]*Worker, error) {
	var newWorkers []*Worker
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

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

			instance, err := wm.provisioner.Create(ctx, spec)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}

			// Create controller - use private key from memory (etcd)
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
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				// Try to cleanup failed instance
				if delErr := wm.provisioner.Delete(ctx, instance.ID); delErr != nil {
					logging.Logger().Error("Failed to delete instance during cleanup", zap.String("instance_id", instance.ID), zap.Error(delErr))
				}
				return
			}

			// Setup VM
			if err := wm.setupWorker(controller); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				controller.Close()
				if delErr := wm.provisioner.Delete(ctx, instance.ID); delErr != nil {
					logging.Logger().Error("Failed to delete instance during cleanup", zap.String("instance_id", instance.ID), zap.Error(delErr))
				}
				return
			}

			w := &Worker{
				ID:          uuid.NewString(),
				Name:        instance.Name,
				IP:          instance.IP,
				Status:      WorkerStatusBusy,
				CurrentTask: taskID,
				InstanceID:  instance.ID,
				Controller:  controller,
				Provisioner: wm.provisioner,
				LastUsed:    time.Now(),
			}

			mu.Lock()
			wm.workers[w.ID] = w
			newWorkers = append(newWorkers, w)
			mu.Unlock()

			// Save to Etcd
			if err := wm.stateManager.SaveWorker(ctx, w.ID, w); err != nil {
				logging.Logger().Error("Failed to save worker state", zap.Error(err))
			}

		}()
	}

	wg.Wait()

	if len(errs) > 0 {
		return newWorkers, fmt.Errorf("encountered %d errors during worker creation", len(errs))
	}

	return newWorkers, nil
}

func (wm *WorkerManager) setupWorker(controller control.Controller) error {
	for _, cmd := range wm.setupCmds {
		if err := controller.Run(cmd); err != nil {
			return fmt.Errorf("failed to run setup command '%s': %w", cmd, err)
		}
	}
	return nil
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
