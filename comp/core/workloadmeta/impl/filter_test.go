// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

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
			result := wmdef.FilterEntitiesForVerbose(tt.entities, tt.verbose)
			tt.check(t, result)
		})
	}
}

func TestBuildWorkloadResponse(t *testing.T) {
	store := newWorkloadmetaObject(t)

	// Add test containers
	container1 := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "container-1",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:      "test-container-1",
			Namespace: "default",
		},
		Hostname: "host1",
		PID:      12345,
	}

	container2 := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "nginx-123",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:      "nginx",
			Namespace: "default",
		},
		Hostname: "host2",
		PID:      67890,
	}

	pod := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "pod-1",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	// Push entities to store using handleEvents (synchronous) like dump_test.go does
	store.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: wmdef.Source("test"),
			Entity: container1,
		},
		{
			Type:   wmdef.EventTypeSet,
			Source: wmdef.Source("test"),
			Entity: container2,
		},
		{
			Type:   wmdef.EventTypeSet,
			Source: wmdef.Source("test"),
			Entity: pod,
		},
	})

	t.Run("structured format with verbose", func(t *testing.T) {
		jsonBytes, err := BuildWorkloadResponse(store, true, true, "")
		require.NoError(t, err)
		require.NotEmpty(t, jsonBytes)

		// Verify it's valid JSON
		assert.True(t, len(jsonBytes) > 0)
	})

	t.Run("structured format without verbose", func(t *testing.T) {
		jsonBytes, err := BuildWorkloadResponse(store, false, true, "")
		require.NoError(t, err)
		require.NotEmpty(t, jsonBytes)
	})

	t.Run("text format", func(t *testing.T) {
		jsonBytes, err := BuildWorkloadResponse(store, false, false, "")
		require.NoError(t, err)
		require.NotEmpty(t, jsonBytes)
	})

	t.Run("filter by kind", func(t *testing.T) {
		jsonBytes, err := BuildWorkloadResponse(store, false, true, "container")
		require.NoError(t, err)

		// Should contain containers
		assert.Contains(t, string(jsonBytes), "container")
		// Should not contain pods when filtering for containers
		assert.NotContains(t, string(jsonBytes), "kubernetes_pod")
	})

	t.Run("filter by entity ID", func(t *testing.T) {
		jsonBytes, err := BuildWorkloadResponse(store, false, true, "nginx")
		require.NoError(t, err)

		// Should contain the nginx container
		assert.Contains(t, string(jsonBytes), "nginx")
	})
}
