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
// Workers are stored as part of PipelineState, not separately.
type StateManager interface {
	SavePipeline(ctx context.Context, pipelineID string, pipelineState *PipelineState) error
	GetPipeline(ctx context.Context, pipelineID string, state any) error
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
func (sm *EtcdStateManager) SavePipeline(ctx context.Context, pipelineID string, pipelineState *PipelineState) error {
	data, err := json.Marshal(pipelineState)
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

