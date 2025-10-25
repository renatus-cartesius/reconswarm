package recon

import (
	"os"
	"reconswarm/internal/config"
	"reconswarm/internal/pipeline"
	"strings"
	"testing"
)

// mockController is a mock implementation of the Controller interface for testing
type mockController struct {
	instanceName string
	commands     []string
	syncedFiles  []syncedFile
}

type syncedFile struct {
	remotePath string
	localPath  string
}

func (m *mockController) Close() error {
	return nil
}

func (m *mockController) Run(command string) error {
	m.commands = append(m.commands, command)
	return nil
}

func (m *mockController) ReadFile(remotePath string) (string, error) {
	return "mock file content", nil
}

func (m *mockController) WriteFile(remotePath, content string, mode os.FileMode) error {
	return nil
}

func (m *mockController) GetInstanceName() string {
	return m.instanceName
}

func (m *mockController) SyncFile(remotePath, localPath string) error {
	m.syncedFiles = append(m.syncedFiles, syncedFile{
		remotePath: remotePath,
		localPath:  localPath,
	})
	return nil
}

func TestPrepareTargets(t *testing.T) {
	// Set up mock crt.sh client
	mockClient := NewMockCrtshClient()
	mockClient.SetMockResults("example.com", []string{"www.example.com", "api.example.com"})
	SetCrtshClient(mockClient)
	defer SetCrtshClient(&DefaultCrtshClient{}) // Restore default client

	cfg := config.Config{
		PipelineRaw: pipeline.PipelineRaw{
			Targets: []pipeline.Target{
				{
					Type:  "crtsh",
					Value: "example.com",
				},
				{
					Type:  "list",
					Value: []any{"test1.com", "test2.com"},
				},
			},
		},
	}

	targets := PrepareTargets(cfg)

	// Should contain the original domain + mock subdomains + list targets
	expectedTargets := []string{"example.com", "www.example.com", "api.example.com", "test1.com", "test2.com"}

	if len(targets) != len(expectedTargets) {
		t.Errorf("Expected %d targets, got %d", len(expectedTargets), len(targets))
	}

	// Check that all expected targets are present
	for _, expected := range expectedTargets {
		found := false
		for _, target := range targets {
			if target == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected target '%s' not found in results", expected)
		}
	}
}

func TestRenderTemplate(t *testing.T) {
	templateStr := "echo 'Targets: {{.Targets.filepath}}'"
	context := map[string]interface{}{
		"Targets": map[string]interface{}{
			"filepath": "/opt/recon/targets.txt",
			"list":     []string{"example.com"},
		},
		"Worker": map[string]interface{}{
			"Name": "test-worker",
		},
	}

	result, err := pipeline.RenderTemplate(templateStr, context)
	if err != nil {
		t.Fatalf("Failed to render template: %v", err)
	}

	expected := "echo 'Targets: /opt/recon/targets.txt'"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestRenderTemplateWithWorker(t *testing.T) {
	templateStr := "docker run --name {{.Worker.Name}} test-image"
	context := map[string]interface{}{
		"Targets": map[string]interface{}{
			"filepath": "/opt/recon/targets.txt",
			"list":     []string{"example.com"},
		},
		"Worker": map[string]interface{}{
			"Name": "test-worker",
		},
	}

	result, err := pipeline.RenderTemplate(templateStr, context)
	if err != nil {
		t.Fatalf("Failed to render template: %v", err)
	}

	expected := "docker run --name test-worker test-image"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestExecuteExecStage_BasicTemplate(t *testing.T) {
	// Create mock controller
	controller := &mockController{
		instanceName: "test-instance-123",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	// Create test stage
	stage := &pipeline.ExecStage{
		Name: "Test Stage",
		Type: "exec",
		Steps: []string{
			"echo 'Hello {{.Worker.Name}}'",
			"docker run --name {{.Worker.Name}} test-image",
		},
	}

	targets := []string{"example.com", "test.com"}
	targetsFile := "/opt/recon/targets-20231201-120000.txt"

	// Execute the stage
	err := stage.Execute(controller, targets, targetsFile)
	if err != nil {
		t.Fatalf("Failed to execute exec stage: %v", err)
	}

	// Verify commands were executed
	expectedCommands := []string{
		"echo 'Hello test-instance-123'",
		"docker run --name test-instance-123 test-image",
	}

	if len(controller.commands) != len(expectedCommands) {
		t.Fatalf("Expected %d commands, got %d", len(expectedCommands), len(controller.commands))
	}

	for i, expected := range expectedCommands {
		if controller.commands[i] != expected {
			t.Errorf("Command %d: expected '%s', got '%s'", i, expected, controller.commands[i])
		}
	}
}

func TestExecuteExecStage_TargetsTemplate(t *testing.T) {
	controller := &mockController{
		instanceName: "worker-456",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	stage := &pipeline.ExecStage{
		Name: "Targets Test Stage",
		Type: "exec",
		Steps: []string{
			"nuclei -l {{.Targets.filepath}} -o /opt/recon/{{.Worker.Name}}.txt",
			"echo 'Processed {{len .Targets.list}} targets'",
		},
	}

	targets := []string{"example.com", "test.com", "demo.org"}
	targetsFile := "/opt/recon/targets-20231201-120000.txt"

	err := stage.Execute(controller, targets, targetsFile)
	if err != nil {
		t.Fatalf("Failed to execute exec stage: %v", err)
	}

	expectedCommands := []string{
		"nuclei -l /opt/recon/targets-20231201-120000.txt -o /opt/recon/worker-456.txt",
		"echo 'Processed 3 targets'",
	}

	if len(controller.commands) != len(expectedCommands) {
		t.Fatalf("Expected %d commands, got %d", len(expectedCommands), len(controller.commands))
	}

	for i, expected := range expectedCommands {
		if controller.commands[i] != expected {
			t.Errorf("Command %d: expected '%s', got '%s'", i, expected, controller.commands[i])
		}
	}
}

func TestExecuteExecStage_ComplexTemplate(t *testing.T) {
	controller := &mockController{
		instanceName: "reconswarm-demo-abc123",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	stage := &pipeline.ExecStage{
		Name: "Complex Test Stage",
		Type: "exec",
		Steps: []string{
			"docker run --rm -v /opt/recon:/data -v {{.Targets.filepath}}:/data/hosts.txt projectdiscovery/nuclei:latest -l /data/hosts.txt -o /data/{{.Worker.Name}}.txt",
			"docker run --rm -v /opt/recon:/data -v {{.Targets.filepath}}:/data/hosts.txt projectdiscovery/subfinder:latest -dL /data/hosts.txt -o /data/{{.Worker.Name}}-subfinder.txt",
		},
	}

	targets := []string{"example.com", "test.com"}
	targetsFile := "/opt/recon/targets-20231201-120000.txt"

	err := stage.Execute(controller, targets, targetsFile)
	if err != nil {
		t.Fatalf("Failed to execute exec stage: %v", err)
	}

	expectedCommands := []string{
		"docker run --rm -v /opt/recon:/data -v /opt/recon/targets-20231201-120000.txt:/data/hosts.txt projectdiscovery/nuclei:latest -l /data/hosts.txt -o /data/reconswarm-demo-abc123.txt",
		"docker run --rm -v /opt/recon:/data -v /opt/recon/targets-20231201-120000.txt:/data/hosts.txt projectdiscovery/subfinder:latest -dL /data/hosts.txt -o /data/reconswarm-demo-abc123-subfinder.txt",
	}

	if len(controller.commands) != len(expectedCommands) {
		t.Fatalf("Expected %d commands, got %d", len(expectedCommands), len(controller.commands))
	}

	for i, expected := range expectedCommands {
		if controller.commands[i] != expected {
			t.Errorf("Command %d: expected '%s', got '%s'", i, expected, controller.commands[i])
		}
	}
}

func TestExecuteExecStage_EmptySteps(t *testing.T) {
	controller := &mockController{
		instanceName: "test-instance",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	stage := &pipeline.ExecStage{
		Name:  "Empty Stage",
		Type:  "exec",
		Steps: []string{},
	}

	targets := []string{"example.com"}
	targetsFile := "/opt/recon/targets.txt"

	err := stage.Execute(controller, targets, targetsFile)
	if err != nil {
		t.Fatalf("Failed to execute exec stage: %v", err)
	}

	// Should have no commands executed
	if len(controller.commands) != 0 {
		t.Errorf("Expected 0 commands, got %d", len(controller.commands))
	}
}

func TestExecuteExecStage_InvalidTemplate(t *testing.T) {
	controller := &mockController{
		instanceName: "test-instance",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	stage := &pipeline.ExecStage{
		Name: "Invalid Template Stage",
		Type: "exec",
		Steps: []string{
			"echo 'Hello {{.Invalid.Field'", // Missing closing brace
		},
	}

	targets := []string{"example.com"}
	targetsFile := "/opt/recon/targets.txt"

	err := stage.Execute(controller, targets, targetsFile)
	if err == nil {
		t.Error("Expected error for invalid template, but got none")
	}

	// Should have no commands executed due to template error
	if len(controller.commands) != 0 {
		t.Errorf("Expected 0 commands due to template error, got %d", len(controller.commands))
	}
}

func TestRenderTemplate_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		template string
		context  map[string]interface{}
		expected string
		hasError bool
	}{
		{
			name:     "Empty template",
			template: "",
			context: map[string]interface{}{
				"Worker": map[string]interface{}{"Name": "test"},
			},
			expected: "",
			hasError: false,
		},
		{
			name:     "Template with no variables",
			template: "echo 'Hello World'",
			context: map[string]interface{}{
				"Worker": map[string]interface{}{"Name": "test"},
			},
			expected: "echo 'Hello World'",
			hasError: false,
		},
		{
			name:     "Template with multiple variables",
			template: "{{.Worker.Name}} processes {{len .Targets.list}} targets from {{.Targets.filepath}}",
			context: map[string]interface{}{
				"Worker": map[string]interface{}{"Name": "worker-123"},
				"Targets": map[string]interface{}{
					"filepath": "/opt/recon/targets.txt",
					"list":     []string{"a.com", "b.com", "c.com"},
				},
			},
			expected: "worker-123 processes 3 targets from /opt/recon/targets.txt",
			hasError: false,
		},
		{
			name:     "Template with special characters",
			template: "echo 'Worker: {{.Worker.Name}}' && echo 'File: {{.Targets.filepath}}'",
			context: map[string]interface{}{
				"Worker": map[string]interface{}{"Name": "test-worker-123"},
				"Targets": map[string]interface{}{
					"filepath": "/opt/recon/targets-2023-12-01.txt",
				},
			},
			expected: "echo 'Worker: test-worker-123' && echo 'File: /opt/recon/targets-2023-12-01.txt'",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := pipeline.RenderTemplate(tt.template, tt.context)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestExecuteSyncStage_BasicSync(t *testing.T) {
	controller := &mockController{
		instanceName: "test-worker-123",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	stage := &pipeline.SyncStage{
		Name: "Basic Sync Stage",
		Type: "sync",
		Src:  "/opt/recon/results-{{.Worker.Name}}.json",
		Dest: "./results/results-{{.Worker.Name}}.json",
	}

	targets := []string{"example.com", "test.com"}
	targetsFile := "/opt/recon/targets-1234567890.txt"

	err := stage.Execute(controller, targets, targetsFile)
	if err != nil {
		t.Fatalf("Failed to execute sync stage: %v", err)
	}

	// Verify file was synced
	if len(controller.syncedFiles) != 1 {
		t.Fatalf("Expected 1 synced file, got %d", len(controller.syncedFiles))
	}

	expectedSrc := "/opt/recon/results-test-worker-123.json"
	expectedDest := "./results/results-test-worker-123.json"

	if controller.syncedFiles[0].remotePath != expectedSrc {
		t.Errorf("Expected src '%s', got '%s'", expectedSrc, controller.syncedFiles[0].remotePath)
	}

	if controller.syncedFiles[0].localPath != expectedDest {
		t.Errorf("Expected dest '%s', got '%s'", expectedDest, controller.syncedFiles[0].localPath)
	}
}

func TestExecuteSyncStage_TargetsTemplate(t *testing.T) {
	controller := &mockController{
		instanceName: "worker-456",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	stage := &pipeline.SyncStage{
		Name: "Targets Sync Stage",
		Type: "sync",
		Src:  "{{.Targets.filepath}}",
		Dest: "./backup/targets-{{.Worker.Name}}.txt",
	}

	targets := []string{"example.com", "test.com", "demo.org"}
	targetsFile := "/opt/recon/targets-1234567890.txt"

	err := stage.Execute(controller, targets, targetsFile)
	if err != nil {
		t.Fatalf("Failed to execute sync stage: %v", err)
	}

	// Verify file was synced
	if len(controller.syncedFiles) != 1 {
		t.Fatalf("Expected 1 synced file, got %d", len(controller.syncedFiles))
	}

	expectedSrc := "/opt/recon/targets-1234567890.txt"
	expectedDest := "./backup/targets-worker-456.txt"

	if controller.syncedFiles[0].remotePath != expectedSrc {
		t.Errorf("Expected src '%s', got '%s'", expectedSrc, controller.syncedFiles[0].remotePath)
	}

	if controller.syncedFiles[0].localPath != expectedDest {
		t.Errorf("Expected dest '%s', got '%s'", expectedDest, controller.syncedFiles[0].localPath)
	}
}

func TestExecuteSyncStage_InvalidTemplate(t *testing.T) {
	controller := &mockController{
		instanceName: "test-instance",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	stage := &pipeline.SyncStage{
		Name: "Invalid Template Sync Stage",
		Type: "sync",
		Src:  "/opt/recon/{{.Invalid.Field'", // Missing closing brace
		Dest: "./results/test.json",
	}

	targets := []string{"example.com"}
	targetsFile := "/opt/recon/targets.txt"

	err := stage.Execute(controller, targets, targetsFile)
	if err == nil {
		t.Error("Expected error for invalid template, but got none")
	}

	// Should have no synced files due to template error
	if len(controller.syncedFiles) != 0 {
		t.Errorf("Expected 0 synced files due to template error, got %d", len(controller.syncedFiles))
	}
}

func TestRunStages_CompleteFlow(t *testing.T) {
	controller := &mockController{
		instanceName: "test-worker-789",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	cfg := config.Config{
		PipelineRaw: pipeline.PipelineRaw{
			Stages: []pipeline.StageRaw{
				{
					Name: "First Stage",
					Type: "exec",
					Steps: []string{
						"echo 'Starting {{.Worker.Name}}'",
						"mkdir -p /opt/recon",
					},
				},
				{
					Name: "Second Stage",
					Type: "exec",
					Steps: []string{
						"nuclei -l {{.Targets.filepath}} -o /opt/recon/{{.Worker.Name}}-nuclei.txt",
					},
				},
			},
		},
	}

	targets := []string{"example.com", "test.com", "demo.org"}

	err := runStages(controller, cfg, targets)
	if err != nil {
		t.Fatalf("Failed to run stages: %v", err)
	}

	// Should have executed all commands from both stages plus directory creation and file writing
	expectedMinCommands := 4 // 2 from first stage + 1 from second stage + 1 for directory creation + 1 for file writing
	if len(controller.commands) < expectedMinCommands {
		t.Errorf("Expected at least %d commands, got %d", expectedMinCommands, len(controller.commands))
	}

	// Check that the first command was executed
	if controller.commands[0] != "sudo mkdir -p /opt/recon" {
		t.Errorf("Expected first command to create directory, got: %s", controller.commands[0])
	}

	// Check that targets file was created
	foundTargetsFile := false
	for _, cmd := range controller.commands {
		if strings.Contains(cmd, "echo") && strings.Contains(cmd, "sudo tee /opt/recon/targets-") && strings.Contains(cmd, "> /dev/null") {
			foundTargetsFile = true
			break
		}
	}
	if !foundTargetsFile {
		t.Error("Expected targets file creation command not found")
	}

	// Check that stage commands were executed
	foundStageCommand := false
	for _, cmd := range controller.commands {
		if cmd == "echo 'Starting test-worker-789'" {
			foundStageCommand = true
			break
		}
	}
	if !foundStageCommand {
		t.Error("Expected stage command not found")
	}
}
