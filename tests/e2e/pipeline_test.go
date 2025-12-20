package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/provisioning"
	"reconswarm/internal/server/manager"
	"reconswarm/internal/ssh"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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

// MockController implements control.Controller
type MockController struct {
	InstanceName string
}

func (m *MockController) Close() error {
	return nil
}

func (m *MockController) Run(command string) error {
	// Mock successful execution
	// If command is sleep, actually sleep to simulate long running task
	if len(command) > 6 && command[:5] == "sleep" {
		var duration int
		_, _ = fmt.Sscanf(command, "sleep %d", &duration)
		time.Sleep(time.Duration(duration) * time.Second)
	}
	return nil
}

func (m *MockController) ReadFile(remotePath string) (string, error) {
	return "", nil
}

func (m *MockController) WriteFile(remotePath, content string, mode os.FileMode) error {
	return nil
}

func (m *MockController) GetInstanceName() string {
	return m.InstanceName
}

func (m *MockController) Sync(remotePath, localPath string) error {
	return nil
}

func MockControllerFactory(config control.Config) (control.Controller, error) {
	return &MockController{InstanceName: config.InstanceName}, nil
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
	pipelines map[string][]byte
	workers   map[string][]byte
}

func NewMockStateManager() *MockStateManager {
	return &MockStateManager{
		pipelines: make(map[string][]byte),
		workers:   make(map[string][]byte),
	}
}

func (m *MockStateManager) Close() error {
	return nil
}

func (m *MockStateManager) SavePipeline(ctx context.Context, pipelineID string, state any) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	m.pipelines[pipelineID] = data
	return nil
}

func (m *MockStateManager) GetPipeline(ctx context.Context, pipelineID string, dest any) error {
	data, ok := m.pipelines[pipelineID]
	if !ok {
		return fmt.Errorf("pipeline %s not found", pipelineID)
	}
	return json.Unmarshal(data, dest)
}

func (m *MockStateManager) DeletePipeline(ctx context.Context, pipelineID string) error {
	delete(m.pipelines, pipelineID)
	return nil
}

func (m *MockStateManager) ListPipelines(ctx context.Context) ([]string, error) {
	var ids []string
	for id := range m.pipelines {
		ids = append(ids, id)
	}
	return ids, nil
}

func (m *MockStateManager) SaveWorker(ctx context.Context, workerID string, state any) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	m.workers[workerID] = data
	return nil
}

func (m *MockStateManager) GetWorker(ctx context.Context, workerID string, dest any) error {
	data, ok := m.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}
	return json.Unmarshal(data, dest)
}

func (m *MockStateManager) DeleteWorker(ctx context.Context, workerID string) error {
	delete(m.workers, workerID)
	return nil
}

func (m *MockStateManager) ListWorkers(ctx context.Context) (map[string][]byte, error) {
	// Return a copy to avoid race conditions if caller modifies it
	workers := make(map[string][]byte)
	for k, v := range m.workers {
		workers[k] = v
	}
	return workers, nil
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
			id, err := pipelineManager.SubmitPipeline(ctx, pipelineYAML)
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
			id1, err := pipelineManager.SubmitPipeline(ctx, pipeline1)
			Expect(err).NotTo(HaveOccurred())

			id2, err := pipelineManager.SubmitPipeline(ctx, pipeline2)
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
			id, err := pipelineManager.SubmitPipeline(ctx, pipelineYAML)
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
})
