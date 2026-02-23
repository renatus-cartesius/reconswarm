package e2e_test

import (
	"context"
	"encoding/json"
	"reconswarm/internal/pipeline"
	"reconswarm/internal/server/manager"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// GetRawPipeline returns raw JSON bytes for a pipeline (for test inspection)
func (m *MockStateManager) GetRawPipeline(pipelineID string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, ok := m.pipelines[pipelineID]
	return data, ok
}

var _ = Describe("Pipeline State Persistence", func() {
	var (
		stateManager *MockStateManager
		ctx          context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		stateManager = NewMockStateManager()
	})

	AfterEach(func() {
		stateManager.Close()
	})

	Context("Saving pipeline state to state store", func() {
		It("should save complete pipeline state with all fields", func() {
			now := time.Now().Truncate(time.Second)
			state := &manager.PipelineState{
				ID:              "pipe-test-123",
				Status:          manager.PipelineStatusRunning,
				Error:           "",
				TotalStages:     3,
				CompletedStages: 1,
				StartTime:       now,
				EndTime:         time.Time{},
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{
						{Type: "list", Value: []interface{}{"example.com", "test.com"}},
					},
					Stages: []pipeline.Stage{
						&pipeline.ExecStage{
							Name:  "scan",
							Type:  "exec",
							Steps: []string{"nuclei -l {{.Targets.filepath}}", "httpx -l {{.Targets.filepath}}"},
						},
						&pipeline.SyncStage{
							Name: "sync-results",
							Type: "sync",
							Src:  "/opt/recon/{{.Worker.Name}}-results.json",
							Dest: "./results/{{.Worker.Name}}-results.json",
						},
					},
				},
				Workers: []*manager.Worker{
					{
						ID:          "worker-1",
						Name:        "reconswarm-abc",
						IP:          "10.0.0.1",
						Status:      manager.WorkerStatusBusy,
						CurrentTask: "pipe-test-123",
						InstanceID:  "instance-abc",
					},
					{
						ID:          "worker-2",
						Name:        "reconswarm-def",
						IP:          "10.0.0.2",
						Status:      manager.WorkerStatusBusy,
						CurrentTask: "pipe-test-123",
						InstanceID:  "instance-def",
					},
				},
			}

			err := stateManager.SavePipeline(ctx, "pipe-test-123", state)
			Expect(err).NotTo(HaveOccurred())

			rawData, exists := stateManager.GetRawPipeline("pipe-test-123")
			Expect(exists).To(BeTrue())
			Expect(rawData).NotTo(BeEmpty())

			var rawMap map[string]interface{}
			err = json.Unmarshal(rawData, &rawMap)
			Expect(err).NotTo(HaveOccurred())

			// Verify top-level scalar fields
			Expect(rawMap["ID"]).To(Equal("pipe-test-123"))
			Expect(rawMap["Status"]).To(Equal("Running"))
			Expect(rawMap["TotalStages"]).To(BeNumerically("==", 3))
			Expect(rawMap["CompletedStages"]).To(BeNumerically("==", 1))

			// Verify Pipeline is present and contains stages
			Expect(rawMap).To(HaveKey("Pipeline"))
			pipelineMap, ok := rawMap["Pipeline"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(pipelineMap).To(HaveKey("stages"))
			stages, ok := pipelineMap["stages"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(stages).To(HaveLen(2))

			// Verify first stage (exec) in raw JSON
			stage0, ok := stages[0].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(stage0["type"]).To(Equal("exec"))
			Expect(stage0["name"]).To(Equal("scan"))
			stepsRaw, ok := stage0["steps"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(stepsRaw).To(HaveLen(2))

			// Verify second stage (sync) in raw JSON
			stage1, ok := stages[1].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(stage1["type"]).To(Equal("sync"))
			Expect(stage1["name"]).To(Equal("sync-results"))
			Expect(stage1["src"]).To(Equal("/opt/recon/{{.Worker.Name}}-results.json"))
			Expect(stage1["dest"]).To(Equal("./results/{{.Worker.Name}}-results.json"))

			// Verify Workers are present
			workers, ok := rawMap["Workers"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(workers).To(HaveLen(2))

			// Verify Controller and Provisioner are NOT in serialized workers (json:"-")
			worker1, ok := workers[0].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(worker1).NotTo(HaveKey("Controller"))
			Expect(worker1).NotTo(HaveKey("Provisioner"))
			Expect(worker1["Name"]).To(Equal("reconswarm-abc"))
			Expect(worker1["IP"]).To(Equal("10.0.0.1"))
			Expect(worker1["Status"]).To(Equal("Busy"))
			Expect(worker1["CurrentTask"]).To(Equal("pipe-test-123"))
			Expect(worker1["InstanceID"]).To(Equal("instance-abc"))

			worker2, ok := workers[1].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(worker2["Name"]).To(Equal("reconswarm-def"))
			Expect(worker2["IP"]).To(Equal("10.0.0.2"))
		})

		It("should save pipeline state with empty workers list", func() {
			state := &manager.PipelineState{
				ID:     "pipe-empty-workers",
				Status: manager.PipelineStatusPending,
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{
						{Type: "list", Value: []interface{}{"example.com"}},
					},
					Stages: []pipeline.Stage{
						&pipeline.ExecStage{Name: "test", Type: "exec", Steps: []string{"echo hello"}},
					},
				},
				Workers: []*manager.Worker{},
			}

			err := stateManager.SavePipeline(ctx, "pipe-empty-workers", state)
			Expect(err).NotTo(HaveOccurred())

			rawData, exists := stateManager.GetRawPipeline("pipe-empty-workers")
			Expect(exists).To(BeTrue())

			var rawMap map[string]interface{}
			err = json.Unmarshal(rawData, &rawMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(rawMap["Status"]).To(Equal("Pending"))

			workers, ok := rawMap["Workers"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(workers).To(BeEmpty())
		})

		It("should save pipeline state with error message", func() {
			state := &manager.PipelineState{
				ID:     "pipe-failed",
				Status: manager.PipelineStatusFailed,
				Error:  "worker creation failed: no resources available",
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{
						{Type: "list", Value: []interface{}{"example.com"}},
					},
					Stages: []pipeline.Stage{
						&pipeline.ExecStage{Name: "test", Type: "exec", Steps: []string{"echo hello"}},
					},
				},
			}

			err := stateManager.SavePipeline(ctx, "pipe-failed", state)
			Expect(err).NotTo(HaveOccurred())

			rawData, _ := stateManager.GetRawPipeline("pipe-failed")
			var rawMap map[string]interface{}
			err = json.Unmarshal(rawData, &rawMap)
			Expect(err).NotTo(HaveOccurred())

			Expect(rawMap["Error"]).To(Equal("worker creation failed: no resources available"))
			Expect(rawMap["Status"]).To(Equal("Failed"))
		})

		It("should save pipeline targets with different types in raw JSON", func() {
			state := &manager.PipelineState{
				ID:     "pipe-targets",
				Status: manager.PipelineStatusRunning,
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{
						{Type: "list", Value: []interface{}{"a.com", "b.com"}},
						{Type: "crtsh", Value: "domain.com"},
						{Type: "external_list", Value: "https://example.com/targets.txt"},
					},
					Stages: []pipeline.Stage{
						&pipeline.ExecStage{Name: "s1", Type: "exec", Steps: []string{"echo ok"}},
					},
				},
			}

			err := stateManager.SavePipeline(ctx, "pipe-targets", state)
			Expect(err).NotTo(HaveOccurred())

			rawData, _ := stateManager.GetRawPipeline("pipe-targets")
			var rawMap map[string]interface{}
			err = json.Unmarshal(rawData, &rawMap)
			Expect(err).NotTo(HaveOccurred())

			pipelineMap := rawMap["Pipeline"].(map[string]interface{})
			targets := pipelineMap["targets"].([]interface{})
			Expect(targets).To(HaveLen(3))

			t0 := targets[0].(map[string]interface{})
			Expect(t0["type"]).To(Equal("list"))
			t0Values := t0["value"].([]interface{})
			Expect(t0Values).To(ConsistOf("a.com", "b.com"))

			t1 := targets[1].(map[string]interface{})
			Expect(t1["type"]).To(Equal("crtsh"))
			Expect(t1["value"]).To(Equal("domain.com"))

			t2 := targets[2].(map[string]interface{})
			Expect(t2["type"]).To(Equal("external_list"))
			Expect(t2["value"]).To(Equal("https://example.com/targets.txt"))
		})

		It("should overwrite previously saved state for same pipeline ID", func() {
			state1 := &manager.PipelineState{
				ID:     "pipe-overwrite",
				Status: manager.PipelineStatusRunning,
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{{Type: "list", Value: []interface{}{"a.com"}}},
					Stages:  []pipeline.Stage{&pipeline.ExecStage{Name: "s1", Type: "exec", Steps: []string{"echo 1"}}},
				},
			}

			err := stateManager.SavePipeline(ctx, "pipe-overwrite", state1)
			Expect(err).NotTo(HaveOccurred())

			state2 := &manager.PipelineState{
				ID:     "pipe-overwrite",
				Status: manager.PipelineStatusCompleted,
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{{Type: "list", Value: []interface{}{"a.com"}}},
					Stages:  []pipeline.Stage{&pipeline.ExecStage{Name: "s1", Type: "exec", Steps: []string{"echo 1"}}},
				},
				Workers: []*manager.Worker{
					{ID: "w1", Name: "worker-1", IP: "10.0.0.1", Status: manager.WorkerStatusIdle, InstanceID: "i-1"},
				},
			}

			err = stateManager.SavePipeline(ctx, "pipe-overwrite", state2)
			Expect(err).NotTo(HaveOccurred())

			var restored manager.PipelineState
			err = stateManager.GetPipeline(ctx, "pipe-overwrite", &restored)
			Expect(err).NotTo(HaveOccurred())
			Expect(restored.Status).To(Equal(manager.PipelineStatusCompleted))
			Expect(restored.Workers).To(HaveLen(1))
		})
	})

	Context("Restoring pipeline state from state store", func() {
		It("should correctly restore all pipeline state fields", func() {
			now := time.Now().Truncate(time.Second)
			endTime := now.Add(5 * time.Minute)

			original := &manager.PipelineState{
				ID:              "pipe-restore-123",
				Status:          manager.PipelineStatusCompleted,
				Error:           "",
				TotalStages:     2,
				CompletedStages: 2,
				StartTime:       now,
				EndTime:         endTime,
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{
						{Type: "list", Value: []interface{}{"example.com", "test.com"}},
						{Type: "crtsh", Value: "domain.com"},
					},
					Stages: []pipeline.Stage{
						&pipeline.ExecStage{
							Name: "scan",
							Type: "exec",
							Steps: []string{
								"nuclei -l {{.Targets.filepath}} -o /opt/recon/{{.Worker.Name}}.txt",
								"echo 'done'",
							},
						},
						&pipeline.SyncStage{
							Name: "sync-results",
							Type: "sync",
							Src:  "/opt/recon/{{.Worker.Name}}.txt",
							Dest: "./results/{{.Worker.Name}}.txt",
						},
					},
				},
				Workers: []*manager.Worker{
					{
						ID:          "w1",
						Name:        "reconswarm-aaa",
						IP:          "192.168.1.1",
						Status:      manager.WorkerStatusIdle,
						CurrentTask: "",
						InstanceID:  "i-aaa",
					},
				},
			}

			err := stateManager.SavePipeline(ctx, original.ID, original)
			Expect(err).NotTo(HaveOccurred())

			var restored manager.PipelineState
			err = stateManager.GetPipeline(ctx, original.ID, &restored)
			Expect(err).NotTo(HaveOccurred())

			// Verify scalar fields
			Expect(restored.ID).To(Equal(original.ID))
			Expect(restored.Status).To(Equal(original.Status))
			Expect(restored.Error).To(Equal(original.Error))
			Expect(restored.TotalStages).To(Equal(original.TotalStages))
			Expect(restored.CompletedStages).To(Equal(original.CompletedStages))

			// Verify time fields (compare Unix timestamps to avoid sub-second precision issues)
			Expect(restored.StartTime.Unix()).To(Equal(original.StartTime.Unix()))
			Expect(restored.EndTime.Unix()).To(Equal(original.EndTime.Unix()))

			// Verify Pipeline
			Expect(restored.Pipeline).NotTo(BeNil())
			Expect(restored.Pipeline.Targets).To(HaveLen(2))
			Expect(restored.Pipeline.Targets[0].Type).To(Equal("list"))
			Expect(restored.Pipeline.Targets[1].Type).To(Equal("crtsh"))
			Expect(restored.Pipeline.Targets[1].Value).To(Equal("domain.com"))

			// Verify Stages are restored with correct concrete types
			Expect(restored.Pipeline.Stages).To(HaveLen(2))

			execStage, ok := restored.Pipeline.Stages[0].(*pipeline.ExecStage)
			Expect(ok).To(BeTrue(), "First stage should be *ExecStage")
			Expect(execStage.Name).To(Equal("scan"))
			Expect(execStage.Type).To(Equal("exec"))
			Expect(execStage.Steps).To(HaveLen(2))
			Expect(execStage.Steps[0]).To(Equal("nuclei -l {{.Targets.filepath}} -o /opt/recon/{{.Worker.Name}}.txt"))
			Expect(execStage.Steps[1]).To(Equal("echo 'done'"))

			syncStage, ok := restored.Pipeline.Stages[1].(*pipeline.SyncStage)
			Expect(ok).To(BeTrue(), "Second stage should be *SyncStage")
			Expect(syncStage.Name).To(Equal("sync-results"))
			Expect(syncStage.Type).To(Equal("sync"))
			Expect(syncStage.Src).To(Equal("/opt/recon/{{.Worker.Name}}.txt"))
			Expect(syncStage.Dest).To(Equal("./results/{{.Worker.Name}}.txt"))

			// Verify Workers
			Expect(restored.Workers).To(HaveLen(1))
			Expect(restored.Workers[0].ID).To(Equal("w1"))
			Expect(restored.Workers[0].Name).To(Equal("reconswarm-aaa"))
			Expect(restored.Workers[0].IP).To(Equal("192.168.1.1"))
			Expect(restored.Workers[0].Status).To(Equal(manager.WorkerStatusIdle))
			Expect(restored.Workers[0].CurrentTask).To(BeEmpty())
			Expect(restored.Workers[0].InstanceID).To(Equal("i-aaa"))

			// Controller and Provisioner should be nil after deserialization (json:"-")
			Expect(restored.Workers[0].Controller).To(BeNil())
			Expect(restored.Workers[0].Provisioner).To(BeNil())
		})

		It("should restore pipeline with multiple workers preserving all fields", func() {
			original := &manager.PipelineState{
				ID:     "pipe-multi-workers",
				Status: manager.PipelineStatusCompleted,
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{
						{Type: "list", Value: []interface{}{"a.com", "b.com", "c.com", "d.com", "e.com"}},
					},
					Stages: []pipeline.Stage{
						&pipeline.ExecStage{Name: "scan", Type: "exec", Steps: []string{"echo test"}},
					},
				},
				Workers: []*manager.Worker{
					{ID: "w1", Name: "worker-1", IP: "10.0.0.1", Status: manager.WorkerStatusBusy, InstanceID: "i-1", CurrentTask: "pipe-multi-workers"},
					{ID: "w2", Name: "worker-2", IP: "10.0.0.2", Status: manager.WorkerStatusBusy, InstanceID: "i-2", CurrentTask: "pipe-multi-workers"},
					{ID: "w3", Name: "worker-3", IP: "10.0.0.3", Status: manager.WorkerStatusIdle, InstanceID: "i-3", CurrentTask: ""},
				},
			}

			err := stateManager.SavePipeline(ctx, original.ID, original)
			Expect(err).NotTo(HaveOccurred())

			var restored manager.PipelineState
			err = stateManager.GetPipeline(ctx, original.ID, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.Workers).To(HaveLen(3))

			for i, w := range restored.Workers {
				Expect(w.ID).To(Equal(original.Workers[i].ID))
				Expect(w.Name).To(Equal(original.Workers[i].Name))
				Expect(w.IP).To(Equal(original.Workers[i].IP))
				Expect(w.Status).To(Equal(original.Workers[i].Status))
				Expect(w.InstanceID).To(Equal(original.Workers[i].InstanceID))
				Expect(w.CurrentTask).To(Equal(original.Workers[i].CurrentTask))
				Expect(w.Controller).To(BeNil())
				Expect(w.Provisioner).To(BeNil())
			}
		})

		It("should restore pipeline with mixed exec and sync stages", func() {
			original := &manager.PipelineState{
				ID:     "pipe-mixed-stages",
				Status: manager.PipelineStatusCompleted,
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{
						{Type: "list", Value: []interface{}{"target.com"}},
					},
					Stages: []pipeline.Stage{
						&pipeline.ExecStage{
							Name:  "prepare",
							Type:  "exec",
							Steps: []string{"mkdir -p /opt/recon"},
						},
						&pipeline.ExecStage{
							Name: "scan",
							Type: "exec",
							Steps: []string{
								"nuclei -l {{.Targets.filepath}} -o /opt/recon/nuclei.json",
								"httpx -l {{.Targets.filepath}} -o /opt/recon/httpx.json",
							},
						},
						&pipeline.SyncStage{
							Name: "sync-nuclei",
							Type: "sync",
							Src:  "/opt/recon/nuclei.json",
							Dest: "./results/nuclei.json",
						},
						&pipeline.SyncStage{
							Name: "sync-httpx",
							Type: "sync",
							Src:  "/opt/recon/httpx.json",
							Dest: "./results/httpx.json",
						},
					},
				},
				Workers: []*manager.Worker{},
			}

			err := stateManager.SavePipeline(ctx, original.ID, original)
			Expect(err).NotTo(HaveOccurred())

			var restored manager.PipelineState
			err = stateManager.GetPipeline(ctx, original.ID, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.Pipeline.Stages).To(HaveLen(4))

			// Verify concrete types
			exec0, ok := restored.Pipeline.Stages[0].(*pipeline.ExecStage)
			Expect(ok).To(BeTrue(), "Stage 0 should be *ExecStage")
			Expect(exec0.Name).To(Equal("prepare"))
			Expect(exec0.Steps).To(Equal([]string{"mkdir -p /opt/recon"}))

			exec1, ok := restored.Pipeline.Stages[1].(*pipeline.ExecStage)
			Expect(ok).To(BeTrue(), "Stage 1 should be *ExecStage")
			Expect(exec1.Name).To(Equal("scan"))
			Expect(exec1.Steps).To(HaveLen(2))
			Expect(exec1.Steps[0]).To(ContainSubstring("nuclei"))
			Expect(exec1.Steps[1]).To(ContainSubstring("httpx"))

			sync2, ok := restored.Pipeline.Stages[2].(*pipeline.SyncStage)
			Expect(ok).To(BeTrue(), "Stage 2 should be *SyncStage")
			Expect(sync2.Name).To(Equal("sync-nuclei"))
			Expect(sync2.Src).To(Equal("/opt/recon/nuclei.json"))
			Expect(sync2.Dest).To(Equal("./results/nuclei.json"))

			sync3, ok := restored.Pipeline.Stages[3].(*pipeline.SyncStage)
			Expect(ok).To(BeTrue(), "Stage 3 should be *SyncStage")
			Expect(sync3.Name).To(Equal("sync-httpx"))
			Expect(sync3.Src).To(Equal("/opt/recon/httpx.json"))
			Expect(sync3.Dest).To(Equal("./results/httpx.json"))
		})

		It("should return error for non-existent pipeline", func() {
			var restored manager.PipelineState
			err := stateManager.GetPipeline(ctx, "non-existent-id", &restored)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("should preserve all pipeline status values through round-trip", func() {
			statuses := []manager.PipelineStatus{
				manager.PipelineStatusPending,
				manager.PipelineStatusRunning,
				manager.PipelineStatusCompleted,
				manager.PipelineStatusFailed,
			}

			for _, status := range statuses {
				state := &manager.PipelineState{
					ID:     "pipe-status-" + string(status),
					Status: status,
					Pipeline: &pipeline.Pipeline{
						Targets: []pipeline.Target{{Type: "list", Value: []interface{}{"test.com"}}},
						Stages:  []pipeline.Stage{&pipeline.ExecStage{Name: "s", Type: "exec", Steps: []string{"echo"}}},
					},
				}

				err := stateManager.SavePipeline(ctx, state.ID, state)
				Expect(err).NotTo(HaveOccurred())

				var restored manager.PipelineState
				err = stateManager.GetPipeline(ctx, state.ID, &restored)
				Expect(err).NotTo(HaveOccurred())
				Expect(restored.Status).To(Equal(status), "Status %s should survive round-trip", status)
			}
		})

		It("should restore pipeline with nil workers (not yet assigned)", func() {
			original := &manager.PipelineState{
				ID:     "pipe-nil-workers",
				Status: manager.PipelineStatusPending,
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{{Type: "list", Value: []interface{}{"test.com"}}},
					Stages:  []pipeline.Stage{&pipeline.ExecStage{Name: "s", Type: "exec", Steps: []string{"echo"}}},
				},
				Workers: nil,
			}

			err := stateManager.SavePipeline(ctx, original.ID, original)
			Expect(err).NotTo(HaveOccurred())

			var restored manager.PipelineState
			err = stateManager.GetPipeline(ctx, original.ID, &restored)
			Expect(err).NotTo(HaveOccurred())
			Expect(restored.Workers).To(BeNil())
		})

		It("should restore exec stage steps with template placeholders intact", func() {
			original := &manager.PipelineState{
				ID:     "pipe-templates",
				Status: manager.PipelineStatusCompleted,
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{{Type: "list", Value: []interface{}{"test.com"}}},
					Stages: []pipeline.Stage{
						&pipeline.ExecStage{
							Name: "complex-scan",
							Type: "exec",
							Steps: []string{
								"docker run --rm -v /opt/recon:/data -v {{.Targets.filepath}}:/data/hosts.txt projectdiscovery/nuclei:latest -l /data/hosts.txt -o /data/{{.Worker.Name}}.txt",
								"echo 'Worker: {{.Worker.Name}}, Targets: {{len .Targets.list}}'",
								"cat {{.Targets.filepath}} | sort -u > /opt/recon/sorted-{{.Worker.Name}}.txt",
							},
						},
					},
				},
			}

			err := stateManager.SavePipeline(ctx, original.ID, original)
			Expect(err).NotTo(HaveOccurred())

			var restored manager.PipelineState
			err = stateManager.GetPipeline(ctx, original.ID, &restored)
			Expect(err).NotTo(HaveOccurred())

			execStage := restored.Pipeline.Stages[0].(*pipeline.ExecStage)
			Expect(execStage.Steps).To(HaveLen(3))

			// Verify template placeholders are preserved exactly
			Expect(execStage.Steps[0]).To(ContainSubstring("{{.Targets.filepath}}"))
			Expect(execStage.Steps[0]).To(ContainSubstring("{{.Worker.Name}}"))
			Expect(execStage.Steps[1]).To(ContainSubstring("{{.Worker.Name}}"))
			Expect(execStage.Steps[1]).To(ContainSubstring("{{len .Targets.list}}"))
			Expect(execStage.Steps[2]).To(ContainSubstring("{{.Targets.filepath}}"))
			Expect(execStage.Steps[2]).To(ContainSubstring("{{.Worker.Name}}"))

			// Verify exact match for full step content
			Expect(execStage.Steps[0]).To(Equal(
				"docker run --rm -v /opt/recon:/data -v {{.Targets.filepath}}:/data/hosts.txt projectdiscovery/nuclei:latest -l /data/hosts.txt -o /data/{{.Worker.Name}}.txt",
			))
		})

		It("should correctly restore target list values after round-trip", func() {
			original := &manager.PipelineState{
				ID:     "pipe-target-values",
				Status: manager.PipelineStatusCompleted,
				Pipeline: &pipeline.Pipeline{
					Targets: []pipeline.Target{
						{Type: "list", Value: []interface{}{
							"example.com",
							"sub.example.com",
							"test's-domain.com",
							"domain-with-$pecial.com",
						}},
					},
					Stages: []pipeline.Stage{
						&pipeline.ExecStage{Name: "s", Type: "exec", Steps: []string{"echo"}},
					},
				},
			}

			err := stateManager.SavePipeline(ctx, original.ID, original)
			Expect(err).NotTo(HaveOccurred())

			var restored manager.PipelineState
			err = stateManager.GetPipeline(ctx, original.ID, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.Pipeline.Targets).To(HaveLen(1))
			values, ok := restored.Pipeline.Targets[0].Value.([]interface{})
			Expect(ok).To(BeTrue())
			Expect(values).To(HaveLen(4))
			Expect(values[0]).To(Equal("example.com"))
			Expect(values[1]).To(Equal("sub.example.com"))
			Expect(values[2]).To(Equal("test's-domain.com"))
			Expect(values[3]).To(Equal("domain-with-$pecial.com"))
		})
	})
})
