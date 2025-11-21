// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && linux && nvml

package gpu

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	agenterrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestBuildContainerTags_Cardinality tests that buildContainerTags uses the correct cardinality based on runtime
func TestBuildContainerTags_Cardinality(t *testing.T) {
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
			wmetaMock := testutil.GetWorkloadMetaMock(t)
			cache := NewWorkloadTagCache(fakeTagger, wmetaMock, nil)

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
			wmetaMock.Set(container)

			var expectedTags []string
			for cardinality, tags := range cardinalityToTags {
				if tt.expectedCardnlty >= cardinality {
					expectedTags = append(expectedTags, tags...)
				}
			}

			tags, err := cache.buildContainerTags(containerID)
			require.NoError(t, err)
			assert.ElementsMatch(t, expectedTags, tags)
		})
	}
}

// TestBuildContainerTags_NotFound tests that buildContainerTags returns an error when container is not found
func TestBuildContainerTags_NotFound(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	containerID := "nonexistent-container"

	tags, err := cache.buildContainerTags(containerID)
	assert.Error(t, err)
	assert.Nil(t, tags)
	assert.True(t, agenterrors.IsNotFound(err))
}

// TestBuildContainerTags_TaggerReturnsEmptyTags tests that buildContainerTags succeeds even when tagger returns empty tags
func TestBuildContainerTags_TaggerReturnsEmptyTags(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	containerID := "test-container-id"

	// Set up workloadmeta with container
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}
	mockWmeta.Set(container)

	// The fake tagger returns empty tags for unknown entities (no error)
	tags, err := cache.buildContainerTags(containerID)
	assert.NoError(t, err)
	assert.Empty(t, tags)
}

// TestGetWorkloadTags_CacheHit tests that GetWorkloadTags returns cached tags when valid
func TestGetWorkloadTags_CacheHit(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	containerID := "test-container-id"
	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	expectedTags := []string{"service:my-service", "env:prod"}

	// Pre-populate cache with valid entry
	cache.cache[workloadID] = &workloadTagCacheEntry{
		tags:  expectedTags,
		valid: true,
	}

	// Should return cached tags without hitting workloadmeta or tagger, which in this case would return empty
	tags, err := cache.GetWorkloadTags(workloadID)
	assert.NoError(t, err)
	assert.Equal(t, expectedTags, tags)
}

// TestGetWorkloadTags_CacheMiss tests that GetWorkloadTags builds tags when cache is invalid or missing
func TestGetWorkloadTags_CacheMiss(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	containerID := "test-container-id"
	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	expectedTags := []string{"service:my-service", "env:prod"}

	// Set up the mock tagger
	mockTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
		"foo",
		nil,
		expectedTags,
		nil,
		nil,
	)

	// Set up workloadmeta with container
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}
	mockWmeta.Set(container)

	tags, err := cache.GetWorkloadTags(workloadID)
	assert.NoError(t, err)
	assert.Equal(t, expectedTags, tags)

	// Verify tags are now in cache
	cacheEntry, exists := cache.cache[workloadID]
	assert.True(t, exists)
	assert.Equal(t, expectedTags, cacheEntry.tags)
	assert.True(t, cacheEntry.valid)
}

// TestGetWorkloadTags_InvalidCacheEntry tests that GetWorkloadTags rebuilds tags when cache entry is invalid
func TestGetWorkloadTags_InvalidCacheEntry(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	containerID := "test-container-id"
	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	oldTags := []string{"old:tag"}
	newTags := []string{"service:my-service", "env:prod"}

	// Pre-populate cache with invalid entry
	cache.cache[workloadID] = &workloadTagCacheEntry{
		tags:  oldTags,
		valid: false,
	}

	// Set up the mock tagger with new tags
	mockTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
		"foo",
		nil,
		newTags,
		nil,
		nil,
	)

	// Set up workloadmeta with container
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}
	mockWmeta.Set(container)

	tags, err := cache.GetWorkloadTags(workloadID)
	assert.NoError(t, err)
	assert.Equal(t, newTags, tags)

	// Verify cache is updated
	cacheEntry, exists := cache.cache[workloadID]
	assert.True(t, exists)
	assert.Equal(t, newTags, cacheEntry.tags)
	assert.True(t, cacheEntry.valid)
}

// TestGetWorkloadTags_ErrorWithExistingCache tests that GetWorkloadTags returns old tags when error occurs
func TestGetWorkloadTags_ErrorWithExistingCache(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	containerID := "test-container-id"
	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	oldTags := []string{"old:tag"}

	// Pre-populate cache with invalid entry
	cache.cache[workloadID] = &workloadTagCacheEntry{
		tags:  oldTags,
		valid: false,
	}

	// Don't set up container in workloadmeta to cause error

	tags, err := cache.GetWorkloadTags(workloadID)
	assert.Error(t, err)
	assert.Equal(t, oldTags, tags, "should return old tags on error")

	// Verify cache entry is marked as valid (to avoid retrying)
	cacheEntry, exists := cache.cache[workloadID]
	assert.True(t, exists)
	assert.Equal(t, oldTags, cacheEntry.tags)
	assert.True(t, cacheEntry.valid)
}

// TestGetWorkloadTags_ErrorWithoutExistingCache tests that GetWorkloadTags returns nil tags when error occurs without cache
func TestGetWorkloadTags_ErrorWithoutExistingCache(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	containerID := "test-container-id"
	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	// Don't set up container in workloadmeta to cause error

	tags, err := cache.GetWorkloadTags(workloadID)
	assert.Error(t, err)
	assert.Nil(t, tags)

	// Verify cache entry exists and is marked as valid (to avoid retrying)
	cacheEntry, exists := cache.cache[workloadID]
	require.True(t, exists)
	assert.Nil(t, cacheEntry.tags)
	assert.True(t, cacheEntry.valid)
}

// TestGetWorkloadTags_UnsupportedKind tests that GetWorkloadTags returns error for unsupported kinds
func TestGetWorkloadTags_UnsupportedKind(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindKubernetesPod,
		ID:   "test-pod",
	}

	tags, err := cache.GetWorkloadTags(workloadID)
	assert.Error(t, err)
	assert.Nil(t, tags)
	assert.Contains(t, err.Error(), "unsupported workload kind")
}

// TestInvalidate tests that Invalidate marks all cache entries as invalid
func TestInvalidate(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	// Populate cache with some entries
	workloadID1 := workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container-1"}
	workloadID2 := workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "123"}

	cache.cache[workloadID1] = &workloadTagCacheEntry{
		tags:  []string{"tag1"},
		valid: true,
	}
	cache.cache[workloadID2] = &workloadTagCacheEntry{
		tags:  []string{"tag2"},
		valid: true,
	}

	cache.Invalidate()

	// Verify all entries are marked as invalid
	assert.False(t, cache.cache[workloadID1].valid)
	assert.False(t, cache.cache[workloadID2].valid)

	// Verify tags are still present (for fallback)
	assert.Equal(t, []string{"tag1"}, cache.cache[workloadID1].tags)
	assert.Equal(t, []string{"tag2"}, cache.cache[workloadID2].tags)

	// Verify pidToCid is cleared
	assert.Nil(t, cache.pidToCid)
}

// TestBuildProcessTags_FromWorkloadmeta tests building process tags when data is available in workloadmeta
func TestBuildProcessTags_FromWorkloadmeta(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	pid := int32(1234)
	nspid := int32(5678)
	containerID := "container-123"

	containerTags := []string{"service:my-service", "env:prod"}

	// Set up workloadmeta with process and container
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   fmt.Sprintf("%d", pid),
		},
		NsPid: nspid,
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
	}
	mockWmeta.Set(process)

	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}
	mockWmeta.Set(container)

	// Set up tagger for container
	mockTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
		"foo",
		nil,
		containerTags,
		nil,
		nil,
	)

	tags, err := cache.buildProcessTags(fmt.Sprintf("%d", pid))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", nspid),
	}
	expectedTags = append(expectedTags, containerTags...)

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTags_WithoutContainer tests building process tags when process has no container
func TestBuildProcessTags_WithoutContainer(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	pid := int32(1234)
	nspid := int32(5678)

	// Set up workloadmeta with process without container
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   fmt.Sprintf("%d", pid),
		},
		NsPid: nspid,
		Owner: nil,
	}
	mockWmeta.Set(process)

	tags, err := cache.buildProcessTags(fmt.Sprintf("%d", pid))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", nspid),
	}

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTags_NsPidZero tests that nspid defaults to pid when nspid is 0
func TestBuildProcessTags_NsPidZero(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, nil)

	pid := int32(1234)

	// Set up workloadmeta with process with nspid = 0
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   fmt.Sprintf("%d", pid),
		},
		NsPid: 0,
		Owner: nil,
	}
	mockWmeta.Set(process)

	tags, err := cache.buildProcessTags(fmt.Sprintf("%d", pid))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", pid), // nspid should default to pid
	}

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTags_FallbackToContainerProvider tests fallback when process not in workloadmeta
func TestBuildProcessTags_FallbackToContainerProvider(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider)

	pid := int32(1234)
	containerID := "container-123"
	containerTags := []string{"service:my-service", "env:prod"}

	// Don't set up process in workloadmeta - it should use NotFound error path

	// Set up container provider to return containerID for this PID
	mockContainerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{int(pid): containerID})

	// Set up container in workloadmeta for tag retrieval
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}
	mockWmeta.Set(container)

	// Set up tagger for container
	mockTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
		"foo",
		nil,
		containerTags,
		nil,
		nil,
	)

	tags, err := cache.buildProcessTags(fmt.Sprintf("%d", pid))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", pid), // defaults to pid when not containerized
	}
	expectedTags = append(expectedTags, containerTags...)

	assert.ElementsMatch(t, expectedTags, tags)

	// Verify pidToCid is cached
	assert.NotNil(t, cache.pidToCid)
	assert.Equal(t, containerID, cache.pidToCid[int(pid)])
}

// TestBuildProcessTags_ContainerNotFound tests behavior when container is not found
// Note: Currently IsNotFound doesn't support wrapped errors, so this returns an error
func TestBuildProcessTags_ContainerNotFound(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider)

	pid := int32(1234)
	nspid := int32(5678)
	containerID := "nonexistent-container"

	// Set up workloadmeta with process that references nonexistent container
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   fmt.Sprintf("%d", pid),
		},
		NsPid: nspid,
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
	}
	mockWmeta.Set(process)

	// Don't set up container in workloadmeta - it will return NotFound

	tags, err := cache.buildProcessTags(fmt.Sprintf("%d", pid))
	// Due to IsNotFound not supporting wrapped errors, this currently returns an error
	// TODO: Once IsNotFound supports wrapped errors, this should not error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error building container tags")

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", nspid),
		// no container tags
	}

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTags_ContainerTagsReturnsEmpty tests behavior when tagger returns empty tags
func TestBuildProcessTags_ContainerTagsReturnsEmpty(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider)

	pid := int32(1234)
	nspid := int32(5678)
	containerID := "container-123"

	// Set up workloadmeta with process and container
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   fmt.Sprintf("%d", pid),
		},
		NsPid: nspid,
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
	}
	mockWmeta.Set(process)

	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}
	mockWmeta.Set(container)

	// Don't set up tagger tags - the fake tagger returns empty tags (no error)

	tags, err := cache.buildProcessTags(fmt.Sprintf("%d", pid))
	require.NoError(t, err)

	// Tags should include process tags but no container tags
	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", nspid),
	}

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTags_InvalidPID tests that buildProcessTags returns error for invalid PID
func TestBuildProcessTags_InvalidPID(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider)

	tags, err := cache.buildProcessTags("invalid-pid")
	assert.Error(t, err)
	assert.Nil(t, tags)
	assert.Contains(t, err.Error(), "error converting process ID to int")
}

// TestGetContainerID_FirstCall tests that getContainerID initializes pidToCid on first call
func TestGetContainerID_FirstCall(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider)

	pid := int32(1234)
	containerID := "container-123"

	mockContainerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{int(pid): containerID})

	result := cache.getContainerID(pid)
	assert.Equal(t, containerID, result)

	// Verify pidToCid is now populated
	assert.NotNil(t, cache.pidToCid)
	assert.Equal(t, containerID, cache.pidToCid[int(pid)])
}

// TestGetContainerID_SubsequentCall tests that getContainerID reuses cached pidToCid
func TestGetContainerID_SubsequentCall(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider)

	cache.pidToCid = map[int]string{
		1234: "container-123",
		5678: "container-456",
	}

	pid := int32(1234)

	// Should not call GetPidToCid since pidToCid is already populated
	// (no EXPECT call)

	result := cache.getContainerID(pid)
	assert.Equal(t, "container-123", result)
}

// TestGetContainerID_PIDNotFound tests that getContainerID returns empty string when PID not found
func TestGetContainerID_PIDNotFound(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider)

	pid := int32(9999)

	mockContainerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{1234: "container-123"})

	result := cache.getContainerID(pid)
	assert.Equal(t, "", result)
}

// TestGetNsPID_NotFoundError tests that getNsPID preserves NotFound errors
func TestGetNsPID_NotFoundError(t *testing.T) {
	// This test would require mocking secutils.GetNsPids which is challenging
	// In a real scenario, we'd need to use integration tests or refactor to inject the dependency
	t.Skip("getNsPID requires integration testing or dependency injection refactoring")
}

// TestGetWorkloadTags_Integration tests the full flow through GetWorkloadTags for both container and process
func TestGetWorkloadTags_Integration(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider)

	// Test container workload
	containerID := "container-123"
	containerWorkloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}
	containerTags := []string{"service:my-service", "env:prod"}

	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeDocker,
	}
	mockWmeta.Set(container)

	mockTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
		"foo",
		nil,
		nil,
		containerTags,
		nil,
	)

	tags, err := cache.GetWorkloadTags(containerWorkloadID)
	require.NoError(t, err)
	assert.ElementsMatch(t, containerTags, tags)

	// Test process workload
	pid := int32(1234)
	processWorkloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindProcess,
		ID:   fmt.Sprintf("%d", pid),
	}

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   fmt.Sprintf("%d", pid),
		},
		NsPid: 5678,
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
	}
	mockWmeta.Set(process)

	tags, err = cache.GetWorkloadTags(processWorkloadID)
	require.NoError(t, err)

	expectedProcessTags := []string{
		"pid:1234",
		"nspid:5678",
	}
	expectedProcessTags = append(expectedProcessTags, containerTags...)

	assert.ElementsMatch(t, expectedProcessTags, tags)

	// Verify both are cached
	assert.True(t, cache.cache[containerWorkloadID].valid)
	assert.True(t, cache.cache[processWorkloadID].valid)

	// Test invalidation
	cache.Invalidate()
	assert.False(t, cache.cache[containerWorkloadID].valid)
	assert.False(t, cache.cache[processWorkloadID].valid)

	// Test that after invalidation, we rebuild tags
	newContainerTags := []string{"service:new-service", "env:staging"}
	mockTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
		"foo",
		nil,
		nil,
		newContainerTags,
		nil,
	)

	tags, err = cache.GetWorkloadTags(containerWorkloadID)
	require.NoError(t, err)
	assert.ElementsMatch(t, newContainerTags, tags)
	assert.True(t, cache.cache[containerWorkloadID].valid)
}

// TestBuildProcessTags_CachedPidToCid tests that subsequent calls to getContainerID reuse cached pidToCid
func TestBuildProcessTags_CachedPidToCid(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContainerProvider := mock_containers.NewMockContainerProvider(ctrl)

	cache := &WorkloadTagCache{
		cache:             make(map[workloadmeta.EntityID]*workloadTagCacheEntry),
		tagger:            mockTagger,
		wmeta:             mockWmeta,
		containerProvider: mockContainerProvider,
	}

	pid1 := int32(1234)
	pid2 := int32(5678)
	containerID1 := "container-123"
	containerID2 := "container-456"

	containerTags1 := []string{"service:service1"}
	containerTags2 := []string{"service:service2"}

	// Set up container provider to return both PIDs (will be called only once)
	mockContainerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{
			int(pid1): containerID1,
			int(pid2): containerID2,
		}).
		Times(1) // Should only be called once

	// Set up containers in workloadmeta
	container1 := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID1,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}
	mockWmeta.Set(container1)

	container2 := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID2,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}
	mockWmeta.Set(container2)

	// Set up tagger for both containers
	mockTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID1),
		"foo",
		nil,
		containerTags1,
		nil,
		nil,
	)
	mockTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID2),
		"foo",
		nil,
		containerTags2,
		nil,
		nil,
	)

	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: uint32(pid1), NsPid: 3, Cmdline: "", Command: "", Exe: ""},
		{Pid: uint32(pid2), NsPid: 4, Cmdline: "", Command: "", Exe: ""},
	})
	kernel.WithFakeProcFS(t, procRoot)

	// First process - should initialize pidToCid
	tags1, err := cache.buildProcessTags(fmt.Sprintf("%d", pid1))
	require.NoError(t, err)
	assert.Contains(t, tags1, "service:service1")

	// Second process - should reuse cached pidToCid (mockContainerProvider.EXPECT() will fail if called again)
	tags2, err := cache.buildProcessTags(fmt.Sprintf("%d", pid2))
	require.NoError(t, err)
	assert.Contains(t, tags2, "service:service2")
}

// TestGetWorkloadTags_ErrorRecovery tests that cache can recover after an error
func TestGetWorkloadTags_ErrorRecovery(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider)

	containerID := "test-container-id"
	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	// First call - container not in workloadmeta, should error
	tags, err := cache.GetWorkloadTags(workloadID)
	assert.Error(t, err)
	assert.Nil(t, tags)

	// Cache entry should be marked as valid to avoid retrying
	cacheEntry, exists := cache.cache[workloadID]
	require.True(t, exists)
	assert.True(t, cacheEntry.valid)

	// Invalidate cache
	cache.Invalidate()
	assert.False(t, cacheEntry.valid)

	// Now add the container to workloadmeta
	expectedTags := []string{"service:my-service", "env:prod"}
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   containerID,
			Kind: workloadmeta.KindContainer,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}
	mockWmeta.Set(container)

	mockTagger.SetTags(
		taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
		"foo",
		nil,
		expectedTags,
		nil,
		nil,
	)

	// Second call after invalidation - should succeed
	tags, err = cache.GetWorkloadTags(workloadID)
	require.NoError(t, err)
	assert.Equal(t, expectedTags, tags)
}

// TestBuildContainerTags_Bug86Fix verifies the bug fix where cacheEntry wasn't initialized
func TestBuildContainerTags_Bug86Fix(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider)

	containerID := "test-container-id"
	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	// Don't set up container in workloadmeta - will cause error

	// This should not panic (the bug was that cacheEntry wasn't initialized)
	tags, err := cache.GetWorkloadTags(workloadID)
	assert.Error(t, err)
	assert.Nil(t, tags)

	// Verify cache entry was created even on error
	cacheEntry, exists := cache.cache[workloadID]
	require.True(t, exists)
	assert.NotNil(t, cacheEntry)
}
