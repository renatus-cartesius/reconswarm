package pipeline

import (
	"bytes"
	"fmt"
	"text/template"

	"reconswarm/internal/control"
	"reconswarm/internal/logging"

	"go.uber.org/zap"
)

// Stage defines the interface for pipeline stages
type Stage interface {
	GetName() string
	GetType() string
	Execute(controller interface{}, targets []string, targetsFile string) error
}

// ExecStage represents an execution stage
type ExecStage struct {
	Name  string   `yaml:"name"`
	Type  string   `yaml:"type"`
	Steps []string `yaml:"steps"`
}

// SyncStage represents a file synchronization stage
type SyncStage struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
	Src  string `yaml:"src"`
	Dest string `yaml:"dest"`
}

// GetName returns the stage name
func (e *ExecStage) GetName() string {
	return e.Name
}

// GetType returns the stage type
func (e *ExecStage) GetType() string {
	return e.Type
}

// GetName returns the stage name
func (s *SyncStage) GetName() string {
	return s.Name
}

// GetType returns the stage type
func (s *SyncStage) GetType() string {
	return s.Type
}

// Execute executes the exec stage
func (e *ExecStage) Execute(controller interface{}, targets []string, targetsFile string) error {
	ctrl, ok := controller.(control.Controller)
	if !ok {
		return fmt.Errorf("invalid controller type for exec stage")
	}

	logging.Logger().Debug("executing exec stage", zap.String("stage_name", e.Name))

	// Create template context
	templateContext := map[string]interface{}{
		"Targets": map[string]interface{}{
			"filepath": targetsFile,
			"list":     targets,
		},
		"Worker": map[string]interface{}{
			"Name": ctrl.GetInstanceName(),
		},
	}

	// Execute each step in the stage
	for stepIndex, stepTemplate := range e.Steps {
		logging.Logger().Debug("executing step",
			zap.Int("step_index", stepIndex+1),
			zap.String("template", stepTemplate))

		// Render template
		renderedCommand, err := renderTemplate(stepTemplate, templateContext)
		if err != nil {
			return fmt.Errorf("failed to render template for step %d: %v", stepIndex+1, err)
		}

		logging.Logger().Debug("rendered command", zap.String("command", renderedCommand))

		// Execute the rendered command
		if err := ctrl.Run(renderedCommand); err != nil {
			return fmt.Errorf("failed to execute step %d: %v", stepIndex+1, err)
		}

		logging.Logger().Debug("step completed successfully", zap.Int("step_index", stepIndex+1))
	}

	return nil
}

// Execute executes the sync stage
func (s *SyncStage) Execute(controller interface{}, targets []string, targetsFile string) error {
	ctrl, ok := controller.(control.Controller)
	if !ok {
		return fmt.Errorf("invalid controller type for sync stage")
	}

	logging.Logger().Debug("executing sync stage", zap.String("stage_name", s.Name))

	// Create template context
	templateContext := map[string]interface{}{
		"Targets": map[string]interface{}{
			"filepath": targetsFile,
			"list":     targets,
		},
		"Worker": map[string]interface{}{
			"Name": ctrl.GetInstanceName(),
		},
	}

	// Render source path template
	renderedSrc, err := renderTemplate(s.Src, templateContext)
	if err != nil {
		return fmt.Errorf("failed to render source path template: %v", err)
	}

	// Render destination path template
	renderedDest, err := renderTemplate(s.Dest, templateContext)
	if err != nil {
		return fmt.Errorf("failed to render destination path template: %v", err)
	}

	logging.Logger().Debug("rendered sync paths",
		zap.String("src", renderedSrc),
		zap.String("dest", renderedDest))

	// Execute the file sync using SFTP
	if err := ctrl.SyncFile(renderedSrc, renderedDest); err != nil {
		return fmt.Errorf("failed to sync file: %v", err)
	}

	logging.Logger().Debug("sync stage completed successfully", zap.String("stage_name", s.Name))
	return nil
}

// renderTemplate renders a Go template with the given context
func renderTemplate(templateStr string, context map[string]interface{}) (string, error) {
	tmpl, err := template.New("command").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, context); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return buf.String(), nil
}

// RenderTemplate renders a Go template with the given context
func RenderTemplate(templateStr string, context map[string]interface{}) (string, error) {
	tmpl, err := template.New("command").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, context); err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}

	return buf.String(), nil
}
