package state

import (
	"os"
	"testing"
)

func TestStatePersistence(t *testing.T) {
	tmpFile := "test_state.json"
	defer os.Remove(tmpFile)

	// Create new state
	s := New()
	workerID := "worker-1"
	s.AddWorker(WorkerState{
		ID:      workerID,
		Targets: []string{"example.com"},
		Status:  "pending",
	})

	// Save state
	if err := s.Save(tmpFile); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Load state
	loaded, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	// Verify contents
	w, exists := loaded.GetWorker(workerID)
	if !exists {
		t.Fatal("Worker not found in loaded state")
	}
	if w.Status != "pending" {
		t.Errorf("Expected status pending, got %s", w.Status)
	}
	if len(w.Targets) != 1 || w.Targets[0] != "example.com" {
		t.Errorf("Targets mismatch")
	}
}

func TestUpdateWorker(t *testing.T) {
	s := New()
	id := "worker-1"
	s.AddWorker(WorkerState{ID: id, Status: "pending"})

	s.UpdateWorker(id, func(w *WorkerState) {
		w.Status = "running"
		w.InstanceID = "inst-1"
	})

	w, _ := s.GetWorker(id)
	if w.Status != "running" {
		t.Errorf("Expected status running, got %s", w.Status)
	}
	if w.InstanceID != "inst-1" {
		t.Errorf("Expected instance ID inst-1, got %s", w.InstanceID)
	}
}
