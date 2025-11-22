package state

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// WorkerState represents the state of a single worker
type WorkerState struct {
	ID              string   `json:"id"`
	Targets         []string `json:"targets"`
	InstanceID      string   `json:"instance_id"`
	InstanceIP      string   `json:"instance_ip"`
	InstanceName    string   `json:"instance_name"`
	Status          string   `json:"status"` // "pending", "running", "completed", "failed"
	CompletedStages []string `json:"completed_stages"`
	Error           string   `json:"error,omitempty"`
}

// State represents the global execution state
type State struct {
	mu sync.RWMutex

	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Workers   map[string]WorkerState `json:"workers"`
}

// New creates a new empty state
func New() *State {
	return &State{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Workers:   make(map[string]WorkerState),
	}
}

// Load loads the state from a file
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &state, nil
}

// Save saves the state to a file
func (s *State) Save(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// UpdateWorker updates the state of a specific worker safely
func (s *State) UpdateWorker(id string, updateFn func(*WorkerState)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	worker, exists := s.Workers[id]
	if !exists {
		// If worker doesn't exist, we can't update it.
		// In a real scenario, we might want to create it or log an error.
		// For now, let's assume it should exist if we are updating it,
		// or we create a new one if that's the intended behavior.
		// Let's create a new one if it doesn't exist to be safe.
		worker = WorkerState{ID: id}
	}

	updateFn(&worker)
	s.Workers[id] = worker
}

// GetWorker returns a copy of the worker state
func (s *State) GetWorker(id string) (WorkerState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	worker, exists := s.Workers[id]
	return worker, exists
}

// AddWorker adds a new worker to the state
func (s *State) AddWorker(worker WorkerState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Workers[worker.ID] = worker
}
