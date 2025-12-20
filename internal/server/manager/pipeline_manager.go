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

// pipelineWrapper is used to parse YAML files with "pipeline:" root key
type pipelineWrapper struct {
	Pipeline pipeline.PipelineRaw `yaml:"pipeline"`
}

// SubmitPipeline submits a pipeline for execution
func (pm *PipelineManager) SubmitPipeline(ctx context.Context, yamlContent string) (string, error) {

	// Try to parse with "pipeline:" wrapper first
	var wrapper pipelineWrapper
	if err := yaml.Unmarshal([]byte(yamlContent), &wrapper); err != nil {
		return "", fmt.Errorf("failed to parse pipeline YAML: %w", err)
	}

	// Use the wrapper if it has content, otherwise try direct parsing
	var rawPipeline pipeline.PipelineRaw
	if len(wrapper.Pipeline.Targets) > 0 || len(wrapper.Pipeline.Stages) > 0 {
		rawPipeline = wrapper.Pipeline
		logging.Logger().Debug("Parsed pipeline with wrapper",
			zap.Int("targets", len(rawPipeline.Targets)),
			zap.Int("stages", len(rawPipeline.Stages)))
	} else {
		// Fallback: try parsing without wrapper
		if err := yaml.Unmarshal([]byte(yamlContent), &rawPipeline); err != nil {
			return "", fmt.Errorf("failed to parse pipeline YAML (direct): %w", err)
		}
		logging.Logger().Debug("Parsed pipeline directly",
			zap.Int("targets", len(rawPipeline.Targets)),
			zap.Int("stages", len(rawPipeline.Stages)))
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

	// Process each chunk: create ephemeral worker -> execute -> delete worker
	for chunk := range slices.Chunk(targetsList, (len(targetsList)+workersCount-1)/workersCount) {
		pool.Submit(func() {
			logging.Logger().Info("started ephemeral worker", zap.Int("targets_count", len(chunk)))

			// Create ephemeral worker
			worker, err := pm.workerManager.CreateEphemeralWorker(ctx)
			if err != nil {
				logging.Logger().Error("failed to create ephemeral worker", zap.Error(err))
				return
			}

			// Always delete worker after execution
			defer func() {
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

	pool.StopAndWait()

	pm.updateStatus(id, PipelineStatusCompleted, "")
	logging.Logger().Info("Pipeline execution completed", zap.String("pipeline_id", id))
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
