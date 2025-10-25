package pipeline

// Pipeline represents the main pipeline configuration
type Pipeline struct {
	Targets []Target `yaml:"targets"`
	Stages  []Stage  `yaml:"stages"`
}

// PipelineRaw represents the raw pipeline configuration for YAML deserialization
type PipelineRaw struct {
	Targets []Target   `yaml:"targets"`
	Stages  []StageRaw `yaml:"stages"`
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

// Target represents a target for reconnaissance
type Target struct {
	Type  string `yaml:"type,omitempty"`
	Value any    `yaml:"value,omitempty"`
}
