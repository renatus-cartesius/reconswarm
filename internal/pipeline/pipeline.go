package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"reconswarm/internal/control"
	"reconswarm/internal/logging"
	"reconswarm/internal/recon/targets"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// Pipeline represents the main pipeline configuration.
type Pipeline struct {
	Targets []Target `yaml:"targets"`
	Stages  []Stage  `yaml:"stages"`
}

// Target represents a target source for the pipeline.
// It defines how to obtain targets (from a list, external URL, crt.sh, etc.)
type Target struct {
	Type  string `yaml:"type,omitempty" json:"type,omitempty"`
	Value any    `yaml:"value,omitempty" json:"value,omitempty"`
}

// Stage defines the interface for pipeline stages
type Stage interface {
	GetName() string
	GetType() string
	Execute(ctx context.Context, controller control.Controller, targets []string, targetsFile string) error
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for Pipeline
func (p *Pipeline) UnmarshalYAML(node *yaml.Node) error {
	// Interim structure to decode targets and raw stages
	var raw struct {
		Targets []Target    `yaml:"targets"`
		Stages  []yaml.Node `yaml:"stages"`
	}

	if err := node.Decode(&raw); err != nil {
		return err
	}

	p.Targets = raw.Targets
	p.Stages = make([]Stage, 0, len(raw.Stages))

	for _, stageNode := range raw.Stages {
		// Interim structure to determine stage type
		var typeMeta struct {
			Type string `yaml:"type"`
		}

		if err := stageNode.Decode(&typeMeta); err != nil {
			return fmt.Errorf("failed to decode stage type: %w", err)
		}

		var stage Stage
		switch typeMeta.Type {
		case "exec":
			var s ExecStage
			if err := stageNode.Decode(&s); err != nil {
				return fmt.Errorf("failed to decode exec stage: %w", err)
			}
			stage = &s
		case "sync":
			var s SyncStage
			if err := stageNode.Decode(&s); err != nil {
				return fmt.Errorf("failed to decode sync stage: %w", err)
			}
			stage = &s
		default:
			logging.Logger().Warn("unknown stage type, skipping", zap.String("type", typeMeta.Type))
			continue
		}

		p.Stages = append(p.Stages, stage)
	}

	return nil
}

// MarshalJSON implements json.Marshaler for Pipeline.
// Serializes Stage interface slice using a type-discriminated format.
func (p Pipeline) MarshalJSON() ([]byte, error) {
	type rawStage struct {
		Name  string   `json:"name"`
		Type  string   `json:"type"`
		Steps []string `json:"steps,omitempty"`
		Src   string   `json:"src,omitempty"`
		Dest  string   `json:"dest,omitempty"`
	}

	stages := make([]rawStage, 0, len(p.Stages))
	for _, s := range p.Stages {
		rs := rawStage{
			Name: s.GetName(),
			Type: s.GetType(),
		}
		switch v := s.(type) {
		case *ExecStage:
			rs.Steps = v.Steps
		case *SyncStage:
			rs.Src = v.Src
			rs.Dest = v.Dest
		}
		stages = append(stages, rs)
	}

	return json.Marshal(struct {
		Targets []Target   `json:"targets"`
		Stages  []rawStage `json:"stages"`
	}{
		Targets: p.Targets,
		Stages:  stages,
	})
}

// UnmarshalJSON implements json.Unmarshaler for Pipeline.
// Deserializes Stage interface slice using the "type" field as discriminator.
func (p *Pipeline) UnmarshalJSON(data []byte) error {
	var raw struct {
		Targets []Target          `json:"targets"`
		Stages  []json.RawMessage `json:"stages"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.Targets = raw.Targets
	p.Stages = make([]Stage, 0, len(raw.Stages))

	for _, stageData := range raw.Stages {
		var meta struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(stageData, &meta); err != nil {
			return fmt.Errorf("failed to decode stage type: %w", err)
		}

		switch meta.Type {
		case "exec":
			var s ExecStage
			if err := json.Unmarshal(stageData, &s); err != nil {
				return fmt.Errorf("failed to decode exec stage: %w", err)
			}
			p.Stages = append(p.Stages, &s)
		case "sync":
			var s SyncStage
			if err := json.Unmarshal(stageData, &s); err != nil {
				return fmt.Errorf("failed to decode sync stage: %w", err)
			}
			p.Stages = append(p.Stages, &s)
		default:
			return fmt.Errorf("unknown stage type: %s", meta.Type)
		}
	}

	return nil
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

		case "shell_output":
			cmd, ok := target.Value.(string)
			if !ok {
				logging.Logger().Error("invalid type for shell_output target, expected command string", zap.Any("value", target.Value))
				continue
			}

			t, err := targets.FromShellOutput(cmd)
			if err != nil {
				logging.Logger().Error("error on running command for shell_output", zap.String("command", cmd), zap.Error(err))
				continue
			}

			result = append(result, t...)
			logging.Logger().Info("loaded output from shell command", zap.Int("count", len(t)))

		default:
			logging.Logger().Error("unknown target type", zap.String("type", target.Type))
		}
	}

	logging.Logger().Info("compiled targets list", zap.Int("total_targets", len(result)))

	return result
}
