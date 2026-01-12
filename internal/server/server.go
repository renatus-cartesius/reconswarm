package server

import (
	"context"
	"fmt"
	"net"
	"reconswarm/api"
	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/logging"
	"reconswarm/internal/control/provisioning"
	"reconswarm/internal/server/manager"
	"reconswarm/internal/ssh"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Server represents the ReconSwarm gRPC server
type Server struct {
	api.UnimplementedReconSwarmServer
	pipelineManager *manager.PipelineManager
	workerManager   *manager.WorkerManager
	stateManager    manager.StateManager
	config          config.Config
}

// NewServer creates a new Server from configuration
func NewServer(cfg config.Config) (*Server, error) {
	// Initialize StateManager with etcd config
	sm, err := manager.NewStateManager(cfg.Etcd.Endpoints)
	if err != nil {
		return nil, fmt.Errorf("failed to create state manager: %w", err)
	}

	// Initialize SSH Key Provider (stores keys in etcd)
	keyProvider := ssh.NewKeyProvider(cfg.Etcd.Endpoints)

	// Initialize Provisioner using factory pattern
	prov, err := provisioning.NewProvisioner(cfg.Provisioner)
	if err != nil {
		return nil, fmt.Errorf("failed to create provisioner: %w", err)
	}

	// Initialize WorkerManager
	wm, err := manager.NewWorkerManager(cfg, sm, prov, control.NewController, keyProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create worker manager: %w", err)
	}

	// Initialize PipelineManager
	pm := manager.NewPipelineManager(wm, sm)

	return &Server{
		pipelineManager: pm,
		workerManager:   wm,
		stateManager:    sm,
		config:          cfg,
	}, nil
}

// NewServerWithDependencies creates a new Server with injected dependencies (for testing)
func NewServerWithDependencies(pm *manager.PipelineManager, wm *manager.WorkerManager, sm manager.StateManager) *Server {
	return &Server{
		pipelineManager: pm,
		workerManager:   wm,
		stateManager:    sm,
	}
}

// RunPipeline implements the RunPipeline RPC
func (s *Server) RunPipeline(ctx context.Context, req *api.RunPipelineRequest) (*api.RunPipelineResponse, error) {
	logging.Logger().Info("RunPipeline RPC called", zap.Int("yaml_length", len(req.PipelineYaml)))

	id, err := s.pipelineManager.SubmitPipeline(ctx, req.PipelineYaml)
	if err != nil {
		logging.Logger().Error("SubmitPipeline failed", zap.Error(err))
		return nil, err
	}

	logging.Logger().Info("Pipeline submitted", zap.String("pipeline_id", id))
	return &api.RunPipelineResponse{PipelineId: id}, nil
}

// GetStatus implements the GetStatus RPC
func (s *Server) GetStatus(ctx context.Context, req *api.GetStatusRequest) (*api.GetStatusResponse, error) {
	state, err := s.pipelineManager.GetStatus(ctx, req.PipelineId)
	if err != nil {
		return nil, err
	}

	resp := &api.GetStatusResponse{
		PipelineId:      state.ID,
		Status:          string(state.Status),
		Error:           state.Error,
		TotalStages:     int32(state.TotalStages),
		CompletedStages: int32(state.CompletedStages),
	}

	// Add worker status
	workers := s.workerManager.GetStatus()
	for _, w := range workers {
		if w.CurrentTask == req.PipelineId || w.Status == manager.WorkerStatusIdle {
			resp.Workers = append(resp.Workers, &api.WorkerStatus{
				WorkerId:    w.ID,
				Name:        w.Name,
				Status:      string(w.Status),
				CurrentTask: w.CurrentTask,
			})
		}
	}

	return resp, nil
}

// Start starts the gRPC server on the configured port
func (s *Server) Start() error {
	return s.StartOnPort(s.config.Server.Port)
}

// StartOnPort starts the gRPC server on a specific port (for backwards compatibility)
func (s *Server) StartOnPort(port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := grpc.NewServer()
	api.RegisterReconSwarmServer(grpcServer, s)

	logging.Logger().Info("Starting gRPC server", zap.Int("port", port))
	return grpcServer.Serve(lis)
}
