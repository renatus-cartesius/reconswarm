package pipeline

import (
	"bytes"
	"context"
	"io"
	"testing"
)

// mockFile implements io.ReadWriteCloser for testing
type mockFile struct {
	bytes.Buffer
}

func (m *mockFile) Close() error {
	return nil
}

// mockController is a mock implementation of the Controller interface for testing
type mockController struct {
	instanceName string
	commands     []string
	syncedFiles  []syncedFile
	writtenFiles map[string]*bytes.Buffer
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

func (m *mockController) OpenFile(path string, flags int) (io.ReadWriteCloser, error) {
	if m.writtenFiles == nil {
		m.writtenFiles = make(map[string]*bytes.Buffer)
	}
	buf := &bytes.Buffer{}
	m.writtenFiles[path] = buf
	return &mockFile{Buffer: *buf}, nil
}

func (m *mockController) GetInstanceName() string {
	return m.instanceName
}

func (m *mockController) Sync(remotePath, localPath string) error {
	m.syncedFiles = append(m.syncedFiles, syncedFile{
		remotePath: remotePath,
		localPath:  localPath,
	})
	return nil
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

	result, err := RenderTemplate(templateStr, context)
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

	result, err := RenderTemplate(templateStr, context)
	if err != nil {
		t.Fatalf("Failed to render template: %v", err)
	}

	expected := "docker run --name test-worker test-image"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestExecuteExecStage_BasicTemplate(t *testing.T) {
	controller := &mockController{
		instanceName: "test-instance-123",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	stage := &ExecStage{
		Name: "Test Stage",
		Type: "exec",
		Steps: []string{
			"echo 'Hello {{.Worker.Name}}'",
			"docker run --name {{.Worker.Name}} test-image",
		},
	}

	targets := []string{"example.com", "test.com"}
	targetsFile := "/opt/recon/targets-20231201-120000.txt"

	err := stage.Execute(context.Background(), controller, targets, targetsFile)
	if err != nil {
		t.Fatalf("Failed to execute exec stage: %v", err)
	}

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

	stage := &ExecStage{
		Name: "Targets Test Stage",
		Type: "exec",
		Steps: []string{
			"nuclei -l {{.Targets.filepath}} -o /opt/recon/{{.Worker.Name}}.txt",
			"echo 'Processed {{len .Targets.list}} targets'",
		},
	}

	targets := []string{"example.com", "test.com", "demo.org"}
	targetsFile := "/opt/recon/targets-20231201-120000.txt"

	err := stage.Execute(context.Background(), controller, targets, targetsFile)
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

	stage := &ExecStage{
		Name: "Complex Test Stage",
		Type: "exec",
		Steps: []string{
			"docker run --rm -v /opt/recon:/data -v {{.Targets.filepath}}:/data/hosts.txt projectdiscovery/nuclei:latest -l /data/hosts.txt -o /data/{{.Worker.Name}}.txt",
			"docker run --rm -v /opt/recon:/data -v {{.Targets.filepath}}:/data/hosts.txt projectdiscovery/subfinder:latest -dL /data/hosts.txt -o /data/{{.Worker.Name}}-subfinder.txt",
		},
	}

	targets := []string{"example.com", "test.com"}
	targetsFile := "/opt/recon/targets-20231201-120000.txt"

	err := stage.Execute(context.Background(), controller, targets, targetsFile)
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

	stage := &ExecStage{
		Name:  "Empty Stage",
		Type:  "exec",
		Steps: []string{},
	}

	targets := []string{"example.com"}
	targetsFile := "/opt/recon/targets.txt"

	err := stage.Execute(context.Background(), controller, targets, targetsFile)
	if err != nil {
		t.Fatalf("Failed to execute exec stage: %v", err)
	}

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

	stage := &ExecStage{
		Name: "Invalid Template Stage",
		Type: "exec",
		Steps: []string{
			"echo 'Hello {{.Invalid.Field'",
		},
	}

	targets := []string{"example.com"}
	targetsFile := "/opt/recon/targets.txt"

	err := stage.Execute(context.Background(), controller, targets, targetsFile)
	if err == nil {
		t.Error("Expected error for invalid template, but got none")
	}

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
			result, err := RenderTemplate(tt.template, tt.context)

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

	stage := &SyncStage{
		Name: "Basic Sync Stage",
		Type: "sync",
		Src:  "/opt/recon/results-{{.Worker.Name}}.json",
		Dest: "./results/results-{{.Worker.Name}}.json",
	}

	targets := []string{"example.com", "test.com"}
	targetsFile := "/opt/recon/targets-1234567890.txt"

	err := stage.Execute(context.Background(), controller, targets, targetsFile)
	if err != nil {
		t.Fatalf("Failed to execute sync stage: %v", err)
	}

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

	stage := &SyncStage{
		Name: "Targets Sync Stage",
		Type: "sync",
		Src:  "{{.Targets.filepath}}",
		Dest: "./backup/targets-{{.Worker.Name}}.txt",
	}

	targets := []string{"example.com", "test.com", "demo.org"}
	targetsFile := "/opt/recon/targets-1234567890.txt"

	err := stage.Execute(context.Background(), controller, targets, targetsFile)
	if err != nil {
		t.Fatalf("Failed to execute sync stage: %v", err)
	}

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

	stage := &SyncStage{
		Name: "Invalid Template Sync Stage",
		Type: "sync",
		Src:  "/opt/recon/{{.Invalid.Field'",
		Dest: "./results/test.json",
	}

	targets := []string{"example.com"}
	targetsFile := "/opt/recon/targets.txt"

	err := stage.Execute(context.Background(), controller, targets, targetsFile)
	if err == nil {
		t.Error("Expected error for invalid template, but got none")
	}

	if len(controller.syncedFiles) != 0 {
		t.Errorf("Expected 0 synced files due to template error, got %d", len(controller.syncedFiles))
	}
}

func TestExecuteOnWorker_CompleteFlow(t *testing.T) {
	controller := &mockController{
		instanceName: "test-worker-789",
		commands:     []string{},
		syncedFiles:  []syncedFile{},
	}

	pipelineRaw := PipelineRaw{
		Stages: []StageRaw{
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
	}

	targets := []string{"example.com", "test.com", "demo.org"}

	err := ExecuteOnWorker(context.Background(), controller, pipelineRaw.ToPipeline(), targets)
	if err != nil {
		t.Fatalf("Failed to run stages: %v", err)
	}

	// Check first command creates directory with proper permissions
	expectedFirstCmd := "sudo mkdir -p /opt/recon && sudo chmod 777 /opt/recon"
	if controller.commands[0] != expectedFirstCmd {
		t.Errorf("Expected first command '%s', got: %s", expectedFirstCmd, controller.commands[0])
	}

	// Check targets file was written via OpenFile (not via shell command)
	if len(controller.writtenFiles) == 0 {
		t.Error("Expected targets file to be written via OpenFile")
	}

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
