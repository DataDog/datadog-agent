// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && linux && nvml

package gpu

import (
	"fmt"
	"math/rand"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	agenterrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const fakeTaggerSource = "foo"
const defaultCacheSize = 1024

// some helper functions to set up the mock data

func setCacheEntry(cache *WorkloadTagCache, workloadID workloadmeta.EntityID, tags []string, stale bool) {
	cache.cache.Add(workloadID, &workloadTagCacheEntry{
		tags:  tags,
		stale: stale,
	})
}

func setWorkloadInWorkloadMeta(t *testing.T, mockWmeta workloadmetamock.Mock, workloadID workloadmeta.EntityID, runtime workloadmeta.ContainerRuntime) {
	switch workloadID.Kind {
	case workloadmeta.KindContainer:
		mockWmeta.Set(&workloadmeta.Container{
			EntityID: workloadID,
			Runtime:  runtime,
		})
	case workloadmeta.KindProcess:
		mockWmeta.Set(&workloadmeta.Process{
			EntityID: workloadID,
		})
	default:
		t.Fatalf("unsupported workload kind: %s", workloadID.Kind)
	}
}

func setWorkloadTags(t *testing.T, mockTagger taggermock.Mock, workloadID workloadmeta.EntityID, low, orch, high []string) {
	workloadToTaggerType := map[workloadmeta.Kind]taggertypes.EntityIDPrefix{
		workloadmeta.KindContainer: taggertypes.ContainerID,
		workloadmeta.KindProcess:   taggertypes.Process,
	}

	require.Contains(t, workloadToTaggerType, workloadID.Kind, "workloadID.Kind not found in workloadToTaggerType")

	taggerID := taggertypes.NewEntityID(workloadToTaggerType[workloadID.Kind], workloadID.ID)
	mockTagger.SetTags(taggerID, fakeTaggerSource, low, orch, high, nil)
}

func newContainerWorkloadID(containerID string) workloadmeta.EntityID {
	return workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}
}

func newProcessWorkloadID(pid int32) workloadmeta.EntityID {
	return workloadmeta.EntityID{
		Kind: workloadmeta.KindProcess,
		ID:   fmt.Sprintf("%d", pid),
	}
}

// TestBuildContainerTagsCardinality tests that buildContainerTags uses the correct cardinality based on runtime
func TestBuildContainerTagsCardinality(t *testing.T) {
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
			cache, err := NewWorkloadTagCache(fakeTagger, wmetaMock, nil, testutil.GetTelemetryMock(t), defaultCacheSize)
			require.NoError(t, err)

			containerID := "test-container-id"
			cardinalityToTags := map[taggertypes.TagCardinality][]string{
				taggertypes.HighCardinality:         {"type:high"},
				taggertypes.OrchestratorCardinality: {"type:orchestrator"},
				taggertypes.LowCardinality:          {"type:low"},
			}
			workloadID := newContainerWorkloadID(containerID)

			// Set up the mock tagger with expected tags
			setWorkloadTags(
				t,
				fakeTagger,
				workloadID,
				cardinalityToTags[taggertypes.LowCardinality],
				cardinalityToTags[taggertypes.OrchestratorCardinality],
				cardinalityToTags[taggertypes.HighCardinality],
			)

			wmetaMock.Set(&workloadmeta.Container{
				EntityID: workloadID,
				Runtime:  tt.runtime,
			})

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

// TestBuildContainerTagsNotFound tests that buildContainerTags returns an error when container is not found
func TestBuildContainerTagsNotFound(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, nil, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

	containerID := "nonexistent-container"

	tags, err := cache.buildContainerTags(containerID)
	assert.Error(t, err)
	assert.Nil(t, tags)
	assert.True(t, agenterrors.IsNotFound(err))
}

// TestBuildContainerTagsTaggerReturnsEmptyTags tests that buildContainerTags succeeds even when tagger returns empty tags
func TestBuildContainerTagsTaggerReturnsEmptyTags(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, nil, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)
	containerID := "test-container-id"
	workloadID := newContainerWorkloadID(containerID)

	// Set up workloadmeta with container
	setWorkloadInWorkloadMeta(t, mockWmeta, workloadID, workloadmeta.ContainerRuntimeContainerd)

	// The fake tagger returns empty tags for unknown entities (no error)
	tags, err := cache.buildContainerTags(containerID)
	assert.NoError(t, err)
	assert.Empty(t, tags)
}

func TestGetWorkloadTags(t *testing.T) {
	const containerID = "test-container-id"
	const processID = 123

	containerWorkloadID := newContainerWorkloadID(containerID)
	processWorkloadID := newProcessWorkloadID(processID)

	expectedTags := []string{"service:my-service", "env:prod"}
	errorCacheTags := []string{"old:tag"}

	workloadSetup := []struct {
		name       string
		workloadID workloadmeta.EntityID
	}{
		{
			name:       "container",
			workloadID: containerWorkloadID,
		},
		{
			name:       "process",
			workloadID: processWorkloadID,
		},
	}

	tests := []struct {
		name              string
		workloadID        workloadmeta.EntityID
		cacheEntry        *workloadTagCacheEntry
		setInWorkloadMeta bool
		setTaggerTags     []string
		expected          []string
		expectErr         bool
		assertFunc        func(t *testing.T, cache *WorkloadTagCache)
	}{
		{
			name: "cache hit returns stored tags",
			cacheEntry: &workloadTagCacheEntry{
				tags:  expectedTags,
				stale: false,
			},
			expected: expectedTags,
		},
		{
			name:              "cache miss builds tags",
			workloadID:        containerWorkloadID,
			setInWorkloadMeta: true,
			setTaggerTags:     expectedTags,
			expected:          expectedTags,
			assertFunc: func(t *testing.T, cache *WorkloadTagCache) {
				cacheEntry, exists := cache.cache.Get(containerWorkloadID)
				require.True(t, exists)
				assert.Equal(t, expectedTags, cacheEntry.tags)
				assert.False(t, cacheEntry.stale)
			},
		},
		{
			name:       "invalid cache entry rebuilds tags",
			workloadID: containerWorkloadID,
			cacheEntry: &workloadTagCacheEntry{
				tags:  errorCacheTags,
				stale: true,
			},
			setInWorkloadMeta: true,
			setTaggerTags:     expectedTags,
			expected:          expectedTags,
			assertFunc: func(t *testing.T, cache *WorkloadTagCache) {
				cacheEntry, exists := cache.cache.Get(containerWorkloadID)
				require.True(t, exists)
				assert.Equal(t, expectedTags, cacheEntry.tags)
				assert.False(t, cacheEntry.stale)
			},
		},
		{
			name:       "error returns cached tags when entry exists",
			workloadID: containerWorkloadID,
			cacheEntry: &workloadTagCacheEntry{
				tags:  errorCacheTags,
				stale: true,
			},
			expected:  errorCacheTags,
			expectErr: true,
			assertFunc: func(t *testing.T, cache *WorkloadTagCache) {
				cacheEntry, exists := cache.cache.Get(containerWorkloadID)
				require.True(t, exists)
				assert.Equal(t, errorCacheTags, cacheEntry.tags)
				assert.False(t, cacheEntry.stale)
			},
		},
		{
			name:       "error without cache entry stores nil tags",
			workloadID: containerWorkloadID,
			expected:   nil,
			expectErr:  true,
			assertFunc: func(t *testing.T, cache *WorkloadTagCache) {
				cacheEntry, exists := cache.cache.Get(containerWorkloadID)
				require.True(t, exists)
				assert.Nil(t, cacheEntry.tags)
				assert.False(t, cacheEntry.stale)
			},
		},
	}

	for _, workload := range workloadSetup {
		t.Run(workload.name, func(t *testing.T) {
			for _, testCase := range tests {
				t.Run(testCase.name, func(tt *testing.T) {
					mockTagger := taggerfxmock.SetupFakeTagger(tt)
					mockWmeta := testutil.GetWorkloadMetaMock(tt)
					cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, nil, testutil.GetTelemetryMock(tt), defaultCacheSize)
					require.NoError(tt, err)

					if testCase.cacheEntry != nil {
						setCacheEntry(cache, testCase.workloadID, testCase.cacheEntry.tags, testCase.cacheEntry.stale)
					}

					if testCase.setInWorkloadMeta {
						setWorkloadInWorkloadMeta(
							tt,
							mockWmeta,
							testCase.workloadID,
							workloadmeta.ContainerRuntimeContainerd,
						)
					}

					if testCase.setTaggerTags != nil {
						setWorkloadTags(tt, mockTagger, testCase.workloadID, testCase.setTaggerTags, nil, nil)
					}

					tags, err := cache.GetWorkloadTags(testCase.workloadID)

					if testCase.expectErr {
						assert.Error(tt, err)
					} else {
						require.NoError(tt, err)
					}

					assert.Equal(tt, testCase.expected, tags)

					if testCase.assertFunc != nil {
						testCase.assertFunc(tt, cache)
					}
				})
			}
		})
	}
}

// TestInvalidate tests that Invalidate marks all cache entries as invalid
func TestInvalidate(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, nil, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

	// Populate cache with some entries
	workloadID1 := workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container-1"}
	workloadID2 := workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "123"}
	tags1 := []string{"tag1"}
	tags2 := []string{"tag2"}

	setCacheEntry(cache, workloadID1, tags1, true)
	setCacheEntry(cache, workloadID2, tags2, true)

	cache.MarkStale()

	// Verify all entries are marked as invalid and that the tags are still present for fallback
	cacheEntry1, exists := cache.cache.Get(workloadID1)
	require.True(t, exists)
	assert.True(t, cacheEntry1.stale)
	assert.Equal(t, tags1, cacheEntry1.tags)

	cacheEntry2, exists := cache.cache.Get(workloadID2)
	require.True(t, exists)
	assert.True(t, cacheEntry2.stale)
	assert.Equal(t, tags2, cacheEntry2.tags)
	// Verify pidToCid is cleared
	assert.Nil(t, cache.pidToCid)
}

// TestBuildProcessTagsFromWorkloadMetaIncludingContainer tests building process
// tags when data is available in workloadmeta including container tags
func TestBuildProcessTagsFromWorkloadMetaIncludingContainer(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, nil, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

	pid := int32(1234)
	nspid := int32(5678)
	containerID := "container-123"

	containerTags := []string{"service:my-service", "env:prod"}

	// Set up workloadmeta with process and container
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}
	mockWmeta.Set(container)

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   fmt.Sprintf("%d", pid),
		},
		NsPid: nspid,
		Owner: &container.EntityID,
	}
	mockWmeta.Set(process)

	// Set up tagger for container
	setWorkloadTags(t, mockTagger, container.EntityID, containerTags, nil, nil)

	tags, err := cache.buildProcessTags(fmt.Sprintf("%d", pid))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", nspid),
	}
	expectedTags = append(expectedTags, containerTags...)

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTagsWithoutContainer tests building process tags when process has no container
func TestBuildProcessTagsWithoutContainer(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, nil, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

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

// TestBuildProcessTagsNsPidZero tests that nspid defaults to pid when nspid is 0
func TestBuildProcessTagsNsPidZero(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, nil, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

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

// TestBuildProcessTagsNsPidMissingEntry tests that buildProcessTags falls back to pid when
// there's no NSpid entry available in procfs.
func TestBuildProcessTagsWithNoNsPidField(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, nil, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

	pid := int32(4321)

	fakeProcFS := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: uint32(pid), NsPid: 0},
	})
	kernel.WithFakeProcFS(t, fakeProcFS)

	tags, err := cache.buildProcessTags(fmt.Sprintf("%d", pid))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", pid),
	}

	assert.ElementsMatch(t, expectedTags, tags)
	if err != nil {
		assert.Contains(t, err.Error(), "nspid")
	}
}

// TestBuildProcessTagsFallbackToContainerProvider tests fallback when process not in workloadmeta
func TestBuildProcessTagsFallbackToContainerProvider(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

	pid := int32(1234)
	containerID := "container-123"
	containerTags := []string{"service:my-service", "env:prod"}

	// Don't set up process in workloadmeta - it should use NotFound error path

	// Set up container provider to return containerID for this PID
	mockContainerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{int(pid): containerID})

	setWorkloadInWorkloadMeta(t, mockWmeta, newContainerWorkloadID(containerID), workloadmeta.ContainerRuntimeContainerd)
	setWorkloadTags(t, mockTagger, newContainerWorkloadID(containerID), containerTags, nil, nil)

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

// TestBuildProcessTagsContainerNotFound tests behavior when container is not found
// Note: Currently IsNotFound doesn't support wrapped errors, so this returns an error
func TestBuildProcessTagsContainerNotFound(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

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
	assert.NoError(t, err) //

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", nspid),
		// no container tags
	}

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTagsContainerTagsReturnsEmpty tests behavior when tagger returns empty tags
func TestBuildProcessTagsContainerTagsReturnsEmpty(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

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

	setWorkloadInWorkloadMeta(t, mockWmeta, newContainerWorkloadID(containerID), workloadmeta.ContainerRuntimeContainerd)

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

// TestBuildProcessTagsInvalidPID tests that buildProcessTags returns error for invalid PID
func TestBuildProcessTagsInvalidPID(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

	tags, err := cache.buildProcessTags("invalid-pid")
	assert.Error(t, err)
	assert.Nil(t, tags)
	assert.Contains(t, err.Error(), "error converting process ID to int")
}

func TestBuildProcessTagsProcessNotFound(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, nil, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

	// Create an empty procfs to ensure the process is not found
	kernel.WithFakeProcFS(t, kernel.CreateFakeProcFS(t, nil))

	_, err = cache.buildProcessTags("1234")
	assert.Error(t, err)
	assert.True(t, agenterrors.IsNotFound(err))
}

// TestGetContainerIDFirstCall tests that getContainerID initializes pidToCid on first call
func TestGetContainerIDFirstCall(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

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

// TestGetContainerIDSubsequentCall tests that getContainerID reuses cached pidToCid
func TestGetContainerIDSubsequentCall(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

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

// TestGetContainerIDPIDNotFound tests that getContainerID returns empty string when PID not found
func TestGetContainerIDPIDNotFound(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

	pid := int32(9999)

	mockContainerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{1234: "container-123"})

	result := cache.getContainerID(pid)
	assert.Equal(t, "", result)
}

// TestGetNsPID_NotFoundError tests that getNsPID returns NotFound when there's no nspid field
func TestGetNsPIDNotFoundError(t *testing.T) {
	pid := int32(1234)
	fakeprocfs := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: uint32(pid), NsPid: 0}, // With 0 will not have the nspid field
	})
	kernel.WithFakeProcFS(t, fakeprocfs)

	nspid, err := getNsPID(pid)
	assert.Error(t, err)
	assert.True(t, agenterrors.IsNotFound(err))
	assert.Equal(t, int32(0), nspid)
}

// TestGetWorkloadTagsMultipleRuns tests the full flow through GetWorkloadTags
// for both container and process, using multiple runs with an invalidation in
// between.
func TestGetWorkloadTagsMultipleRuns(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

	// Test container workload
	containerID := "container-123"
	containerWorkloadID := newContainerWorkloadID(containerID)
	containerTags := []string{"service:my-service", "env:prod"}

	setWorkloadInWorkloadMeta(t, mockWmeta, containerWorkloadID, workloadmeta.ContainerRuntimeDocker)
	setWorkloadTags(t, mockTagger, containerWorkloadID, nil, nil, containerTags)

	tags, err := cache.GetWorkloadTags(containerWorkloadID)
	require.NoError(t, err)
	assert.ElementsMatch(t, containerTags, tags)

	// Test process workload
	pid := int32(1234)
	processWorkloadID := newProcessWorkloadID(pid)

	process := &workloadmeta.Process{
		EntityID: processWorkloadID,
		NsPid:    5678,
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
	}
	mockWmeta.Set(process)

	tags, err = cache.GetWorkloadTags(processWorkloadID)
	require.NoError(t, err)

	baseProcessTags := []string{
		"pid:1234",
		"nspid:5678",
	}
	expectedProcessTags := append(baseProcessTags, containerTags...)

	assert.ElementsMatch(t, expectedProcessTags, tags)

	// Verify both are cached
	cacheEntry, exists := cache.cache.Get(containerWorkloadID)
	require.True(t, exists)
	assert.False(t, cacheEntry.stale)
	cacheEntry, exists = cache.cache.Get(processWorkloadID)
	require.True(t, exists)
	assert.False(t, cacheEntry.stale)

	// Test invalidation
	cache.MarkStale()
	cacheEntry, exists = cache.cache.Get(containerWorkloadID)
	require.True(t, exists)
	assert.True(t, cacheEntry.stale)
	cacheEntry, exists = cache.cache.Get(processWorkloadID)
	require.True(t, exists)
	assert.True(t, cacheEntry.stale)

	// Test that after invalidation, we rebuild tags
	newContainerTags := []string{"service:new-service", "env:staging"}
	setWorkloadTags(t, mockTagger, containerWorkloadID, nil, nil, newContainerTags)

	tags, err = cache.GetWorkloadTags(containerWorkloadID)
	require.NoError(t, err)
	assert.ElementsMatch(t, newContainerTags, tags)
	cacheEntry, exists = cache.cache.Get(containerWorkloadID)
	require.True(t, exists)
	assert.False(t, cacheEntry.stale)

	// tags for the process owned by the container should also be rebuilt
	tags, err = cache.GetWorkloadTags(processWorkloadID)
	require.NoError(t, err)
	expectedProcessTags = append(baseProcessTags, newContainerTags...)
	assert.ElementsMatch(t, expectedProcessTags, tags)
	cacheEntry, exists = cache.cache.Get(processWorkloadID)
	require.True(t, exists)
	assert.False(t, cacheEntry.stale)
}

func TestBuildProcessTagsUsesCachedPidToCid(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockContainerProvider := mock_containers.NewMockContainerProvider(ctrl)
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

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
	setWorkloadInWorkloadMeta(t, mockWmeta, newContainerWorkloadID(containerID1), workloadmeta.ContainerRuntimeContainerd)
	setWorkloadInWorkloadMeta(t, mockWmeta, newContainerWorkloadID(containerID2), workloadmeta.ContainerRuntimeContainerd)

	// Set up tagger for both containers
	setWorkloadTags(t, mockTagger, newContainerWorkloadID(containerID1), containerTags1, nil, nil)
	setWorkloadTags(t, mockTagger, newContainerWorkloadID(containerID2), containerTags2, nil, nil)

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

func TestGetWorkloadTagsRecoversFromInitialError(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, mockContainerProvider, testutil.GetTelemetryMock(t), defaultCacheSize)
	require.NoError(t, err)

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
	cacheEntry, exists := cache.cache.Get(workloadID)
	require.True(t, exists)
	assert.False(t, cacheEntry.stale)

	// Invalidate cache
	cache.MarkStale()
	assert.True(t, cacheEntry.stale)

	// Now add the container to workloadmeta
	expectedTags := []string{"service:my-service", "env:prod"}
	setWorkloadInWorkloadMeta(t, mockWmeta, newContainerWorkloadID(containerID), workloadmeta.ContainerRuntimeContainerd)
	setWorkloadTags(t, mockTagger, newContainerWorkloadID(containerID), expectedTags, nil, nil)

	// Second call after invalidation - should succeed
	tags, err = cache.GetWorkloadTags(workloadID)
	require.NoError(t, err)
	assert.Equal(t, expectedTags, tags)
}

type createdWorkload struct {
	workloadID workloadmeta.EntityID
	tags       []string
}

type expectedTelemetryMetrics struct {
	queriesNewWorkloads      int
	queriesExistingWorkloads int
	queriesRemovedWorkloads  int
}

// TestWorkloadTagCacheSizeLimit tests that the cache size doesn't grow indefinitely
// by creating many workloads, requesting tags, invalidating, and removing workloads.
func TestWorkloadTagCacheSizeLimit(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockWmeta := testutil.GetWorkloadMetaMock(t)
	telemetryMock := testutil.GetTelemetryMock(t)
	cacheSize := 20 // Use a small cache size to make the test more effective
	cache, err := NewWorkloadTagCache(mockTagger, mockWmeta, nil, telemetryMock, cacheSize)
	require.NoError(t, err)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Use an empty fake procfs to avoid any issues with the real procfs
	fakeprocfs := kernel.CreateFakeProcFS(t, nil)
	kernel.WithFakeProcFS(t, fakeprocfs)

	createdWorkloads := make([]createdWorkload, 0)

	// Run multiple iterations to test cache behavior
	const iterations = 2000
	const workloadsPerIteration = 30
	const removeRatio = 0.4           // Remove 40% of workloads each iteration
	const queryExistingWorkloads = 10 // Query 3 additional workloads each iteration

	var expectedTelemetryMetrics expectedTelemetryMetrics

	for i := range iterations {
		// Create some random workloads
		for j := range workloadsPerIteration {
			var workloadID workloadmeta.EntityID
			var tags []string

			// Randomly choose between container and process
			if rng.Intn(2) == 0 {
				containerID := fmt.Sprintf("container-%d-%d", i, j)
				workloadID = newContainerWorkloadID(containerID)
				tags = []string{fmt.Sprintf("service:service-%d", rng.Intn(10))}
				setWorkloadInWorkloadMeta(t, mockWmeta, workloadID, workloadmeta.ContainerRuntimeContainerd)
				setWorkloadTags(t, mockTagger, workloadID, tags, nil, nil)
			} else {
				pid := int32(i*2000 + j)
				nspid := pid + 1000
				workloadID = newProcessWorkloadID(pid)
				tags = []string{fmt.Sprintf("pid:%d", pid), fmt.Sprintf("nspid:%d", nspid)}
				process := &workloadmeta.Process{
					EntityID: workloadID,
					NsPid:    pid + 1000,
					Owner:    nil,
				}
				mockWmeta.Set(process)
			}

			createdWorkloads = append(createdWorkloads, createdWorkload{
				workloadID: workloadID,
				tags:       tags,
			})

			// Request tags to populate the cache
			expectedTelemetryMetrics.queriesNewWorkloads++
			actualTags, err := cache.GetWorkloadTags(workloadID)
			require.NoError(t, err)
			require.ElementsMatch(t, tags, actualTags)
		}

		// In order to have deterministic results for the telemetry, mark all entries as valid
		// so that we know the following loop will always hit the cache
		for entry := range cache.cache.ValuesIter() {
			entry.stale = false
		}

		for range queryExistingWorkloads {
			workload := createdWorkloads[rng.Intn(len(createdWorkloads))]

			// Ensure that the workload is still in workloadmeta. This is just to make
			// debugging easier, in case we get errors that are actually cased by the test logic.
			var err error
			switch workload.workloadID.Kind {
			case workloadmeta.KindContainer:
				_, err = mockWmeta.GetContainer(workload.workloadID.ID)
			case workloadmeta.KindProcess:
				var pid int
				pid, err = strconv.Atoi(workload.workloadID.ID)
				require.NoError(t, err)
				pid32 := int32(pid)
				_, err = mockWmeta.GetProcess(pid32)
			}
			require.NoError(t, err, "workload %+v is not in workloadmeta. Possible test logic error, this should not happen.", workload.workloadID)

			expectedTelemetryMetrics.queriesExistingWorkloads++
			actualTags, err := cache.GetWorkloadTags(workload.workloadID)
			require.NoError(t, err)
			require.Equal(t, workload.tags, actualTags)
		}

		cache.MarkStale()

		// Remove some random workloads from workloadmeta
		removeCount := int(float64(len(createdWorkloads)) * removeRatio)
		if removeCount > 0 && len(createdWorkloads) > 0 {
			for k := 0; k < removeCount && len(createdWorkloads) > 0; k++ {
				idx := rng.Intn(len(createdWorkloads))
				workload := createdWorkloads[idx]

				// Remove from workloadmeta
				switch workload.workloadID.Kind {
				case workloadmeta.KindContainer:
					mockWmeta.Unset(&workloadmeta.Container{
						EntityID: workload.workloadID,
					})
				case workloadmeta.KindProcess:
					mockWmeta.Unset(&workloadmeta.Process{
						EntityID: workload.workloadID,
					})
				}

				createdWorkloads = slices.Delete(createdWorkloads, idx, idx+1)

				// Query it to ensure we get a stale entry, all entries have been invalidated before
				// Ignore the tags because the entry might have been evicted before and we won't have stale data
				expectedTelemetryMetrics.queriesRemovedWorkloads++
				_, err := cache.GetWorkloadTags(workload.workloadID)
				require.Error(t, err, "stale entries should return an error")
				require.True(t, agenterrors.IsNotFound(err), "stale entries should return a NotFound error, got %v", err)
			}
		}

		cacheSizeActual := cache.Size()
		require.LessOrEqual(t, cacheSizeActual, cacheSize,
			"Cache size (%d) exceeded limit (%d) at iteration %d", cacheSizeActual, cacheSize, i)

		validateTelemetryMetrics(t, telemetryMock, cache, cacheSize, expectedTelemetryMetrics)
	}
}

func getTotalMetricValue(telemetryMock telemetry.Mock, metricName string) int {
	metrics, err := telemetryMock.GetCountMetric(workloadTagCacheTelemetrySubsystem, metricName)
	if err != nil {
		return 0
	}

	total := 0
	for _, m := range metrics {
		total += int(m.Value())
	}
	return total
}

// validateTelemetryMetrics validates telemetry metrics match expected values
func validateTelemetryMetrics(t *testing.T, telemetryMock telemetry.Mock, cache *WorkloadTagCache, cacheSize int, expectedTelemetryMetrics expectedTelemetryMetrics) {
	subsystem := "gpu__workload_tag_cache"

	// Validate cache size gauge matches actual cache size
	cacheSizeMetrics, err := telemetryMock.GetGaugeMetric(subsystem, "size")
	require.NoError(t, err)
	require.Len(t, cacheSizeMetrics, 1)
	require.Equal(t, float64(cache.Size()), cacheSizeMetrics[0].Value(), "cache size gauge should match actual cache size")

	hits := getTotalMetricValue(telemetryMock, "hits")
	misses := getTotalMetricValue(telemetryMock, "misses")
	evictions := getTotalMetricValue(telemetryMock, "evictions")
	staleEntriesUsed := getTotalMetricValue(telemetryMock, "stale_entries_used")
	buildErrors := getTotalMetricValue(telemetryMock, "build_errors")

	// Validate cache hits (should be > 0 when querying existing workloads). We
	// cannot assert the exact number of hits, because we don't know if they
	// have been evicted
	if expectedTelemetryMetrics.queriesExistingWorkloads > 0 {
		require.Greater(t, hits, 0, "cache hits should be greater than 0 when querying existing workloads")
	}

	// Validate cache misses (should be > 0 when creating new workloads)
	if expectedTelemetryMetrics.queriesNewWorkloads > 0 {

		// There might be more misses in case there were evictions, but we know
		// for sure that these ones should have hit the miss path
		require.LessOrEqual(t, expectedTelemetryMetrics.queriesNewWorkloads, misses, "cache misses should match the expected number of queries for new workloads")
	}

	if expectedTelemetryMetrics.queriesNewWorkloads > cacheSize {
		require.Greater(t, evictions, 0, "cache evictions should occur when cache is at capacity")
	}

	if expectedTelemetryMetrics.queriesRemovedWorkloads > 0 {
		// Removed workloads can hit two paths:
		// - using the stale entry if available
		// - build error because there's no cache either.
		// There might be more stale entries from other parts of the code, so we just have a minimum value
		require.GreaterOrEqual(t, staleEntriesUsed+buildErrors, expectedTelemetryMetrics.queriesRemovedWorkloads, "stale entries used should be greater than or equal to the expected number of stale queries")
	}

	// build errors are not accounted for in here, as the test logic we're using
	// does not create any errors, and that's validated by test assertions
	actualTotalQueries := hits + misses + staleEntriesUsed + buildErrors
	expectedTotalQueries := expectedTelemetryMetrics.queriesNewWorkloads + expectedTelemetryMetrics.queriesExistingWorkloads + expectedTelemetryMetrics.queriesRemovedWorkloads
	require.Equal(t, expectedTotalQueries, actualTotalQueries, "total queries should match the expected number of queries")
}
