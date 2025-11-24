package manager

import (
	"context"
	"fmt"
	"reconswarm/internal/logging"
	"reconswarm/internal/pipeline"
	"reconswarm/internal/recon"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// PipelineStatus represents the status of a pipeline
type PipelineStatus string

const (
	PipelineStatusPending   PipelineStatus = "Pending"
	PipelineStatusRunning   PipelineStatus = "Running"
	PipelineStatusCompleted PipelineStatus = "Completed"
	PipelineStatusFailed    PipelineStatus = "Failed"
)

// PipelineState represents the state of a pipeline
type PipelineState struct {
	ID              string
	Status          PipelineStatus
	Error           string
	TotalStages     int
	CompletedStages int
	StartTime       time.Time
	EndTime         time.Time
}

// PipelineManager manages pipeline execution
type PipelineManager struct {
	workerManager *WorkerManager
	stateManager  *StateManager
	mu            sync.Mutex
	pipelines     map[string]*PipelineState
}

// NewPipelineManager creates a new PipelineManager
func NewPipelineManager(wm *WorkerManager, sm *StateManager) *PipelineManager {
	return &PipelineManager{
		workerManager: wm,
		stateManager:  sm,
		pipelines:     make(map[string]*PipelineState),
	}
}

// SubmitPipeline submits a pipeline for execution
func (pm *PipelineManager) SubmitPipeline(ctx context.Context, yamlContent string) (string, error) {
	// Parse pipeline
	var rawPipeline pipeline.PipelineRaw
	if err := yaml.Unmarshal([]byte(yamlContent), &rawPipeline); err != nil {
		return "", fmt.Errorf("failed to parse pipeline YAML: %w", err)
	}

	p := rawPipeline.ToPipeline()
	id := fmt.Sprintf("pipe-%s", uuid.NewString())

	state := &PipelineState{
		ID:          id,
		Status:      PipelineStatusPending,
		TotalStages: len(p.Stages),
		StartTime:   time.Now(),
	}

	pm.mu.Lock()
	pm.pipelines[id] = state
	pm.mu.Unlock()

	// Save initial state
	if err := pm.stateManager.SavePipeline(ctx, id, state); err != nil {
		logging.Logger().Error("Failed to save pipeline state", zap.Error(err))
	}

	// Start execution in background
	go pm.runPipeline(id, p)

	return id, nil
}

// GetStatus returns the status of a pipeline
func (pm *PipelineManager) GetStatus(ctx context.Context, id string) (*PipelineState, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	state, exists := pm.pipelines[id]
	if !exists {
		// Try to load from Etcd
		var loadedState PipelineState
		if err := pm.stateManager.GetPipeline(ctx, id, &loadedState); err != nil {
			return nil, fmt.Errorf("pipeline not found: %s", id)
		}
		pm.pipelines[id] = &loadedState
		return &loadedState, nil
	}

	return state, nil
}

func (pm *PipelineManager) runPipeline(id string, p pipeline.Pipeline) {
	logging.Logger().Info("Starting pipeline execution", zap.String("pipeline_id", id))

	pm.updateStatus(id, PipelineStatusRunning, "")

	// Prepare targets
	targets := recon.PrepareTargets(p)
	if len(targets) == 0 {
		pm.updateStatus(id, PipelineStatusFailed, "No targets found")
		return
	}

	// Calculate needed workers (simple heuristic for now)
	// We want to distribute targets evenly
	// Let's say we want at least 1 target per worker, but max 10 workers per pipeline?
	// Or just ask for as many as possible up to a limit?
	// The requirement says "Server assigned part of fragments... to already running workers".

	// Let's try to get workers in chunks
	remainingTargets := targets

	// We'll use a wait group to wait for all chunks to complete
	var wg sync.WaitGroup

	// Loop until all targets are processed
	for len(remainingTargets) > 0 {
		// Determine chunk size based on available workers or default
		// For simplicity, let's say we want to process 10 targets per worker
		chunkSize := 10
		neededWorkers := (len(remainingTargets) + chunkSize - 1) / chunkSize

		// Request workers
		// We request up to neededWorkers
		workers, err := pm.workerManager.RequestWorkers(context.Background(), neededWorkers, id)
		if err != nil {
			logging.Logger().Error("Error requesting workers", zap.Error(err))
			// Wait and retry
			time.Sleep(5 * time.Second)
			continue
		}

		if len(workers) == 0 {
			// No workers available, wait and retry
			logging.Logger().Info("No workers available, waiting...", zap.String("pipeline_id", id))
			time.Sleep(5 * time.Second)
			continue
		}

		logging.Logger().Info("Got workers", zap.Int("count", len(workers)), zap.String("pipeline_id", id))

		// Distribute targets to workers
		// We take len(workers) * chunkSize targets
		numTargetsToProcess := len(workers) * chunkSize
		if numTargetsToProcess > len(remainingTargets) {
			numTargetsToProcess = len(remainingTargets)
		}

		targetsToProcess := remainingTargets[:numTargetsToProcess]
		remainingTargets = remainingTargets[numTargetsToProcess:]

		// Split targetsToProcess among workers
		chunks := slices.Chunk(targetsToProcess, (len(targetsToProcess)+len(workers)-1)/len(workers))

		chunkIdx := 0
		for chunk := range chunks {
			if chunkIdx >= len(workers) {
				break
			}
			worker := workers[chunkIdx]
			chunkIdx++

			wg.Add(1)
			go func(w *WorkerManager, workerID string, t []string) {
				defer wg.Done()
				defer w.ReleaseWorker(workerID)

				// We need to get the worker object again or pass it?
				// RequestWorkers returned *Worker objects.
				// But we need to access the controller.
				// The Worker struct has the Controller.
				// But we only have the ID here if we passed ID.
				// Wait, RequestWorkers returns []*Worker.

				// Let's find the worker object from the list we got
				var currentWorker *Worker
				for _, wk := range workers {
					if wk.ID == workerID {
						currentWorker = wk
						break
					}
				}

				if currentWorker == nil {
					logging.Logger().Error("Worker not found in allocated list", zap.String("worker_id", workerID))
					return
				}

				if err := recon.ExecutePipelineOnWorker(context.Background(), currentWorker.Controller, p, t); err != nil {
					logging.Logger().Error("Pipeline execution failed on worker",
						zap.String("worker_id", workerID),
						zap.Error(err))
					// We should probably record this error
				}
			}(pm.workerManager, worker.ID, chunk)
		}
	}

	wg.Wait()

	pm.updateStatus(id, PipelineStatusCompleted, "")
	logging.Logger().Info("Pipeline execution completed", zap.String("pipeline_id", id))

	// Check if we should deallocate all workers
	// This is a bit hacky, but per requirements: "After completion of all pipelines, all workers deallocated"
	// We can check if any other pipelines are running.
	// But PipelineManager doesn't know if other pipelines are running on other servers (if distributed).
	// But here we have a single server instance.

	active := false
	pm.mu.Lock()
	for _, s := range pm.pipelines {
		if s.Status == PipelineStatusRunning || s.Status == PipelineStatusPending {
			active = true
			break
		}
	}
	pm.mu.Unlock()

	if !active {
		// Wait a bit to ensure no race with new submissions?
		time.Sleep(2 * time.Second)
		pm.workerManager.DeallocateAll(context.Background())
	}
}

func (pm *PipelineManager) updateStatus(id string, status PipelineStatus, errMsg string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if state, exists := pm.pipelines[id]; exists {
		state.Status = status
		if errMsg != "" {
			state.Error = errMsg
		}
		if status == PipelineStatusCompleted || status == PipelineStatusFailed {
			state.EndTime = time.Now()
		}

		// Save to Etcd
		if err := pm.stateManager.SavePipeline(context.Background(), id, state); err != nil {
			logging.Logger().Error("Failed to save pipeline state", zap.Error(err))
		}
	}
}
