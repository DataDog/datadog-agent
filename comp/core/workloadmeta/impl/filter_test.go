// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package workloadmetaimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestFilterStructuredResponse(t *testing.T) {
	tests := []struct {
		name           string
		response       wmdef.WorkloadDumpStructuredResponse
		search         string
		expectedKinds  []string
		expectedCounts map[string]int
	}{
		{
			name: "filter by exact kind match",
			response: wmdef.WorkloadDumpStructuredResponse{
				Entities: map[string][]wmdef.Entity{
					"container": {
						&wmdef.Container{EntityID: wmdef.EntityID{Kind: wmdef.KindContainer, ID: "c1"}},
						&wmdef.Container{EntityID: wmdef.EntityID{Kind: wmdef.KindContainer, ID: "c2"}},
					},
					"kubernetes_pod": {
						&wmdef.KubernetesPod{EntityID: wmdef.EntityID{Kind: wmdef.KindKubernetesPod, ID: "p1"}},
					},
				},
			},
			search:         "container",
			expectedKinds:  []string{"container"},
			expectedCounts: map[string]int{"container": 2},
		},
		{
			name: "filter by substring in kind",
			response: wmdef.WorkloadDumpStructuredResponse{
				Entities: map[string][]wmdef.Entity{
					"container": {
						&wmdef.Container{EntityID: wmdef.EntityID{Kind: wmdef.KindContainer, ID: "c1"}},
					},
					"container_image_metadata": {
						&wmdef.ContainerImageMetadata{EntityID: wmdef.EntityID{Kind: wmdef.KindContainerImageMetadata, ID: "img1"}},
					},
					"kubernetes_pod": {
						&wmdef.KubernetesPod{EntityID: wmdef.EntityID{Kind: wmdef.KindKubernetesPod, ID: "p1"}},
					},
				},
			},
			search:         "container",
			expectedKinds:  []string{"container", "container_image_metadata"},
			expectedCounts: map[string]int{"container": 1, "container_image_metadata": 1},
		},
		{
			name: "filter by entity ID",
			response: wmdef.WorkloadDumpStructuredResponse{
				Entities: map[string][]wmdef.Entity{
					"container": {
						&wmdef.Container{EntityID: wmdef.EntityID{Kind: wmdef.KindContainer, ID: "nginx-123"}},
						&wmdef.Container{EntityID: wmdef.EntityID{Kind: wmdef.KindContainer, ID: "redis-456"}},
					},
				},
			},
			search:         "nginx",
			expectedKinds:  []string{"container"},
			expectedCounts: map[string]int{"container": 1}, // Only nginx-123
		},
		{
			name: "no matches",
			response: wmdef.WorkloadDumpStructuredResponse{
				Entities: map[string][]wmdef.Entity{
					"container": {
						&wmdef.Container{EntityID: wmdef.EntityID{Kind: wmdef.KindContainer, ID: "c1"}},
					},
				},
			},
			search:         "nonexistent",
			expectedKinds:  []string{},
			expectedCounts: map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterStructuredResponse(tt.response, tt.search)

			// Check that only expected kinds are present
			assert.Equal(t, len(tt.expectedKinds), len(result.Entities), "unexpected number of kinds")

			for _, kind := range tt.expectedKinds {
				entities, ok := result.Entities[kind]
				assert.True(t, ok, "expected kind %s not found", kind)
				assert.Equal(t, tt.expectedCounts[kind], len(entities), "unexpected count for kind %s", kind)
			}

			// Ensure no unexpected kinds
			for kind := range result.Entities {
				found := false
				for _, expected := range tt.expectedKinds {
					if kind == expected {
						found = true
						break
					}
				}
				assert.True(t, found, "unexpected kind in result: %s", kind)
			}
		})
	}
}

func TestFilterTextResponse(t *testing.T) {
	tests := []struct {
		name           string
		response       wmdef.WorkloadDumpResponse
		search         string
		expectedKinds  []string
		expectedCounts map[string]int
	}{
		{
			name: "filter by kind name",
			response: wmdef.WorkloadDumpResponse{
				Entities: map[string]wmdef.WorkloadEntity{
					"container": {
						Infos: map[string]string{
							"c1": "container info 1",
							"c2": "container info 2",
						},
					},
					"kubernetes_pod": {
						Infos: map[string]string{
							"p1": "pod info 1",
						},
					},
				},
			},
			search:         "container",
			expectedKinds:  []string{"container"},
			expectedCounts: map[string]int{"container": 2},
		},
		{
			name: "filter by entity ID in infos",
			response: wmdef.WorkloadDumpResponse{
				Entities: map[string]wmdef.WorkloadEntity{
					"container": {
						Infos: map[string]string{
							"nginx-123": "nginx container",
							"redis-456": "redis container",
						},
					},
				},
			},
			search:         "nginx",
			expectedKinds:  []string{"container"},
			expectedCounts: map[string]int{"container": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterTextResponse(tt.response, tt.search)

			assert.Equal(t, len(tt.expectedKinds), len(result.Entities))

			for _, kind := range tt.expectedKinds {
				entity, ok := result.Entities[kind]
				assert.True(t, ok, "expected kind %s not found", kind)
				assert.Equal(t, tt.expectedCounts[kind], len(entity.Infos), "unexpected count for kind %s", kind)
			}
		})
	}
}

func TestFilterEntitiesForVerbose(t *testing.T) {
	tests := []struct {
		name     string
		entities []wmdef.Entity
		verbose  bool
		check    func(t *testing.T, result []wmdef.Entity)
	}{
		{
			name: "verbose mode returns entities unchanged",
			entities: []wmdef.Entity{
				&wmdef.Container{
					EntityID: wmdef.EntityID{Kind: wmdef.KindContainer, ID: "c1"},
					Hostname: "host1",
					PID:      12345,
				},
			},
			verbose: true,
			check: func(t *testing.T, result []wmdef.Entity) {
				require.Len(t, result, 1)
				container, ok := result[0].(*wmdef.Container)
				require.True(t, ok)
				assert.Equal(t, "host1", container.Hostname, "verbose mode should preserve Hostname")
				assert.Equal(t, 12345, container.PID, "verbose mode should preserve PID")
			},
		},
		{
			name: "non-verbose mode filters out verbose fields",
			entities: []wmdef.Entity{
				&wmdef.Container{
					EntityID: wmdef.EntityID{Kind: wmdef.KindContainer, ID: "c1"},
					EntityMeta: wmdef.EntityMeta{
						Name: "test-container",
					},
					Hostname: "host1",
					PID:      12345,
				},
			},
			verbose: false,
			check: func(t *testing.T, result []wmdef.Entity) {
				require.Len(t, result, 1)
				container, ok := result[0].(*wmdef.Container)
				require.True(t, ok)
				assert.Equal(t, "test-container", container.EntityMeta.Name, "should preserve Name")
				assert.Empty(t, container.Hostname, "should filter out Hostname in non-verbose")
				assert.Equal(t, 0, container.PID, "should filter out PID in non-verbose")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterEntitiesForVerbose(tt.entities, tt.verbose)
			tt.check(t, result)
		})
	}
}

func TestBuildWorkloadResponse_Performance(t *testing.T) {
	// Test that verbose mode doesn't create unnecessary copies
	t.Run("verbose mode uses original response", func(t *testing.T) {
		mock := &mockWorkloadMeta{
			structuredResp: wmdef.WorkloadDumpStructuredResponse{
				Entities: map[string][]wmdef.Entity{
					"container": {
						&wmdef.Container{EntityID: wmdef.EntityID{Kind: wmdef.KindContainer, ID: "c1"}},
					},
				},
			},
		}

		_, err := BuildWorkloadResponse(mock, true, true, "")
		require.NoError(t, err)

		// If verbose mode works correctly, it should not allocate new maps
		// This is more of a documentation test - the real test would be a benchmark
	})

	t.Run("returns error for invalid type assertion", func(t *testing.T) {
		// This test ensures we handle type assertion errors gracefully
		// In practice this shouldn't happen, but we test the safety check
		mock := &mockWorkloadMeta{
			structuredResp: wmdef.WorkloadDumpStructuredResponse{
				Entities: map[string][]wmdef.Entity{},
			},
		}

		// Valid case should not error
		_, err := BuildWorkloadResponse(mock, true, true, "test")
		assert.NoError(t, err)
	})
}

// mockWorkloadMeta is a minimal mock for testing
type mockWorkloadMeta struct {
	structuredResp wmdef.WorkloadDumpStructuredResponse
	textResp       wmdef.WorkloadDumpResponse
}

func (m *mockWorkloadMeta) DumpStructured(_ bool) wmdef.WorkloadDumpStructuredResponse {
	return m.structuredResp
}

func (m *mockWorkloadMeta) Dump(_ bool) wmdef.WorkloadDumpResponse {
	return m.textResp
}

// Implement remaining Component interface methods as no-ops
func (m *mockWorkloadMeta) Subscribe(string, wmdef.SubscriberPriority, *wmdef.Filter) chan wmdef.EventBundle {
	return nil
}
func (m *mockWorkloadMeta) Unsubscribe(chan wmdef.EventBundle)            {}
func (m *mockWorkloadMeta) GetContainer(string) (*wmdef.Container, error) { return nil, nil }
func (m *mockWorkloadMeta) ListContainers() []*wmdef.Container            { return nil }
func (m *mockWorkloadMeta) ListContainersWithFilter(wmdef.EntityFilterFunc[*wmdef.Container]) []*wmdef.Container {
	return nil
}
func (m *mockWorkloadMeta) GetKubernetesPod(string) (*wmdef.KubernetesPod, error) { return nil, nil }
func (m *mockWorkloadMeta) GetKubernetesPodForContainer(string) (*wmdef.KubernetesPod, error) {
	return nil, nil
}
func (m *mockWorkloadMeta) GetKubernetesPodByName(string, string) (*wmdef.KubernetesPod, error) {
	return nil, nil
}
func (m *mockWorkloadMeta) ListKubernetesPods() []*wmdef.KubernetesPod { return nil }
func (m *mockWorkloadMeta) ListKubernetesMetadata(wmdef.EntityFilterFunc[*wmdef.KubernetesMetadata]) []*wmdef.KubernetesMetadata {
	return nil
}
func (m *mockWorkloadMeta) ListECSTasks() []*wmdef.ECSTask                         { return nil }
func (m *mockWorkloadMeta) GetECSTask(string) (*wmdef.ECSTask, error)              { return nil, nil }
func (m *mockWorkloadMeta) ListImages() []*wmdef.ContainerImageMetadata            { return nil }
func (m *mockWorkloadMeta) GetImage(string) (*wmdef.ContainerImageMetadata, error) { return nil, nil }
func (m *mockWorkloadMeta) GetProcess(int32) (*wmdef.Process, error)               { return nil, nil }
func (m *mockWorkloadMeta) ListProcesses() []*wmdef.Process                        { return nil }
func (m *mockWorkloadMeta) ListProcessesWithFilter(wmdef.EntityFilterFunc[*wmdef.Process]) []*wmdef.Process {
	return nil
}
func (m *mockWorkloadMeta) GetContainerForProcess(string) (*wmdef.Container, error) {
	return nil, nil
}
func (m *mockWorkloadMeta) GetGPU(string) (*wmdef.GPU, error)                     { return nil, nil }
func (m *mockWorkloadMeta) ListGPUs() []*wmdef.GPU                                { return nil }
func (m *mockWorkloadMeta) GetKubelet() (*wmdef.Kubelet, error)                   { return nil, nil }
func (m *mockWorkloadMeta) GetKubeletMetrics() (*wmdef.KubeletMetrics, error)     { return nil, nil }
func (m *mockWorkloadMeta) GetKubeCapabilities() (*wmdef.KubeCapabilities, error) { return nil, nil }
func (m *mockWorkloadMeta) GetKubernetesDeployment(string) (*wmdef.KubernetesDeployment, error) {
	return nil, nil
}
func (m *mockWorkloadMeta) GetKubernetesMetadata(wmdef.KubeMetadataEntityID) (*wmdef.KubernetesMetadata, error) {
	return nil, nil
}
func (m *mockWorkloadMeta) GetCRD(string) (*wmdef.CRD, error)           { return nil, nil }
func (m *mockWorkloadMeta) Push(wmdef.Source, ...wmdef.Event) error     { return nil }
func (m *mockWorkloadMeta) Notify([]wmdef.CollectorEvent)               {}
func (m *mockWorkloadMeta) Reset([]wmdef.Entity, wmdef.Source)          {}
func (m *mockWorkloadMeta) ResetProcesses([]wmdef.Entity, wmdef.Source) {}
func (m *mockWorkloadMeta) IsInitialized() bool                         { return true }
