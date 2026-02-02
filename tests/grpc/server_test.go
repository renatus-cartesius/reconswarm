package grpc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"reconswarm/api"
	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/provisioning"
	"reconswarm/internal/server"
	"reconswarm/internal/server/manager"
	"reconswarm/internal/ssh"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
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
	return nil
}

func (m *MockController) OpenFile(path string, flags int) (io.ReadWriteCloser, error) {
	return nil, nil
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
	mu        sync.RWMutex
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

func (m *MockStateManager) DeletePipeline(ctx context.Context, pipelineID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pipelines, pipelineID)
	return nil
}

func (m *MockStateManager) ListPipelines(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var ids []string
	for id := range m.pipelines {
		ids = append(ids, id)
	}
	return ids, nil
}

func (m *MockStateManager) SaveWorker(ctx context.Context, workerID string, state any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	m.workers[workerID] = data
	return nil
}

func (m *MockStateManager) GetWorker(ctx context.Context, workerID string, dest any) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}
	return json.Unmarshal(data, dest)
}

func (m *MockStateManager) DeleteWorker(ctx context.Context, workerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.workers, workerID)
	return nil
}

func (m *MockStateManager) ListWorkers(ctx context.Context) (map[string][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
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
				DefaultZone:     "test-zone",
				DefaultImage:    "test-image",
				DefaultUsername: "test-user",
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

const bufSize = 1024 * 1024

var _ = Describe("gRPC Server", func() {
	var (
		lis      *bufconn.Listener
		srv      *grpc.Server
		conn     *grpc.ClientConn
		client   api.ReconSwarmClient
		ctx      context.Context
		cancel   context.CancelFunc
		mockSM   *MockStateManager
		mockProv *MockProvisioner
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		lis = bufconn.Listen(bufSize)
		srv = grpc.NewServer()

		// Setup mocks
		mockSM = NewMockStateManager()
		mockProv = &MockProvisioner{}
		cfg := createTestConfig()

		// Create managers
		mockKeyProvider := NewMockKeyProvider()
		wm, err := manager.NewWorkerManager(cfg, mockSM, mockProv, MockControllerFactory, mockKeyProvider)
		Expect(err).NotTo(HaveOccurred())
		pm := manager.NewPipelineManager(wm, mockSM)

		// Create server with injected dependencies
		reconServer := server.NewServerWithDependencies(pm, wm, mockSM)
		api.RegisterReconSwarmServer(srv, reconServer)

		go func() {
			if err := srv.Serve(lis); err != nil {
				_ = err
			}
		}()

		// Create client
		conn, err = grpc.NewClient("passthrough://bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}), grpc.WithTransportCredentials(insecure.NewCredentials()))
		Expect(err).NotTo(HaveOccurred())

		client = api.NewReconSwarmClient(conn)
	})

	AfterEach(func() {
		cancel()
		conn.Close()
		srv.Stop()
		lis.Close()
	})

	Context("RunPipeline", func() {
		It("should successfully submit a pipeline", func() {
			req := &api.RunPipelineRequest{
				PipelineYaml: `
name: test-pipeline
targets:
  - type: list
    value:
      - localhost
stages:
  - name: stage1
    type: exec
    steps:
      - echo hello
`,
			}
			resp, err := client.RunPipeline(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.PipelineId).NotTo(BeEmpty())
		})

		It("should fail with invalid YAML", func() {
			req := &api.RunPipelineRequest{
				PipelineYaml: `invalid: yaml: content: [`,
			}
			_, err := client.RunPipeline(ctx, req)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("GetPipeline", func() {
		It("should return status for existing pipeline", func() {
			// First submit a pipeline
			req := &api.RunPipelineRequest{
				PipelineYaml: `
name: test-pipeline
targets:
  - type: list
    value:
      - localhost
stages:
  - name: stage1
    type: exec
    steps:
      - echo hello
`,
			}
			resp, err := client.RunPipeline(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			id := resp.PipelineId

			// Get status
			statusResp, err := client.GetPipeline(ctx, &api.GetPipelineRequest{PipelineId: id})
			Expect(err).NotTo(HaveOccurred())
			Expect(statusResp.PipelineId).To(Equal(id))
			Expect(statusResp.Status).To(Or(Equal("Pending"), Equal("Running"), Equal("Completed")))
		})

		It("should return error for non-existent pipeline", func() {
			_, err := client.GetPipeline(ctx, &api.GetPipelineRequest{PipelineId: "non-existent"})
			Expect(err).To(HaveOccurred())
		})
	})
})
