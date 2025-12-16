package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// StateManager defines the interface for state persistence
// Note: SSH key management is handled by ssh.KeyProvider
type StateManager interface {
	SavePipeline(ctx context.Context, pipelineID string, state any) error
	GetPipeline(ctx context.Context, pipelineID string, state any) error
	SaveWorker(ctx context.Context, workerID string, state any) error
	GetWorker(ctx context.Context, workerID string, state any) error
	DeleteWorker(ctx context.Context, workerID string) error
	ListWorkers(ctx context.Context) (map[string][]byte, error)
	Close() error
}

// EtcdStateManager handles state persistence using Etcd
type EtcdStateManager struct {
	client *clientv3.Client
}

// NewStateManager creates a new StateManager
func NewStateManager(endpoints []string) (StateManager, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}
	return &EtcdStateManager{client: cli}, nil
}

// Close closes the etcd client connection
func (sm *EtcdStateManager) Close() error {
	return sm.client.Close()
}

// SavePipeline saves the pipeline state
func (sm *EtcdStateManager) SavePipeline(ctx context.Context, pipelineID string, state any) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal pipeline state: %w", err)
	}
	_, err = sm.client.Put(ctx, fmt.Sprintf("/pipelines/%s", pipelineID), string(data))
	if err != nil {
		return fmt.Errorf("failed to save pipeline state to etcd: %w", err)
	}
	return nil
}

// GetPipeline retrieves the pipeline state
func (sm *EtcdStateManager) GetPipeline(ctx context.Context, pipelineID string, state any) error {
	resp, err := sm.client.Get(ctx, fmt.Sprintf("/pipelines/%s", pipelineID))
	if err != nil {
		return fmt.Errorf("failed to get pipeline state from etcd: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return fmt.Errorf("pipeline not found: %s", pipelineID)
	}
	if err := json.Unmarshal(resp.Kvs[0].Value, state); err != nil {
		return fmt.Errorf("failed to unmarshal pipeline state: %w", err)
	}
	return nil
}

// SaveWorker saves the worker state
func (sm *EtcdStateManager) SaveWorker(ctx context.Context, workerID string, state any) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal worker state: %w", err)
	}
	_, err = sm.client.Put(ctx, fmt.Sprintf("/workers/%s", workerID), string(data))
	if err != nil {
		return fmt.Errorf("failed to save worker state to etcd: %w", err)
	}
	return nil
}

// GetWorker retrieves the worker state
func (sm *EtcdStateManager) GetWorker(ctx context.Context, workerID string, state any) error {
	resp, err := sm.client.Get(ctx, fmt.Sprintf("/workers/%s", workerID))
	if err != nil {
		return fmt.Errorf("failed to get worker state from etcd: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return fmt.Errorf("worker not found: %s", workerID)
	}
	if err := json.Unmarshal(resp.Kvs[0].Value, state); err != nil {
		return fmt.Errorf("failed to unmarshal worker state: %w", err)
	}
	return nil
}

// DeleteWorker deletes the worker state
func (sm *EtcdStateManager) DeleteWorker(ctx context.Context, workerID string) error {
	_, err := sm.client.Delete(ctx, fmt.Sprintf("/workers/%s", workerID))
	if err != nil {
		return fmt.Errorf("failed to delete worker state from etcd: %w", err)
	}
	return nil
}

// ListWorkers returns a list of all workers
func (sm *EtcdStateManager) ListWorkers(ctx context.Context) (map[string][]byte, error) {
	resp, err := sm.client.Get(ctx, "/workers/", clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to list workers from etcd: %w", err)
	}
	workers := make(map[string][]byte)
	for _, kv := range resp.Kvs {
		workers[string(kv.Key)] = kv.Value
	}
	return workers, nil
}
