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
	secutils "github.com/DataDog/datadog-agent/pkg/security/utils"
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
		pid, err := strconv.Atoi(workloadID.ID)
		require.NoError(t, err)
		mockWmeta.Set(&workloadmeta.Process{
			EntityID: workloadID,
			NsPid:    int32(pid), // Set NsPid=pid to avoid triggering the procfs fallback in generic tests
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
		ID:   strconv.FormatInt(int64(pid), 10),
	}
}

type workloadTagCacheTestMocks struct {
	tagger            taggermock.Mock
	workloadMeta      workloadmetamock.Mock
	containerProvider *mock_containers.MockContainerProvider
	telemetry         telemetry.Mock
}

type workloadTagCacheTestConfig struct {
	cacheSize int
}

type workloadTagCacheTestOption func(*workloadTagCacheTestConfig)

func withCacheSize(size int) workloadTagCacheTestOption {
	return func(cfg *workloadTagCacheTestConfig) {
		cfg.cacheSize = size
	}
}

func setupWorkloadTagCache(t testing.TB, opts ...workloadTagCacheTestOption) (*WorkloadTagCache, workloadTagCacheTestMocks) {
	t.Helper()

	cfg := workloadTagCacheTestConfig{
		cacheSize: defaultCacheSize,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	ctrl := gomock.NewController(t)
	mocks := workloadTagCacheTestMocks{
		tagger:            taggerfxmock.SetupFakeTagger(t),
		workloadMeta:      testutil.GetWorkloadMetaMock(t),
		containerProvider: mock_containers.NewMockContainerProvider(ctrl),
		telemetry:         testutil.GetTelemetryMock(t),
	}

	cache, err := NewWorkloadTagCache(mocks.tagger, mocks.workloadMeta, mocks.containerProvider, mocks.telemetry, cfg.cacheSize)
	require.NoError(t, err)

	return cache, mocks
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
			cache, mocks := setupWorkloadTagCache(t)

			containerID := "test-container-id"
			cardinalityToTags := map[taggertypes.TagCardinality][]string{
				taggertypes.HighCardinality:         {"type:high"},
				taggertypes.OrchestratorCardinality: {"type:orchestrator"},
				taggertypes.LowCardinality:          {"type:low"},
			}
			workloadID := newContainerWorkloadID(containerID)

			// Set up the mock tagger with expected tags
			setWorkloadTags(t, mocks.tagger, workloadID,
				cardinalityToTags[taggertypes.LowCardinality],
				cardinalityToTags[taggertypes.OrchestratorCardinality],
				cardinalityToTags[taggertypes.HighCardinality],
			)

			mocks.workloadMeta.Set(&workloadmeta.Container{
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
	cache, _ := setupWorkloadTagCache(t)

	containerID := "nonexistent-container"

	tags, err := cache.buildContainerTags(containerID)
	assert.Error(t, err)
	assert.Nil(t, tags)
	assert.True(t, agenterrors.IsNotFound(err))
}

// TestBuildContainerTagsTaggerReturnsEmptyTags tests that buildContainerTags succeeds even when tagger returns empty tags
func TestBuildContainerTagsTaggerReturnsEmptyTags(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	containerID := "test-container-id"
	workloadID := newContainerWorkloadID(containerID)

	// Set up workloadmeta with container
	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, workloadID, workloadmeta.ContainerRuntimeContainerd)

	// The fake tagger returns empty tags for unknown entities (no error)
	tags, err := cache.buildContainerTags(containerID)
	assert.NoError(t, err)
	assert.Empty(t, tags)
}

func TestGetWorkloadTags(t *testing.T) {
	const containerID = "test-container-id"
	const processID = 123

	// Set empty procfs to avoid any confusion with the real procfs
	fakeProcFS := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{})
	kernel.WithFakeProcFS(t, fakeProcFS)

	containerWorkloadID := newContainerWorkloadID(containerID)
	processWorkloadID := newProcessWorkloadID(processID)

	expectedContainerTags := []string{"service:my-service", "env:prod"}
	expectedProcessTags := []string{"pid:123", "nspid:123"}
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
		name                      string
		cacheEntry                *workloadTagCacheEntry
		setInWorkloadMeta         bool
		setTaggerTags             []string
		expected                  map[workloadmeta.EntityID][]string
		expectErr                 bool
		expectedUpdatedCachedTags bool
		expectedCacheEntryStale   bool
	}{
		{
			name: "cache hit returns stored tags",
			cacheEntry: &workloadTagCacheEntry{
				tags:  expectedContainerTags,
				stale: false,
			},
			expected: map[workloadmeta.EntityID][]string{
				containerWorkloadID: expectedContainerTags,
				processWorkloadID:   expectedContainerTags,
			},
			expectedUpdatedCachedTags: false,
			expectedCacheEntryStale:   false,
		},
		{
			name:              "cache miss builds tags",
			setInWorkloadMeta: true,
			setTaggerTags:     expectedContainerTags,
			expected: map[workloadmeta.EntityID][]string{
				containerWorkloadID: expectedContainerTags,
				processWorkloadID:   expectedProcessTags,
			},
			expectedUpdatedCachedTags: true,
			expectedCacheEntryStale:   false,
		},
		{
			name: "invalid cache entry rebuilds tags",
			cacheEntry: &workloadTagCacheEntry{
				tags:  errorCacheTags,
				stale: true,
			},
			setInWorkloadMeta: true,
			setTaggerTags:     expectedContainerTags,
			expected: map[workloadmeta.EntityID][]string{
				containerWorkloadID: expectedContainerTags,
				processWorkloadID:   expectedProcessTags,
			},
			expectedUpdatedCachedTags: true,
			expectedCacheEntryStale:   false,
		},
		{
			name: "error returns cached tags when stale entry exists",
			cacheEntry: &workloadTagCacheEntry{
				tags:  errorCacheTags,
				stale: true,
			},
			expected: map[workloadmeta.EntityID][]string{
				containerWorkloadID: errorCacheTags,
				processWorkloadID:   errorCacheTags,
			},
			expectErr:                 true,
			expectedUpdatedCachedTags: false,
			expectedCacheEntryStale:   false,
		},
		{
			name: "error without cache entry",
			expected: map[workloadmeta.EntityID][]string{
				containerWorkloadID: nil,
				processWorkloadID:   expectedProcessTags, // base process tags are always present based on the PID
			},
			expectErr:                 true,
			expectedUpdatedCachedTags: true,
			expectedCacheEntryStale:   false,
		},
	}

	for _, workload := range workloadSetup {
		t.Run(workload.name, func(t *testing.T) {
			for _, testCase := range tests {
				t.Run(testCase.name, func(tt *testing.T) {
					cache, mocks := setupWorkloadTagCache(tt)

					mocks.containerProvider.EXPECT().
						GetPidToCid(time.Duration(0)).
						Return(map[int]string{}).
						AnyTimes()

					if testCase.cacheEntry != nil {
						setCacheEntry(cache, workload.workloadID, testCase.cacheEntry.tags, testCase.cacheEntry.stale)
					}

					if testCase.setInWorkloadMeta {
						setWorkloadInWorkloadMeta(
							tt,
							mocks.workloadMeta,
							workload.workloadID,
							workloadmeta.ContainerRuntimeContainerd,
						)
					}

					if testCase.setTaggerTags != nil {
						setWorkloadTags(tt, mocks.tagger, workload.workloadID, testCase.setTaggerTags, nil, nil)
					}

					tags, err := cache.GetOrCreateWorkloadTags(workload.workloadID)

					if testCase.expectErr {
						assert.Error(tt, err)
					} else {
						require.NoError(tt, err)
					}

					assert.Equal(tt, testCase.expected[workload.workloadID], tags, "returned tags should match the expected tags")

					cacheEntry, exists := cache.cache.Get(workload.workloadID)
					require.True(tt, exists, "cache entry should always exist after querying") // the cache entry should always exist after querying
					assert.NotNil(tt, cacheEntry)
					assert.Equal(tt, testCase.expectedCacheEntryStale, cacheEntry.stale)

					if testCase.expectedUpdatedCachedTags {
						// If we expect an update of the cached tags, we should see whatever we got from the tagger
						assert.Equal(tt, testCase.expected[workload.workloadID], cacheEntry.tags, "cache tags should have been updated with the return value")
					} else {
						var expectedTags []string
						if testCase.cacheEntry != nil {
							expectedTags = testCase.cacheEntry.tags
						}
						// If not, we should see what was in the cache entry before
						assert.Equal(tt, expectedTags, cacheEntry.tags, "cache tags should not have been modified")
					}
				})
			}
		})
	}
}

// TestInvalidate tests that Invalidate marks all cache entries as invalid
func TestInvalidate(t *testing.T) {
	cache, _ := setupWorkloadTagCache(t)

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
	cache, mocks := setupWorkloadTagCache(t)

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
	mocks.workloadMeta.Set(container)

	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.FormatInt(int64(pid), 10),
		},
		NsPid: nspid,
		Owner: &container.EntityID,
	}
	mocks.workloadMeta.Set(process)

	// Set up tagger for container
	setWorkloadTags(t, mocks.tagger, container.EntityID, containerTags, nil, nil)

	tags, err := cache.buildProcessTags(strconv.FormatInt(int64(pid), 10))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", nspid),
	}
	expectedTags = append(expectedTags, containerTags...)

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTagsWithoutContainer tests building process tags when process has no container.
// Since containerID is empty, the container provider fallback triggers but finds nothing.
func TestBuildProcessTagsWithoutContainer(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	pid := int32(1234)
	nspid := int32(5678)

	// Set up workloadmeta with process without container
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.FormatInt(int64(pid), 10),
		},
		NsPid: nspid,
		Owner: nil,
	}
	mocks.workloadMeta.Set(process)

	// containerID="" triggers the container provider fallback
	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{})

	tags, err := cache.buildProcessTags(strconv.FormatInt(int64(pid), 10))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", nspid),
	}

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTagsNsPidZero tests that nspid defaults to pid when nspid is 0.
// With the fallback logic, nspid=0 triggers the procfs fallback, and containerID=""
// triggers the container provider fallback.
func TestBuildProcessTagsNsPidZero(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	pid := int32(1234)

	// Set up workloadmeta with process with nspid = 0
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.FormatInt(int64(pid), 10),
		},
		NsPid: 0,
		Owner: nil,
	}
	mocks.workloadMeta.Set(process)

	// containerID="" triggers the container provider fallback
	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{})

	// nspid=0 triggers the procfs fallback; fake procfs has the process but no NsPid field
	fakeProcFS := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: uint32(pid), NsPid: 0},
	})
	kernel.WithFakeProcFS(t, fakeProcFS)

	tags, err := cache.buildProcessTags(strconv.FormatInt(int64(pid), 10))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", pid), // nspid should default to pid after fallback also fails
	}

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTagsPartialWmetaNsPidZero tests building process tags when
// workloadmeta has the process with a container owner but NsPid=0. Only the
// nspid fallback should trigger; the container provider should NOT be called
// because containerID is already known from wmeta.
func TestBuildProcessTagsPartialWmetaNsPidZero(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	pid := int32(1234)
	nspidFromProcfs := int32(42)
	containerID := "container-123"
	containerTags := []string{"service:my-service", "env:prod"}

	// wmeta has process with container owner but NsPid=0
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.FormatInt(int64(pid), 10),
		},
		NsPid: 0,
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
	}
	mocks.workloadMeta.Set(process)

	// Set up container and tags
	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, newContainerWorkloadID(containerID), workloadmeta.ContainerRuntimeContainerd)
	setWorkloadTags(t, mocks.tagger, newContainerWorkloadID(containerID), containerTags, nil, nil)

	// Set up procfs with NsPid for this process
	fakeProcFS := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: uint32(pid), NsPid: uint32(nspidFromProcfs)},
	})
	kernel.WithFakeProcFS(t, fakeProcFS)

	// containerProvider should NOT be called since containerID is known from wmeta
	// (gomock will fail if it's called unexpectedly)

	tags, err := cache.buildProcessTags(strconv.FormatInt(int64(pid), 10))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", nspidFromProcfs),
	}
	expectedTags = append(expectedTags, containerTags...)

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTagsNsPidMissingEntry tests that buildProcessTags falls back to pid when
// there's no NSpid entry available in procfs.
func TestBuildProcessTagsWithNoNsPidField(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	pid := int32(4321)

	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{})

	fakeProcFS := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: uint32(pid), NsPid: 0},
	})
	kernel.WithFakeProcFS(t, fakeProcFS)

	// Ensure the process exists in the fake procfs
	require.True(t, kernel.ProcessExists(int(pid)))

	tags, err := cache.buildProcessTags(strconv.FormatInt(int64(pid), 10))
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

// TestBuildProcessTagsWorkloadMetaProcessWithoutContainerID tests that when a
// process exists in workloadmeta but has no ContainerID (Owner is nil), the
// code still detects the container via the container provider fallback.
func TestBuildProcessTagsWorkloadMetaProcessWithoutContainerID(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	pid := int32(1234)
	nspid := int32(5678)
	containerID := "container-123"
	containerTags := []string{"service:my-service", "env:prod"}

	// Process is in workloadmeta with nspid but WITHOUT Owner (no ContainerID)
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.FormatInt(int64(pid), 10),
		},
		NsPid: nspid,
		Owner: nil,
	}
	mocks.workloadMeta.Set(process)

	// Container provider returns the correct mapping
	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{int(pid): containerID})

	// Set up container in workloadmeta and tagger
	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, newContainerWorkloadID(containerID), workloadmeta.ContainerRuntimeContainerd)
	setWorkloadTags(t, mocks.tagger, newContainerWorkloadID(containerID), containerTags, nil, nil)

	tags, err := cache.buildProcessTags(strconv.FormatInt(int64(pid), 10))
	require.NoError(t, err)

	expectedTags := []string{
		fmt.Sprintf("pid:%d", pid),
		fmt.Sprintf("nspid:%d", nspid),
	}
	expectedTags = append(expectedTags, containerTags...)

	assert.ElementsMatch(t, expectedTags, tags)
}

// TestBuildProcessTagsFallbackToContainerProvider tests fallback when process not in workloadmeta
func TestBuildProcessTagsFallbackToContainerProvider(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	pid := int32(1234)
	containerID := "container-123"
	containerTags := []string{"service:my-service", "env:prod"}

	// Don't set up process in workloadmeta - it should use NotFound error path

	// Set up container provider to return containerID for this PID
	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{int(pid): containerID})

	// FAke procfs but with no NSPid field
	fakeProcFS := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: uint32(pid), NsPid: 0},
	})
	kernel.WithFakeProcFS(t, fakeProcFS)

	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, newContainerWorkloadID(containerID), workloadmeta.ContainerRuntimeContainerd)
	setWorkloadTags(t, mocks.tagger, newContainerWorkloadID(containerID), containerTags, nil, nil)

	tags, err := cache.buildProcessTags(strconv.FormatInt(int64(pid), 10))
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
	cache, mocks := setupWorkloadTagCache(t)

	pid := int32(1234)
	nspid := int32(5678)
	containerID := "nonexistent-container"

	// Set up workloadmeta with process that references nonexistent container
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.FormatInt(int64(pid), 10),
		},
		NsPid: nspid,
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
	}
	mocks.workloadMeta.Set(process)

	// Don't set up container in workloadmeta - it will return NotFound

	tags, err := cache.buildProcessTags(strconv.FormatInt(int64(pid), 10))
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
	cache, mocks := setupWorkloadTagCache(t)

	pid := int32(1234)
	nspid := int32(5678)
	containerID := "container-123"

	// Set up workloadmeta with process and container
	process := &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.FormatInt(int64(pid), 10),
		},
		NsPid: nspid,
		Owner: &workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
	}
	mocks.workloadMeta.Set(process)

	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, newContainerWorkloadID(containerID), workloadmeta.ContainerRuntimeContainerd)

	// Don't set up tagger tags - the fake tagger returns empty tags (no error)

	tags, err := cache.buildProcessTags(strconv.FormatInt(int64(pid), 10))
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
	cache, _ := setupWorkloadTagCache(t)

	tags, err := cache.buildProcessTags("invalid-pid")
	assert.Error(t, err)
	assert.Nil(t, tags)
	assert.Contains(t, err.Error(), "error converting process ID to int")
}

func TestBuildProcessTagsProcessNotFound(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{})

	// Create an empty procfs to ensure the process is not found
	kernel.WithFakeProcFS(t, kernel.CreateFakeProcFS(t, nil))

	_, err := cache.buildProcessTags("1234")
	assert.Error(t, err)
	assert.True(t, agenterrors.IsNotFound(err))
}

// TestGetContainerIDFirstCall tests that getContainerID initializes pidToCid on first call
func TestGetContainerIDFirstCall(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	pid := int32(1234)
	containerID := "container-123"

	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{int(pid): containerID})

	result, err := cache.getContainerID(pid)
	require.NoError(t, err)
	assert.Equal(t, containerID, result)

	// Verify pidToCid is now populated
	assert.NotNil(t, cache.pidToCid)
	assert.Equal(t, containerID, cache.pidToCid[int(pid)])
}

// TestGetContainerIDSubsequentCall tests that getContainerID reuses cached pidToCid
func TestGetContainerIDSubsequentCall(t *testing.T) {
	cache, _ := setupWorkloadTagCache(t)

	cache.pidToCid = map[int]string{
		1234: "container-123",
		5678: "container-456",
	}

	pid := int32(1234)

	// Should not call GetPidToCid since pidToCid is already populated
	// (no EXPECT call)

	result, err := cache.getContainerID(pid)
	require.NoError(t, err)
	assert.Equal(t, "container-123", result)
}

// TestGetContainerIDPIDNotFound tests that getContainerID returns empty string when PID not found
func TestGetContainerIDPIDNotFound(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	pid := int32(9999)

	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{1234: "container-123"})

	result, err := cache.getContainerID(pid)
	require.Error(t, err)
	require.True(t, agenterrors.IsNotFound(err))
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
	require.ErrorIs(t, err, secutils.ErrNoNSPid)
	assert.Equal(t, int32(0), nspid)
}

// TestGetWorkloadTagsMultipleRuns tests the full flow through GetWorkloadTags
// for both container and process, using multiple runs with an invalidation in
// between.
func TestGetWorkloadTagsMultipleRuns(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	// Test container workload
	containerID := "container-123"
	containerWorkloadID := newContainerWorkloadID(containerID)
	containerTags := []string{"service:my-service", "env:prod"}

	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, containerWorkloadID, workloadmeta.ContainerRuntimeDocker)
	setWorkloadTags(t, mocks.tagger, containerWorkloadID, nil, nil, containerTags)

	tags, err := cache.GetOrCreateWorkloadTags(containerWorkloadID)
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
	mocks.workloadMeta.Set(process)

	tags, err = cache.GetOrCreateWorkloadTags(processWorkloadID)
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
	setWorkloadTags(t, mocks.tagger, containerWorkloadID, nil, nil, newContainerTags)

	tags, err = cache.GetOrCreateWorkloadTags(containerWorkloadID)
	require.NoError(t, err)
	assert.ElementsMatch(t, newContainerTags, tags)
	cacheEntry, exists = cache.cache.Get(containerWorkloadID)
	require.True(t, exists)
	assert.False(t, cacheEntry.stale)

	// tags for the process owned by the container should also be rebuilt
	tags, err = cache.GetOrCreateWorkloadTags(processWorkloadID)
	require.NoError(t, err)
	expectedProcessTags = append(baseProcessTags, newContainerTags...)
	assert.ElementsMatch(t, expectedProcessTags, tags)
	cacheEntry, exists = cache.cache.Get(processWorkloadID)
	require.True(t, exists)
	assert.False(t, cacheEntry.stale)
}

func TestBuildProcessTagsUsesCachedPidToCid(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	pid1 := int32(1234)
	pid2 := int32(5678)
	containerID1 := "container-123"
	containerID2 := "container-456"

	containerTags1 := []string{"service:service1"}
	containerTags2 := []string{"service:service2"}

	// Set up container provider to return both PIDs (will be called only once)
	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{
			int(pid1): containerID1,
			int(pid2): containerID2,
		}).
		Times(1) // Should only be called once

	// Set up containers in workloadmeta
	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, newContainerWorkloadID(containerID1), workloadmeta.ContainerRuntimeContainerd)
	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, newContainerWorkloadID(containerID2), workloadmeta.ContainerRuntimeContainerd)

	// Set up tagger for both containers
	setWorkloadTags(t, mocks.tagger, newContainerWorkloadID(containerID1), containerTags1, nil, nil)
	setWorkloadTags(t, mocks.tagger, newContainerWorkloadID(containerID2), containerTags2, nil, nil)

	procRoot := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{
		{Pid: uint32(pid1), NsPid: 3, Cmdline: "", Command: "", Exe: ""},
		{Pid: uint32(pid2), NsPid: 4, Cmdline: "", Command: "", Exe: ""},
	})
	kernel.WithFakeProcFS(t, procRoot)

	// First process - should initialize pidToCid
	tags1, err := cache.buildProcessTags(strconv.FormatInt(int64(pid1), 10))
	require.NoError(t, err)
	assert.Contains(t, tags1, "service:service1")

	// Second process - should reuse cached pidToCid (mockContainerProvider.EXPECT() will fail if called again)
	tags2, err := cache.buildProcessTags(strconv.FormatInt(int64(pid2), 10))
	require.NoError(t, err)
	assert.Contains(t, tags2, "service:service2")
}

func TestGetWorkloadTagsRecoversFromInitialError(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)

	containerID := "test-container-id"
	workloadID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	// First call - container not in workloadmeta, should error
	tags, err := cache.GetOrCreateWorkloadTags(workloadID)
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
	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, newContainerWorkloadID(containerID), workloadmeta.ContainerRuntimeContainerd)
	setWorkloadTags(t, mocks.tagger, newContainerWorkloadID(containerID), expectedTags, nil, nil)

	// Second call after invalidation - should succeed
	tags, err = cache.GetOrCreateWorkloadTags(workloadID)
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
	cacheSize := 20 // Use a small cache size to make the test more effective
	cache, mocks := setupWorkloadTagCache(t, withCacheSize(cacheSize))

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Use an empty fake procfs to avoid any issues with the real procfs
	fakeprocfs := kernel.CreateFakeProcFS(t, nil)
	kernel.WithFakeProcFS(t, fakeprocfs)

	// Also, set an empty map for the container provider
	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{}).
		AnyTimes()

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
				setWorkloadInWorkloadMeta(t, mocks.workloadMeta, workloadID, workloadmeta.ContainerRuntimeContainerd)
				setWorkloadTags(t, mocks.tagger, workloadID, tags, nil, nil)
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
				mocks.workloadMeta.Set(process)
			}

			createdWorkloads = append(createdWorkloads, createdWorkload{
				workloadID: workloadID,
				tags:       tags,
			})

			// Request tags to populate the cache
			expectedTelemetryMetrics.queriesNewWorkloads++
			actualTags, err := cache.GetOrCreateWorkloadTags(workloadID)
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
				_, err = mocks.workloadMeta.GetContainer(workload.workloadID.ID)
			case workloadmeta.KindProcess:
				var pid int
				pid, err = strconv.Atoi(workload.workloadID.ID)
				require.NoError(t, err)
				pid32 := int32(pid)
				_, err = mocks.workloadMeta.GetProcess(pid32)
			}
			require.NoError(t, err, "workload %+v is not in workloadmeta. Possible test logic error, this should not happen.", workload.workloadID)

			expectedTelemetryMetrics.queriesExistingWorkloads++
			actualTags, err := cache.GetOrCreateWorkloadTags(workload.workloadID)
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
					mocks.workloadMeta.Unset(&workloadmeta.Container{
						EntityID: workload.workloadID,
					})
				case workloadmeta.KindProcess:
					mocks.workloadMeta.Unset(&workloadmeta.Process{
						EntityID: workload.workloadID,
					})
				}

				createdWorkloads = slices.Delete(createdWorkloads, idx, idx+1)

				// Query it to ensure we get a stale entry, all entries have been invalidated before
				// Ignore the tags because the entry might have been evicted before and we won't have stale data
				expectedTelemetryMetrics.queriesRemovedWorkloads++
				_, err := cache.GetOrCreateWorkloadTags(workload.workloadID)
				require.Error(t, err, "stale entries should return an error")
				require.True(t, agenterrors.IsNotFound(err), "stale entries should return a NotFound error, got %v", err)
			}
		}

		cacheSizeActual := cache.Size()
		require.LessOrEqual(t, cacheSizeActual, cacheSize,
			"Cache size (%d) exceeded limit (%d) at iteration %d", cacheSizeActual, cacheSize, i)

		validateTelemetryMetrics(t, mocks.telemetry, cache, cacheSize, expectedTelemetryMetrics)
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
