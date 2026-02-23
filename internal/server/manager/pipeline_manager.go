package manager

import (
	"context"
	"fmt"
	"math/rand"
	"reconswarm/internal/logging"
	"reconswarm/internal/pipeline"
	"slices"
	"sync"
	"time"

	"github.com/alitto/pond/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
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
	Pipeline        *pipeline.Pipeline
	Workers         []*Worker
}

// PipelineManager manages pipeline execution
type PipelineManager struct {
	workerManager *WorkerManager
	stateManager  StateManager
	mu            sync.Mutex
	pipelines     map[string]*PipelineState
}

// NewPipelineManager creates a new PipelineManager
func NewPipelineManager(wm *WorkerManager, sm StateManager) *PipelineManager {
	return &PipelineManager{
		workerManager: wm,
		stateManager:  sm,
		pipelines:     make(map[string]*PipelineState),
	}
}

// SubmitPipeline submits a pipeline for execution.
// The caller is responsible for parsing the pipeline definition (e.g. from YAML or proto).
func (pm *PipelineManager) SubmitPipeline(ctx context.Context, p pipeline.Pipeline) (string, error) {
	if len(p.Targets) == 0 {
		return "", fmt.Errorf("pipeline must have at least one target")
	}
	if len(p.Stages) == 0 {
		return "", fmt.Errorf("pipeline must have at least one stage")
	}

	logging.Logger().Debug("Submitting pipeline",
		zap.Int("targets", len(p.Targets)),
		zap.Int("stages", len(p.Stages)))

	id := fmt.Sprintf("pipe-%s", uuid.NewString())

	state := &PipelineState{
		ID:          id,
		Status:      PipelineStatusPending,
		TotalStages: len(p.Stages),
		StartTime:   time.Now(),
		Pipeline:    &p,
	}

	pm.mu.Lock()
	pm.pipelines[id] = state
	pm.mu.Unlock()

	// Save initial state
	if err := pm.stateManager.SavePipeline(ctx, id, state); err != nil {
		logging.Logger().Error("failed to save pipeline state", zap.Error(err))
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
		// Return a copy to avoid data races
		stateCopy := loadedState
		return &stateCopy, nil
	}

	// Return a copy to avoid data races when caller reads fields
	stateCopy := *state
	return &stateCopy, nil
}

func (pm *PipelineManager) runPipeline(id string, p pipeline.Pipeline) {
	logging.Logger().Info("Starting pipeline execution", zap.String("pipeline_id", id))

	pm.updateStatus(id, PipelineStatusRunning, "")

	// Compile targets using pipeline layer (which uses recon utilities)
	targetsList := pipeline.CompileTargets(p)
	if len(targetsList) == 0 {
		pm.updateStatus(id, PipelineStatusFailed, "No targets found")
		return
	}

	// Calculate workers count (same as CLI)
	workersCount := min(pm.workerManager.MaxWorkers(), len(targetsList))

	logging.Logger().Info("Pipeline targets compiled",
		zap.String("pipeline_id", id),
		zap.Int("total_targets", len(targetsList)),
		zap.Int("workers_count", workersCount))

	// Shuffle targets to distribute them evenly across workers
	rand.Shuffle(len(targetsList), func(i, j int) {
		targetsList[i], targetsList[j] = targetsList[j], targetsList[i]
	})

	// Create worker pool with concurrency limit
	pool := pond.NewPool(workersCount)
	ctx := context.Background()

	var workersMu sync.Mutex
	var workers []*Worker

	// Process each chunk: create ephemeral worker -> execute -> delete worker
	for chunk := range slices.Chunk(targetsList, (len(targetsList)+workersCount-1)/workersCount) {
		pool.Submit(func() {
			// Create ephemeral worker
			worker, err := pm.workerManager.CreateEphemeralWorker(ctx)
			if err != nil {
				logging.Logger().Error("failed to create ephemeral worker", zap.Error(err))
				return
			}

			workersMu.Lock()
			workers = append(workers, worker)
			workersMu.Unlock()

			// Always delete worker after execution
			defer func() {
				logging.Logger().Debug("deleting ephemeral worker", zap.String("name", worker.Name))
				if err := pm.workerManager.DeleteEphemeralWorker(ctx, worker); err != nil {
					logging.Logger().Error("failed to delete ephemeral worker", zap.Error(err))
				}
			}()

			// Execute pipeline on worker
			if err := pipeline.ExecuteOnWorker(ctx, worker.Controller, p, chunk); err != nil {
				logging.Logger().Error("pipeline execution failed on worker",
					zap.String("worker_name", worker.Name),
					zap.Error(err))
				return
			}

			logging.Logger().Info("ephemeral worker completed successfully",
				zap.String("worker_name", worker.Name),
				zap.Int("targets_processed", len(chunk)))
		})
	}

	pm.saveWorkers(id, workers)
	pm.updateStatus(id, PipelineStatusCompleted, "")

	pool.StopAndWait()

	logging.Logger().Info("Pipeline execution completed", zap.String("pipeline_id", id))
}

func (pm *PipelineManager) saveWorkers(id string, workers []*Worker) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if state, exists := pm.pipelines[id]; exists {
		state.Workers = workers
		if err := pm.stateManager.SavePipeline(context.Background(), id, state); err != nil {
			logging.Logger().Error("failed to save pipeline state with workers", zap.Error(err))
		}
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
