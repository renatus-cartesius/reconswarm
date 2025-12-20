package pipeline

import (
	"context"

	"reconswarm/internal/control"
	"reconswarm/internal/logging"
	"reconswarm/internal/recon/targets"

	"go.uber.org/zap"
)

// Pipeline represents the main pipeline configuration.
type Pipeline struct {
	Targets []Target `yaml:"targets"`
	Stages  []Stage  `yaml:"stages"`
}

// PipelineRaw represents the raw pipeline configuration for YAML deserialization
type PipelineRaw struct {
	Targets []Target   `yaml:"targets"`
	Stages  []StageRaw `yaml:"stages"`
}

// Target represents a target source for the pipeline.
// It defines how to obtain targets (from a list, external URL, crt.sh, etc.)
type Target struct {
	Type  string `yaml:"type,omitempty"`
	Value any    `yaml:"value,omitempty"`
}

// Stage defines the interface for pipeline stages
type Stage interface {
	GetName() string
	GetType() string
	Execute(ctx context.Context, controller control.Controller, targets []string, targetsFile string) error
}

// StageRaw represents a raw stage for YAML deserialization
type StageRaw struct {
	Name  string   `yaml:"name"`
	Type  string   `yaml:"type"`
	Steps []string `yaml:"steps,omitempty"`
	Src   string   `yaml:"src,omitempty"`
	Dest  string   `yaml:"dest,omitempty"`
}

// ToPipeline converts PipelineRaw to Pipeline
func (pr *PipelineRaw) ToPipeline() Pipeline {
	p := Pipeline{
		Targets: pr.Targets,
		Stages:  make([]Stage, len(pr.Stages)),
	}

	for i, stageRaw := range pr.Stages {
		switch stageRaw.Type {
		case "exec":
			p.Stages[i] = &ExecStage{
				Name:  stageRaw.Name,
				Type:  stageRaw.Type,
				Steps: stageRaw.Steps,
			}
		case "sync":
			p.Stages[i] = &SyncStage{
				Name: stageRaw.Name,
				Type: stageRaw.Type,
				Src:  stageRaw.Src,
				Dest: stageRaw.Dest,
			}
		default:
			// Skip unknown stage types
			continue
		}
	}

	return p
}

// CompileTargets compiles all targets from the pipeline by processing each target definition.
// It uses the recon layer utilities to fetch targets from various sources.
func CompileTargets(p Pipeline) []string {
	var result []string

	logging.Logger().Info("compiling target list")

	for _, target := range p.Targets {
		switch target.Type {
		case "crtsh":
			domain, ok := target.Value.(string)
			if !ok {
				logging.Logger().Error("invalid type for crtsh target, expected domain string", zap.Any("value", target.Value))
				continue
			}
			// Add the domain itself
			result = append(result, domain)
			// Fetch subdomains from crt.sh
			t, err := targets.FromCrtsh(domain)
			if err != nil {
				logging.Logger().Error("error fetching crtsh targets", zap.String("domain", domain), zap.Error(err))
				continue
			}
			result = append(result, t...)
			logging.Logger().Info("added crtsh resolved targets", zap.String("domain", domain), zap.Int("count", len(t)))

		case "list":
			t := targets.FromList(target.Value)
			result = append(result, t...)

		case "external_list":
			url, ok := target.Value.(string)
			if !ok {
				logging.Logger().Error("invalid type for external_list target, expected URL string", zap.Any("value", target.Value))
				continue
			}
			t, err := targets.FromExternalList(url)
			if err != nil {
				logging.Logger().Error("error fetching external list", zap.String("url", url), zap.Error(err))
				continue
			}
			result = append(result, t...)
			logging.Logger().Info("loaded external list", zap.String("url", url), zap.Int("count", len(t)))

		default:
			logging.Logger().Error("unknown target type", zap.String("type", target.Type))
		}
	}

	logging.Logger().Info("compiled targets list", zap.Int("total_targets", len(result)))

	return result
}
