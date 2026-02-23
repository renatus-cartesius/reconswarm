package server

import (
	"context"
	"fmt"
	"net"
	"reconswarm/api"
	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/logging"
	"reconswarm/internal/pipeline"
	"reconswarm/internal/provisioning"
	"reconswarm/internal/server/manager"
	"reconswarm/internal/ssh"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Server represents the ReconSwarm gRPC server
type Server struct {
	api.UnimplementedReconSwarmServer
	pipelineManager *manager.PipelineManager
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
		stateManager:    sm,
		config:          cfg,
	}, nil
}

// NewServerWithDependencies creates a new Server with injected dependencies (for testing)
func NewServerWithDependencies(pm *manager.PipelineManager, sm manager.StateManager) *Server {
	return &Server{
		pipelineManager: pm,
		stateManager:    sm,
	}
}

// RunPipeline implements the RunPipeline RPC
func (s *Server) RunPipeline(ctx context.Context, req *api.RunPipelineRequest) (*api.RunPipelineResponse, error) {
	if req.Pipeline == nil {
		return nil, fmt.Errorf("pipeline is required")
	}

	logging.Logger().Info("RunPipeline RPC called",
		zap.Int("targets", len(req.Pipeline.Targets)),
		zap.Int("stages", len(req.Pipeline.Stages)))

	// Convert proto Pipeline to domain Pipeline
	p := protoToPipeline(req.Pipeline)

	id, err := s.pipelineManager.SubmitPipeline(ctx, p)
	if err != nil {
		logging.Logger().Error("SubmitPipeline failed", zap.Error(err))
		return nil, err
	}

	logging.Logger().Info("Pipeline submitted", zap.String("pipeline_id", id))
	return &api.RunPipelineResponse{PipelineId: id}, nil
}

// GetPipeline implements the GetPipeline RPC
func (s *Server) GetPipeline(ctx context.Context, req *api.GetPipelineRequest) (*api.GetPipelineResponse, error) {
	state, err := s.pipelineManager.GetStatus(ctx, req.PipelineId)
	if err != nil {
		return nil, err
	}

	resp := &api.GetPipelineResponse{
		PipelineId:      state.ID,
		Status:          string(state.Status),
		Error:           state.Error,
		TotalStages:     int32(state.TotalStages),
		CompletedStages: int32(state.CompletedStages),
	}

	// Include pipeline definition if available (in-memory pipelines only)
	if state.Pipeline != nil {
		resp.Pipeline = pipelineToProto(*state.Pipeline)
	}

	// Workers are bound to the pipeline state
	for _, w := range state.Workers {
		resp.Workers = append(resp.Workers, &api.Worker{
			WorkerId: w.ID,
			Name:     w.Name,
			Status:   string(w.Status),
		})
	}

	return resp, nil
}

// Start starts the gRPC server on the configured port
func (s *Server) Start() error {
	return s.StartOnPort(s.config.Server.Port)
}

func (s *Server) ListPipelines(ctx context.Context, req *api.ListPipelinesRequest) (*api.ListPipelinesResponse, error){
	return nil, nil
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

// =================== Proto <-> Domain Conversion ===================

// protoToPipeline converts an api.Pipeline proto message to a domain pipeline.Pipeline.
func protoToPipeline(p *api.Pipeline) pipeline.Pipeline {
	var pip pipeline.Pipeline

	for _, t := range p.GetTargets() {
		target := pipeline.Target{Type: t.GetType()}
		if len(t.GetListValue()) > 0 {
			// Convert []string to []any for compatibility with targets.FromList
			vals := make([]any, len(t.GetListValue()))
			for i, v := range t.GetListValue() {
				vals[i] = v
			}
			target.Value = vals
		} else {
			target.Value = t.GetStringValue()
		}
		pip.Targets = append(pip.Targets, target)
	}

	for _, s := range p.GetStages() {
		switch cfg := s.GetConfig().(type) {
		case *api.Stage_Exec:
			pip.Stages = append(pip.Stages, &pipeline.ExecStage{
				Name:  s.GetName(),
				Type:  "exec",
				Steps: cfg.Exec.GetSteps(),
			})
		case *api.Stage_Sync:
			pip.Stages = append(pip.Stages, &pipeline.SyncStage{
				Name: s.GetName(),
				Type: "sync",
				Src:  cfg.Sync.GetSrc(),
				Dest: cfg.Sync.GetDest(),
			})
		}
	}

	return pip
}

// pipelineToProto converts a domain pipeline.Pipeline to an api.Pipeline proto message.
func pipelineToProto(p pipeline.Pipeline) *api.Pipeline {
	proto := &api.Pipeline{}

	for _, t := range p.Targets {
		pt := &api.Target{Type: t.Type}
		switch v := t.Value.(type) {
		case string:
			pt.StringValue = v
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					pt.ListValue = append(pt.ListValue, s)
				}
			}
		case []string:
			pt.ListValue = v
		}
		proto.Targets = append(proto.Targets, pt)
	}

	for _, s := range p.Stages {
		switch stage := s.(type) {
		case *pipeline.ExecStage:
			proto.Stages = append(proto.Stages, &api.Stage{
				Name: stage.Name,
				Config: &api.Stage_Exec{
					Exec: &api.ExecStage{Steps: stage.Steps},
				},
			})
		case *pipeline.SyncStage:
			proto.Stages = append(proto.Stages, &api.Stage{
				Name: stage.Name,
				Config: &api.Stage_Sync{
					Sync: &api.SyncStage{Src: stage.Src, Dest: stage.Dest},
				},
			})
		}
	}

	return proto
}
