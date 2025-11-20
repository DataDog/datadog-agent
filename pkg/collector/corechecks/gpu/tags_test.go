// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestContainerTagCache(t *testing.T) {
	tests := []struct {
		name             string
		runtime          workloadmeta.ContainerRuntime
		expectedCardnlty taggertypes.TagCardinality
	}{
		{
			name:             "docker runtime uses high cardinality",
			runtime:          workloadmeta.ContainerRuntimeDocker,
			expectedCardnlty: taggertypes.HighCardinality,
		},
		{
			name:             "containerd runtime uses orchestrator cardinality",
			runtime:          workloadmeta.ContainerRuntimeContainerd,
			expectedCardnlty: taggertypes.OrchestratorCardinality,
		},
		{
			name:             "crio runtime uses orchestrator cardinality",
			runtime:          workloadmeta.ContainerRuntimeCRIO,
			expectedCardnlty: taggertypes.OrchestratorCardinality,
		},
		{
			name:             "podman runtime uses orchestrator cardinality",
			runtime:          workloadmeta.ContainerRuntimePodman,
			expectedCardnlty: taggertypes.OrchestratorCardinality,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeTagger := taggerfxmock.SetupFakeTagger(t)
			cache := newContainerTagCache(fakeTagger)

			containerID := "test-container-id"
			cardinalityToTags := map[taggertypes.TagCardinality][]string{
				taggertypes.HighCardinality:         {"type:high"},
				taggertypes.OrchestratorCardinality: {"type:orchestrator"},
				taggertypes.LowCardinality:          {"type:low"},
			}

			// Set up the mock tagger with expected tags
			fakeTagger.SetTags(
				taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
				"foo",
				cardinalityToTags[taggertypes.LowCardinality],
				cardinalityToTags[taggertypes.OrchestratorCardinality],
				cardinalityToTags[taggertypes.HighCardinality],
				nil,
			)

			container := &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					ID:   containerID,
					Kind: workloadmeta.KindContainer,
				},
				Runtime: tt.runtime,
			}

			var expectedTags []string
			for cardinality, tags := range cardinalityToTags {
				if tt.expectedCardnlty >= cardinality {
					expectedTags = append(expectedTags, tags...)
				}
			}

			tags, err := cache.getContainerTags(container)
			require.NoError(t, err)
			assert.ElementsMatch(t, expectedTags, tags)
		})
	}
}

func TestContainerTagCacheCaching(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	cache := newContainerTagCache(fakeTagger)

	containerID := "test-container-id"
	expectedTags := []string{"service:my-service", "env:prod"}

	fakeTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
		"foo",
		expectedTags,
		nil,
		nil,
		nil,
	)

	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}

	// First call - should cache
	tags1, err1 := cache.getContainerTags(container)
	require.NoError(t, err1)
	assert.Equal(t, expectedTags, tags1)

	// Verify tags are in cache
	cachedTags, exists := cache.cache[containerID]
	require.True(t, exists)
	assert.Equal(t, expectedTags, cachedTags)

	// Modify the tags in the mock tagger and verify that we still get the cached tags
	fakeTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
		"foo",
		[]string{"service:my-service-modified", "env:prod-modified"},
		nil,
		nil,
		nil,
	)

	tags2, err2 := cache.getContainerTags(container)
	require.NoError(t, err2)
	assert.Equal(t, expectedTags, tags2)
	assert.Equal(t, tags1, tags2, "cached tags should be identical")
}
