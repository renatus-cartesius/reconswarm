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

	// Initialize Provisioner
	prov, err := provisioning.NewYcProvisioner(cfg.IAMToken, cfg.FolderID)
	if err != nil {
		return nil, fmt.Errorf("failed to create provisioner: %w", err)
	}

	// Initialize WorkerManager
	wm, err := manager.NewWorkerManager(cfg, sm, prov, control.NewController)
	if err != nil {
		return nil, fmt.Errorf("failed to create worker manager: %w", err)
	}

	// Initialize PipelineManager
	pm := manager.NewPipelineManager(wm, sm)

	return &Server{
		pipelineManager: pm,
		workerManager:   wm,
		stateManager:    sm,
	}, nil
}

// RunPipeline implements the RunPipeline RPC
func (s *Server) RunPipeline(ctx context.Context, req *api.RunPipelineRequest) (*api.RunPipelineResponse, error) {
	id, err := s.pipelineManager.SubmitPipeline(ctx, req.PipelineYaml)
	if err != nil {
		return nil, err
	}
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
