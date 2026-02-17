package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/pipeline"
	"reconswarm/internal/provisioning"
	"reconswarm/internal/server/manager"
	"reconswarm/internal/ssh"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

// mockFile implements io.ReadWriteCloser for testing
type mockFile struct {
	buf    *bytes.Buffer
	closed bool
}

func newMockFile(buf *bytes.Buffer) *mockFile {
	return &mockFile{buf: buf, closed: false}
}

func (m *mockFile) Read(p []byte) (n int, err error) {
	return m.buf.Read(p)
}

func (m *mockFile) Write(p []byte) (n int, err error) {
	return m.buf.Write(p)
}

func (m *mockFile) Close() error {
	m.closed = true
	return nil
}

// OpenFileCall records a call to OpenFile
type OpenFileCall struct {
	Path    string
	Flags   int
	Content *bytes.Buffer // Content written to this file
}

// SyncCall records a call to Sync
type SyncCall struct {
	RemotePath string
	LocalPath  string
}

// MockProvisioner implements provisioning.Provisioner
type MockProvisioner struct{}

func (m *MockProvisioner) Create(ctx context.Context, spec provisioning.InstanceSpec) (*provisioning.InstanceInfo, error) {
	return &provisioning.InstanceInfo{
		ID:     "mock-instance-" + spec.Name,
		IP:     "127.0.0.1",
		Name:   spec.Name,
		Zone:   spec.Zone,
		Status: "RUNNING",
	}, nil
}

func (m *MockProvisioner) Delete(ctx context.Context, instanceID string) error {
	return nil
}

// MockController implements control.Controller with tracking of file operations
type MockController struct {
	InstanceName  string
	OpenFileCalls []OpenFileCall
	SyncCalls     []SyncCall
	Commands      []string
	mu            sync.Mutex
}

func (m *MockController) Close() error {
	return nil
}

func (m *MockController) Run(command string) error {
	m.mu.Lock()
	m.Commands = append(m.Commands, command)
	m.mu.Unlock()

	// Mock successful execution
	// If command is sleep, actually sleep to simulate long running task
	if len(command) > 6 && command[:5] == "sleep" {
		var duration int
		_, _ = fmt.Sscanf(command, "sleep %d", &duration)
		time.Sleep(time.Duration(duration) * time.Second)
	}
	return nil
}

func (m *MockController) OpenFile(path string, flags int) (io.ReadWriteCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	buf := &bytes.Buffer{}
	m.OpenFileCalls = append(m.OpenFileCalls, OpenFileCall{
		Path:    path,
		Flags:   flags,
		Content: buf,
	})
	return newMockFile(buf), nil
}

func (m *MockController) GetInstanceName() string {
	return m.InstanceName
}

func (m *MockController) Sync(remotePath, localPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.SyncCalls = append(m.SyncCalls, SyncCall{
		RemotePath: remotePath,
		LocalPath:  localPath,
	})
	return nil
}

func MockControllerFactory(config control.Config) (control.Controller, error) {
	return &MockController{
		InstanceName:  config.InstanceName,
		OpenFileCalls: []OpenFileCall{},
		SyncCalls:     []SyncCall{},
		Commands:      []string{},
	}, nil
}

// MockKeyProvider implements ssh.KeyProvider
type MockKeyProvider struct {
	keyPair *ssh.KeyPair
}

func NewMockKeyProvider() *MockKeyProvider {
	return &MockKeyProvider{
		keyPair: &ssh.KeyPair{
			PrivateKey: "mock-private-key",
			PublicKey:  "mock-public-key",
		},
	}
}

func (m *MockKeyProvider) GetOrCreate(ctx context.Context) (*ssh.KeyPair, error) {
	return m.keyPair, nil
}

func (m *MockKeyProvider) Save(ctx context.Context, keyPair *ssh.KeyPair) error {
	m.keyPair = keyPair
	return nil
}

func (m *MockKeyProvider) Delete(ctx context.Context) error {
	m.keyPair = nil
	return nil
}

func (m *MockKeyProvider) Close() error {
	return nil
}

// MockStateManager implements manager.StateManager
type MockStateManager struct {
	mu        sync.RWMutex
	pipelines map[string][]byte
}

func NewMockStateManager() *MockStateManager {
	return &MockStateManager{
		pipelines: make(map[string][]byte),
	}
}

func (m *MockStateManager) Close() error {
	return nil
}

func (m *MockStateManager) SavePipeline(ctx context.Context, pipelineID string, state *manager.PipelineState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	m.pipelines[pipelineID] = data
	return nil
}

func (m *MockStateManager) GetPipeline(ctx context.Context, pipelineID string, dest any) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.pipelines[pipelineID]
	if !ok {
		return fmt.Errorf("pipeline %s not found", pipelineID)
	}
	return json.Unmarshal(data, dest)
}

// createTestConfig creates a test configuration using the new structure
func createTestConfig() config.Config {
	return config.Config{
		Server: config.ServerConfig{
			Port: 50051,
		},
		Etcd: config.EtcdConfig{
			Endpoints:   []string{"localhost:2379"},
			DialTimeout: 5,
		},
		Provisioner: config.ProvisionerConfig{
			Type: config.ProviderYandexCloud,
			YandexCloud: &config.YandexCloudConfig{
				IAMToken:        "fake-token",
				FolderID:        "fake-folder",
				DefaultZone:     "ru-central1-a",
				DefaultImage:    "fd80bm0rh4rkepi5ksdi",
				DefaultUsername: "ubuntu",
				DefaultCores:    2,
				DefaultMemory:   4,
				DefaultDiskSize: 20,
			},
		},
		Workers: config.WorkersConfig{
			MaxWorkers: 5,
		},
	}
}

// pipelineWrapper is used to parse YAML files with "pipeline:" root key
type pipelineWrapper struct {
	Pipeline pipeline.Pipeline `yaml:"pipeline"`
}

// parsePipelineYAML parses YAML into a domain Pipeline (supports both wrapped and direct formats)
func parsePipelineYAML(data string) pipeline.Pipeline {
	var wrapper pipelineWrapper
	err := yaml.Unmarshal([]byte(data), &wrapper)
	Expect(err).NotTo(HaveOccurred())
	if len(wrapper.Pipeline.Targets) > 0 || len(wrapper.Pipeline.Stages) > 0 {
		return wrapper.Pipeline
	}
	var p pipeline.Pipeline
	err = yaml.Unmarshal([]byte(data), &p)
	Expect(err).NotTo(HaveOccurred())
	return p
}

var _ = Describe("Pipeline E2E", func() {
	var (
		cfg             config.Config
		stateManager    *MockStateManager
		workerManager   *manager.WorkerManager
		pipelineManager *manager.PipelineManager
		ctx             context.Context
		cancel          context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Setup configuration for testing using new structure
		cfg = createTestConfig()

		// Use MockStateManager
		stateManager = NewMockStateManager()

		// Initialize Managers with Mocks
		mockProv := &MockProvisioner{}
		mockKeyProvider := NewMockKeyProvider()

		var err error
		workerManager, err = manager.NewWorkerManager(cfg, stateManager, mockProv, MockControllerFactory, mockKeyProvider)
		Expect(err).NotTo(HaveOccurred())

		pipelineManager = manager.NewPipelineManager(workerManager, stateManager)
	})

	AfterEach(func() {
		cancel()
		if stateManager != nil {
			stateManager.Close()
		}
	})

	Context("Single Pipeline Execution", func() {
		It("should successfully run a pipeline", func() {
			pipelineYAML := `
name: test-pipeline
targets:
  - type: list
    value:
      - localhost
stages:
  - name: stage1
    type: exec
    steps:
      - echo "hello"
`
			id, err := pipelineManager.SubmitPipeline(ctx, parsePipelineYAML(pipelineYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			// Wait for completion
			Eventually(func() manager.PipelineStatus {
				status, err := pipelineManager.GetStatus(ctx, id)
				if err != nil {
					return manager.PipelineStatusFailed
				}
				return status.Status
			}, 30*time.Second, 1*time.Second).Should(Equal(manager.PipelineStatusCompleted))

			// Verify workers are released/deallocated
			workers := workerManager.GetStatus()
			for _, w := range workers {
				Expect(w.CurrentTask).NotTo(Equal(id))
			}
		})
	})

	Context("Concurrent Pipelines", func() {
		It("should run two pipelines simultaneously", func() {
			pipeline1 := `
name: p1
targets:
  - type: list
    value:
      - localhost
stages:
  - name: s1
    type: exec
    steps:
      - sleep 2
`
			pipeline2 := `
name: p2
targets:
  - type: list
    value:
      - localhost
stages:
  - name: s1
    type: exec
    steps:
      - sleep 2
`
			id1, err := pipelineManager.SubmitPipeline(ctx, parsePipelineYAML(pipeline1))
			Expect(err).NotTo(HaveOccurred())

			id2, err := pipelineManager.SubmitPipeline(ctx, parsePipelineYAML(pipeline2))
			Expect(err).NotTo(HaveOccurred())

			// Both should be running or completed eventually
			Eventually(func() bool {
				s1, _ := pipelineManager.GetStatus(ctx, id1)
				s2, _ := pipelineManager.GetStatus(ctx, id2)
				return (s1.Status == manager.PipelineStatusRunning || s1.Status == manager.PipelineStatusCompleted) &&
					(s2.Status == manager.PipelineStatusRunning || s2.Status == manager.PipelineStatusCompleted)
			}, 10*time.Second, 1*time.Second).Should(BeTrue())

			// Wait for both to complete
			Eventually(func() manager.PipelineStatus {
				s, _ := pipelineManager.GetStatus(ctx, id1)
				return s.Status
			}, 60*time.Second, 1*time.Second).Should(Equal(manager.PipelineStatusCompleted))

			Eventually(func() manager.PipelineStatus {
				s, _ := pipelineManager.GetStatus(ctx, id2)
				return s.Status
			}, 60*time.Second, 1*time.Second).Should(Equal(manager.PipelineStatusCompleted))
		})
	})

	Context("Graceful Shutdown", func() {
		It("should recover state after restart", func() {
			pipelineYAML := `
name: recovery-test
targets:
  - type: list
    value:
      - localhost
stages:
  - name: s1
    type: exec
    steps:
      - sleep 5
`
			id, err := pipelineManager.SubmitPipeline(ctx, parsePipelineYAML(pipelineYAML))
			Expect(err).NotTo(HaveOccurred())

			// Wait for it to be running
			Eventually(func() manager.PipelineStatus {
				s, _ := pipelineManager.GetStatus(ctx, id)
				return s.Status
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(manager.PipelineStatusRunning))

			// Simulate "Restart" by creating new managers connected to same StateManager
			newStateManager := stateManager

			mockProv := &MockProvisioner{}
			mockKeyProvider := NewMockKeyProvider()
			newWorkerManager, err := manager.NewWorkerManager(cfg, newStateManager, mockProv, MockControllerFactory, mockKeyProvider)
			Expect(err).NotTo(HaveOccurred())

			newPipelineManager := manager.NewPipelineManager(newWorkerManager, newStateManager)

			// Check status from new manager
			status, err := newPipelineManager.GetStatus(ctx, id)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.Status).To(Or(Equal(manager.PipelineStatusRunning), Equal(manager.PipelineStatusCompleted)))

			newStateManager.Close()
		})
	})

	Context("Sync Stage", func() {
		It("should execute sync stage with correct paths", func() {
			pipelineYAML := `
name: sync-test
targets:
  - type: list
    value:
      - target1.com
      - target2.com
stages:
  - name: run-scan
    type: exec
    steps:
      - nuclei -l {{.Targets.filepath}} -o /opt/recon/{{.Worker.Name}}-results.json
  - name: sync-results
    type: sync
    src: /opt/recon/{{.Worker.Name}}-results.json
    dest: ./results/{{.Worker.Name}}-results.json
`
			id, err := pipelineManager.SubmitPipeline(ctx, parsePipelineYAML(pipelineYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			// Wait for completion
			Eventually(func() manager.PipelineStatus {
				status, err := pipelineManager.GetStatus(ctx, id)
				if err != nil {
					return manager.PipelineStatusFailed
				}
				return status.Status
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(manager.PipelineStatusCompleted))
		})

		It("should execute sync stage with targets filepath template", func() {
			pipelineYAML := `
name: sync-targets-test
targets:
  - type: list
    value:
      - example.com
stages:
  - name: backup-targets
    type: sync
    src: "{{.Targets.filepath}}"
    dest: ./backup/targets-{{.Worker.Name}}.txt
`
			id, err := pipelineManager.SubmitPipeline(ctx, parsePipelineYAML(pipelineYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			// Wait for completion
			Eventually(func() manager.PipelineStatus {
				status, err := pipelineManager.GetStatus(ctx, id)
				if err != nil {
					return manager.PipelineStatusFailed
				}
				return status.Status
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(manager.PipelineStatusCompleted))
		})
	})

	Context("File Operations via OpenFile", func() {
		It("should write targets file via OpenFile interface", func() {
			// Create a tracking controller factory that collects ALL controllers
			var capturedControllers []*MockController
			var controllersMu sync.Mutex
			trackingFactory := func(config control.Config) (control.Controller, error) {
				ctrl := &MockController{
					InstanceName:  config.InstanceName,
					OpenFileCalls: []OpenFileCall{},
					SyncCalls:     []SyncCall{},
					Commands:      []string{},
				}
				controllersMu.Lock()
				capturedControllers = append(capturedControllers, ctrl)
				controllersMu.Unlock()
				return ctrl, nil
			}

			// Create new managers with tracking factory
			mockProv := &MockProvisioner{}
			mockKeyProvider := NewMockKeyProvider()
			trackingWorkerManager, err := manager.NewWorkerManager(cfg, stateManager, mockProv, trackingFactory, mockKeyProvider)
			Expect(err).NotTo(HaveOccurred())

			trackingPipelineManager := manager.NewPipelineManager(trackingWorkerManager, stateManager)

			// Use single target to ensure only one worker is created
			pipelineYAML := `
name: openfile-test
targets:
  - type: list
    value:
      - target.example.com
stages:
  - name: test-stage
    type: exec
    steps:
      - echo "test"
`
			id, err := trackingPipelineManager.SubmitPipeline(ctx, parsePipelineYAML(pipelineYAML))
			Expect(err).NotTo(HaveOccurred())

			// Wait for completion
			Eventually(func() manager.PipelineStatus {
				status, _ := trackingPipelineManager.GetStatus(ctx, id)
				return status.Status
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(manager.PipelineStatusCompleted))

			// Give a moment for all operations to complete
			time.Sleep(100 * time.Millisecond)

			// Verify at least one controller was created
			Expect(len(capturedControllers)).To(BeNumerically(">=", 1))

			// Check that OpenFile was called with correct flags for writing in any controller
			foundTargetsFile := false
			for _, ctrl := range capturedControllers {
				for _, call := range ctrl.OpenFileCalls {
					if strings.HasPrefix(call.Path, "/opt/recon/targets-") {
						foundTargetsFile = true
						// Verify flags: O_CREATE | O_WRONLY | O_TRUNC
						expectedFlags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
						Expect(call.Flags).To(Equal(expectedFlags))

						// Verify content was written
						content := call.Content.String()
						Expect(content).To(ContainSubstring("target.example.com"))
					}
				}
			}
			Expect(foundTargetsFile).To(BeTrue(), "Expected targets file to be written via OpenFile")
		})

		It("should handle special characters in targets via OpenFile (not shell)", func() {
			var capturedControllers []*MockController
			var controllersMu sync.Mutex
			trackingFactory := func(config control.Config) (control.Controller, error) {
				ctrl := &MockController{
					InstanceName:  config.InstanceName,
					OpenFileCalls: []OpenFileCall{},
					SyncCalls:     []SyncCall{},
					Commands:      []string{},
				}
				controllersMu.Lock()
				capturedControllers = append(capturedControllers, ctrl)
				controllersMu.Unlock()
				return ctrl, nil
			}

			mockProv := &MockProvisioner{}
			mockKeyProvider := NewMockKeyProvider()
			trackingWorkerManager, err := manager.NewWorkerManager(cfg, stateManager, mockProv, trackingFactory, mockKeyProvider)
			Expect(err).NotTo(HaveOccurred())

			trackingPipelineManager := manager.NewPipelineManager(trackingWorkerManager, stateManager)

			// Single target with special characters that would break shell commands
			// Using single target to ensure one worker handles all special chars
			pipelineYAML := `
name: special-chars-test
targets:
  - type: list
    value:
      - "domain's-test$var.com"
stages:
  - name: test-stage
    type: exec
    steps:
      - echo "done"
`
			id, err := trackingPipelineManager.SubmitPipeline(ctx, parsePipelineYAML(pipelineYAML))
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() manager.PipelineStatus {
				status, _ := trackingPipelineManager.GetStatus(ctx, id)
				return status.Status
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(manager.PipelineStatusCompleted))

			time.Sleep(100 * time.Millisecond)

			// Verify special characters are preserved (not interpreted by shell)
			Expect(len(capturedControllers)).To(BeNumerically(">=", 1))
			foundSpecialChars := false
			for _, ctrl := range capturedControllers {
				for _, call := range ctrl.OpenFileCalls {
					if strings.HasPrefix(call.Path, "/opt/recon/targets-") {
						content := call.Content.String()
						// These characters should be preserved exactly as-is
						// Single quotes and $ signs would be interpreted by shell
						if strings.Contains(content, "domain's-test$var.com") {
							foundSpecialChars = true
						}
					}
				}
			}
			Expect(foundSpecialChars).To(BeTrue(), "Special characters should be preserved via OpenFile")
		})
	})

	Context("Full Pipeline with Exec and Sync", func() {
		It("should execute complete pipeline with exec and sync stages", func() {
			var capturedControllers []*MockController
			var controllersMu sync.Mutex
			trackingFactory := func(config control.Config) (control.Controller, error) {
				ctrl := &MockController{
					InstanceName:  config.InstanceName,
					OpenFileCalls: []OpenFileCall{},
					SyncCalls:     []SyncCall{},
					Commands:      []string{},
				}
				controllersMu.Lock()
				capturedControllers = append(capturedControllers, ctrl)
				controllersMu.Unlock()
				return ctrl, nil
			}

			mockProv := &MockProvisioner{}
			mockKeyProvider := NewMockKeyProvider()
			trackingWorkerManager, err := manager.NewWorkerManager(cfg, stateManager, mockProv, trackingFactory, mockKeyProvider)
			Expect(err).NotTo(HaveOccurred())

			trackingPipelineManager := manager.NewPipelineManager(trackingWorkerManager, stateManager)

			// Single target to ensure predictable behavior
			pipelineYAML := `
name: full-pipeline-test
targets:
  - type: list
    value:
      - target.example.com
stages:
  - name: prepare
    type: exec
    steps:
      - mkdir -p /opt/recon/output
  - name: scan
    type: exec
    steps:
      - nuclei -l {{.Targets.filepath}} -o /opt/recon/output/{{.Worker.Name}}-nuclei.json
      - httpx -l {{.Targets.filepath}} -o /opt/recon/output/{{.Worker.Name}}-httpx.json
  - name: sync-nuclei
    type: sync
    src: /opt/recon/output/{{.Worker.Name}}-nuclei.json
    dest: ./results/{{.Worker.Name}}-nuclei.json
  - name: sync-httpx
    type: sync
    src: /opt/recon/output/{{.Worker.Name}}-httpx.json
    dest: ./results/{{.Worker.Name}}-httpx.json
`
			id, err := trackingPipelineManager.SubmitPipeline(ctx, parsePipelineYAML(pipelineYAML))
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() manager.PipelineStatus {
				status, _ := trackingPipelineManager.GetStatus(ctx, id)
				return status.Status
			}, 30*time.Second, 500*time.Millisecond).Should(Equal(manager.PipelineStatusCompleted))

			time.Sleep(100 * time.Millisecond)

			Expect(len(capturedControllers)).To(BeNumerically(">=", 1))

			// Aggregate stats from all controllers
			var totalOpenFileCalls, totalCommands, totalSyncCalls int
			for _, ctrl := range capturedControllers {
				totalOpenFileCalls += len(ctrl.OpenFileCalls)
				totalCommands += len(ctrl.Commands)
				totalSyncCalls += len(ctrl.SyncCalls)

				// Verify sync paths contain expected patterns
				for _, syncCall := range ctrl.SyncCalls {
					Expect(syncCall.RemotePath).To(ContainSubstring("/opt/recon/output/"))
					Expect(syncCall.LocalPath).To(ContainSubstring("./results/"))
				}
			}

			// Verify targets file was written via OpenFile
			Expect(totalOpenFileCalls).To(BeNumerically(">=", 1))

			// Verify exec commands were run (mkdir creates directory + 2 scan commands per worker)
			// Note: sudo mkdir command from executor + stage commands
			Expect(totalCommands).To(BeNumerically(">=", 3))

			// Verify sync was called twice per worker (nuclei + httpx results)
			Expect(totalSyncCalls).To(Equal(2))
		})
	})
})
