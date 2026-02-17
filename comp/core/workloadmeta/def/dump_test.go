// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package workloadmeta

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_filterStructuredResponse(t *testing.T) {
	tests := []struct {
		name           string
		response       WorkloadDumpStructuredResponse
		search         string
		expectedKinds  []string
		expectedCounts map[string]int
	}{
		{
			name: "filter by exact kind match",
			response: WorkloadDumpStructuredResponse{
				Entities: map[string][]Entity{
					"container": {
						&Container{EntityID: EntityID{Kind: KindContainer, ID: "c1"}},
						&Container{EntityID: EntityID{Kind: KindContainer, ID: "c2"}},
					},
					"kubernetes_pod": {
						&KubernetesPod{EntityID: EntityID{Kind: KindKubernetesPod, ID: "p1"}},
					},
				},
			},
			search:         "container",
			expectedKinds:  []string{"container"},
			expectedCounts: map[string]int{"container": 2},
		},
		{
			name: "filter by substring in kind",
			response: WorkloadDumpStructuredResponse{
				Entities: map[string][]Entity{
					"container": {
						&Container{EntityID: EntityID{Kind: KindContainer, ID: "c1"}},
					},
					"container_image_metadata": {
						&ContainerImageMetadata{EntityID: EntityID{Kind: KindContainerImageMetadata, ID: "img1"}},
					},
					"kubernetes_pod": {
						&KubernetesPod{EntityID: EntityID{Kind: KindKubernetesPod, ID: "p1"}},
					},
				},
			},
			search:         "container",
			expectedKinds:  []string{"container", "container_image_metadata"},
			expectedCounts: map[string]int{"container": 1, "container_image_metadata": 1},
		},
		{
			name: "filter by entity ID",
			response: WorkloadDumpStructuredResponse{
				Entities: map[string][]Entity{
					"container": {
						&Container{EntityID: EntityID{Kind: KindContainer, ID: "nginx-123"}},
						&Container{EntityID: EntityID{Kind: KindContainer, ID: "redis-456"}},
					},
				},
			},
			search:         "nginx",
			expectedKinds:  []string{"container"},
			expectedCounts: map[string]int{"container": 1}, // Only nginx-123
		},
		{
			name: "no matches",
			response: WorkloadDumpStructuredResponse{
				Entities: map[string][]Entity{
					"container": {
						&Container{EntityID: EntityID{Kind: KindContainer, ID: "c1"}},
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
			result := filterStructuredResponse(tt.response, tt.search)

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

func Test_filterTextResponse(t *testing.T) {
	tests := []struct {
		name           string
		response       WorkloadDumpResponse
		search         string
		expectedKinds  []string
		expectedCounts map[string]int
	}{
		{
			name: "filter by exact kind match",
			response: WorkloadDumpResponse{
				Entities: map[string]WorkloadEntity{
					"container": {
						Infos: map[string]string{
							"sources(merged):[runtime] id: c1": "container 1 data",
							"sources(merged):[runtime] id: c2": "container 2 data",
						},
					},
					"kubernetes_pod": {
						Infos: map[string]string{
							"sources(merged):[cluster_orchestrator] id: p1": "pod 1 data",
						},
					},
				},
			},
			search:         "container",
			expectedKinds:  []string{"container"},
			expectedCounts: map[string]int{"container": 2},
		},
		{
			name: "filter by entity ID in key",
			response: WorkloadDumpResponse{
				Entities: map[string]WorkloadEntity{
					"crd": {
						Infos: map[string]string{
							"sources(merged):[kubeapiserver] id: datadogmetrics.datadoghq.com": "crd 1 data",
							"sources(merged):[kubeapiserver] id: other.datadoghq.com":          "crd 2 data",
						},
					},
				},
			},
			search:         "datadogmetrics",
			expectedKinds:  []string{"crd"},
			expectedCounts: map[string]int{"crd": 1}, // Only datadogmetrics
		},
		{
			name: "filter preserves sources() prefix in keys",
			response: WorkloadDumpResponse{
				Entities: map[string]WorkloadEntity{
					"container": {
						Infos: map[string]string{
							"sources(merged):[runtime node_orchestrator] id: nginx-123": "nginx data",
						},
					},
				},
			},
			search:         "nginx",
			expectedKinds:  []string{"container"},
			expectedCounts: map[string]int{"container": 1},
		},
		{
			name: "no matches",
			response: WorkloadDumpResponse{
				Entities: map[string]WorkloadEntity{
					"container": {
						Infos: map[string]string{
							"sources(merged):[runtime] id: c1": "container data",
						},
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
			result := filterTextResponse(tt.response, tt.search)

			// Check that only expected kinds are present
			assert.Equal(t, len(tt.expectedKinds), len(result.Entities), "unexpected number of kinds")

			for _, kind := range tt.expectedKinds {
				entity, ok := result.Entities[kind]
				assert.True(t, ok, "expected kind %s not found", kind)
				assert.Equal(t, tt.expectedCounts[kind], len(entity.Infos), "unexpected count for kind %s", kind)

				// Verify sources() prefix is preserved in all keys
				for key := range entity.Infos {
					assert.Contains(t, key, "sources(merged):", "key should contain sources() prefix: %s", key)
					assert.Contains(t, key, " id: ", "key should contain ' id: ' separator: %s", key)
				}
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
