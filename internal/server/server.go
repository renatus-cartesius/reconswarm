package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"reconswarm/api"
	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/logging"
	"reconswarm/internal/manager"
	"reconswarm/internal/provisioning"
	"reconswarm/internal/ssh"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Server represents the ReconSwarm gRPC server
type Server struct {
	api.UnimplementedReconSwarmServer
	pipelineManager *manager.PipelineManager
	workerManager   *manager.WorkerManager
	stateManager    manager.StateManager
}

// NewServer creates a new Server
func NewServer(cfg config.Config) (*Server, error) {
	// Initialize StateManager
	etcdEndpoints := []string{"localhost:2379"}
	if envEndpoints := os.Getenv("ETCD_ENDPOINTS"); envEndpoints != "" {
		etcdEndpoints = strings.Split(envEndpoints, ",")
	}
	sm, err := manager.NewStateManager(etcdEndpoints)
	if err != nil {
		return nil, fmt.Errorf("failed to create state manager: %w", err)
	}

	// Initialize SSH Key Provider (stores keys in etcd)
	keyProvider := ssh.NewKeyProvider(etcdEndpoints)

	// Initialize Provisioner
	prov, err := provisioning.NewYcProvisioner(cfg.IAMToken, cfg.FolderID)
	if err != nil {
		return nil, fmt.Errorf("failed to create provisioner: %w", err)
	}

	// Initialize WorkerManager with SSH key provider
	wm, err := manager.NewWorkerManager(cfg, sm, prov, control.NewController, keyProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create worker manager: %w", err)
	}

	// Initialize PipelineManager
	pm := manager.NewPipelineManager(wm, sm)

	return NewServerWithDependencies(pm, wm, sm), nil
}

// NewServerWithDependencies creates a new Server with injected dependencies
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
		// Only include workers relevant to this pipeline?
		// Or all workers? The requirement says "information about workers".
		// If we filter by pipeline, we might miss idle workers or workers on other pipelines.
		// But GetStatusRequest has PipelineId.
		// If PipelineId is empty, maybe return all?
		// But the proto says GetStatusRequest takes PipelineId.
		// Let's return all workers for now, or maybe filter if they are working on this pipeline.

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

// Start starts the gRPC server
func (s *Server) Start(port int) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := grpc.NewServer()
	api.RegisterReconSwarmServer(grpcServer, s)

	logging.Logger().Info("Starting gRPC server", zap.Int("port", port))
	return grpcServer.Serve(lis)
}
