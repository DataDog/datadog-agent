// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && linux && nvml

package gpu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
)

// TestFilterAutoscalingDistTags verifies that only keys in autoscalingDistTagKeys survive.
func TestFilterAutoscalingDistTags(t *testing.T) {
	input := []string{
		// must pass through
		"env:prod",
		"service:my-svc",
		"version:v1",
		"kube_container_name:main",
		"kube_deployment:my-deploy",
		"kube_namespace:default",
		"kube_ownerref_kind:Deployment",
		"kube_ownerref_name:my-deploy",
		"kube_stateful_set:my-sts",
		"kube_daemon_set:my-ds",
		"kube_autoscaler_kind:WPA",
		"kube_argo_rollout:my-rollout",
		// must be dropped
		"host:i-12345",
		"image_name:ubuntu",
		"container_id:abc",
		"kube_cluster_name:prod-cluster",
		"orch_cluster_id:123",
		"gpu_uuid:GPU-1234",
		"badtag",
		":nokey",
	}
	got := filterAutoscalingDistTags(input)
	assert.ElementsMatch(t, []string{
		"env:prod",
		"service:my-svc",
		"version:v1",
		"kube_container_name:main",
		"kube_deployment:my-deploy",
		"kube_namespace:default",
		"kube_ownerref_kind:Deployment",
		"kube_ownerref_name:my-deploy",
		"kube_stateful_set:my-sts",
		"kube_daemon_set:my-ds",
		"kube_autoscaler_kind:WPA",
		"kube_argo_rollout:my-rollout",
	}, got)
}

// TestResolveContainerIDContainer verifies that a container workload resolves to itself.
func TestResolveContainerIDContainer(t *testing.T) {
	cache, _ := setupWorkloadTagCache(t)
	containerID := "test-container"
	wid := newContainerWorkloadID(containerID)
	got, err := cache.resolveContainerID(wid)
	require.NoError(t, err)
	assert.Equal(t, containerID, got)
}

// TestResolveContainerIDProcessViaOwner verifies process -> container resolution through Owner.
func TestResolveContainerIDProcessViaOwner(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	pid := int32(2222)
	containerID := "container-owner"
	process := &workloadmeta.Process{
		EntityID: newProcessWorkloadID(pid),
		NsPid:    pid,
		Owner:    &workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
	}
	mocks.workloadMeta.Set(process)
	got, err := cache.resolveContainerID(newProcessWorkloadID(pid))
	require.NoError(t, err)
	assert.Equal(t, containerID, got)
}

// TestResolveContainerIDProcessViaContainerID verifies fallback to process.ContainerID.
func TestResolveContainerIDProcessViaContainerID(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	pid := int32(3333)
	containerID := "container-via-field"
	process := &workloadmeta.Process{
		EntityID:    newProcessWorkloadID(pid),
		NsPid:       pid,
		Owner:       nil,
		ContainerID: containerID,
	}
	mocks.workloadMeta.Set(process)
	mocks.containerProvider.EXPECT().
		GetPidToCid(time.Duration(0)).
		Return(map[int]string{}).
		AnyTimes()
	got, err := cache.resolveContainerID(newProcessWorkloadID(pid))
	require.NoError(t, err)
	assert.Equal(t, containerID, got)
}

// TestGetLowCardContainerTagsFiltersToAllowlist verifies that GetLowCardContainerTags
// returns only allowed tags for a container workload.
func TestGetLowCardContainerTagsFiltersToAllowlist(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	containerID := "container-lowcard"
	wid := newContainerWorkloadID(containerID)

	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, wid, workloadmeta.ContainerRuntimeContainerd)

	// Tagger returns a mix of allowed and non-allowed tags at low cardinality
	containerEntityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)
	mocks.tagger.SetTags(containerEntityID, fakeTaggerSource,
		[]string{"kube_namespace:default", "kube_deployment:my-deploy", "host:node1", "image_name:ubuntu"},
		nil, nil, nil,
	)

	got, err := cache.GetLowCardContainerTags(containerID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"kube_namespace:default", "kube_deployment:my-deploy"}, got)
}

// TestGetLowCardContainerTagsForProcessWorkload verifies that a process workload
// resolves to its owning container's low-card tags.
func TestGetLowCardContainerTagsForProcessWorkload(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	pid := int32(5555)
	containerID := "container-for-process"
	process := &workloadmeta.Process{
		EntityID: newProcessWorkloadID(pid),
		NsPid:    pid,
		Owner:    &workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
	}
	mocks.workloadMeta.Set(process)

	containerWID := newContainerWorkloadID(containerID)
	setWorkloadInWorkloadMeta(t, mocks.workloadMeta, containerWID, workloadmeta.ContainerRuntimeContainerd)

	containerEntityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)
	mocks.tagger.SetTags(containerEntityID, fakeTaggerSource,
		[]string{"kube_namespace:gpu-ns", "env:staging", "host:my-node"},
		nil, nil, nil,
	)

	got, err := cache.GetLowCardContainerTags(containerID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"kube_namespace:gpu-ns", "env:staging"}, got)
}

// TestContainerDistributionsAggregateAcrossProcesses verifies that per-process
// source metrics belonging to the same container are summed into a single
// distribution sample rather than emitted once per process.
func TestContainerDistributionsAggregateAcrossProcesses(t *testing.T) {
	cache, mocks := setupWorkloadTagCache(t)
	containerID := "multi-process-container"

	// Two processes that both resolve to the same container.
	pids := []int32{1001, 1002}
	for _, pid := range pids {
		mocks.workloadMeta.Set(&workloadmeta.Process{
			EntityID: newProcessWorkloadID(pid),
			NsPid:    pid,
			Owner:    &workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		})
	}

	containerEntityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)
	mocks.tagger.SetTags(containerEntityID, fakeTaggerSource,
		[]string{"kube_namespace:gpu-ns"}, nil, nil, nil,
	)

	check := &Check{workloadTagCache: cache}

	acc := newContainerDistAccumulator()
	check.accumulateContainerDistributions(acc, &nvidia.Metric{
		Name:                processCoreUsageMetric,
		Value:               3,
		AssociatedWorkloads: []workloadmeta.EntityID{newProcessWorkloadID(pids[0])},
	}, []workloadmeta.EntityID{newProcessWorkloadID(pids[0])})
	check.accumulateContainerDistributions(acc, &nvidia.Metric{
		Name:                processCoreUsageMetric,
		Value:               5,
		AssociatedWorkloads: []workloadmeta.EntityID{newProcessWorkloadID(pids[1])},
	}, []workloadmeta.EntityID{newProcessWorkloadID(pids[1])})

	mockSender := mocksender.NewMockSender("gpu")
	mockSender.On("Distribution", containerGPUCoreUsageDist, 8.0, "", []string{"kube_namespace:gpu-ns"}).Return()

	errs := check.emitContainerDistributions(acc, mockSender)
	require.Empty(t, errs)

	// Exactly one aggregated sample (3 + 5 = 8), not one per process.
	mockSender.AssertNumberOfCalls(t, "Distribution", 1)
	mockSender.AssertExpectations(t)
}
