// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

// timeSlicedGPUTestLabels is the label set a time-slicing node reports, as
// captured from GPU Feature Discovery on a cluster with
// sharing.timeSlicing.replicas=4.
func timeSlicedGPUTestLabels() map[string]string {
	return map[string]string{
		"nvidia.com/gpu.sharing-strategy": "time-slicing",
		"nvidia.com/gpu.replicas":         "4",
		"nvidia.com/gpu.product":          "NVIDIA-A10G-SHARED",
	}
}

// staticNodeLabels returns a node labels reader always returning labels.
func staticNodeLabels(labels map[string]string) func(context.Context) (map[string]string, error) {
	return func(context.Context) (map[string]string, error) { return labels, nil }
}

// newTimeSliceCollector returns a collector reading the given node labels,
// so the tests can pin the label the verdicts are disambiguated with.
func newTimeSliceCollector(store workloadmeta.Component, nodeLabels map[string]string) *collector {
	c := newCollector(store, nil, noopsimpl.NewComponent())
	c.fetchNodeLabels = staticNodeLabels(nodeLabels)
	return c
}

// allocatedResource is one device-plugin allocation, as the kubelet
// PodResources API reports it.
type allocatedResource struct {
	name string
	id   string
}

// timeSlicedContainer returns a container holding the given allocations, as
// the kubelet PodResources API reports them.
func timeSlicedContainer(id string, allocations ...allocatedResource) *workloadmeta.Container {
	resources := make([]workloadmeta.ContainerAllocatedResource, 0, len(allocations))
	for _, allocation := range allocations {
		resources = append(resources, workloadmeta.ContainerAllocatedResource{
			Name: allocation.name,
			ID:   allocation.id,
		})
	}
	return &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   id,
		},
		ResolvedAllocatedResources: resources,
	}
}

func TestSharingModeFromStrategyLabel(t *testing.T) {
	assert.Equal(t, workloadmeta.GPUSharingModeTimeSlicing, sharingModeFromStrategyLabel("time-slicing"))
	assert.Equal(t, workloadmeta.GPUSharingModeMPS, sharingModeFromStrategyLabel("mps"))
	// A missing or unrecognized label still means shared: the allocation IDs
	// say so, the label only disambiguates the mechanism.
	assert.Equal(t, workloadmeta.GPUSharingModeUnknown, sharingModeFromStrategyLabel(""))
	assert.Equal(t, workloadmeta.GPUSharingModeUnknown, sharingModeFromStrategyLabel("something-new"))
}

func TestGetNodeGPULabels(t *testing.T) {
	labels := timeSlicedGPUTestLabels()
	reads := 0
	var readErr error
	c := newCollector(nil, nil, noopsimpl.NewComponent())
	c.fetchNodeLabels = func(context.Context) (map[string]string, error) {
		reads++
		if readErr != nil {
			return nil, readErr
		}
		return labels, nil
	}

	// The first call reads the labels; the second, within the interval, is
	// served from the cache.
	assert.Equal(t, labels, c.getNodeGPULabels(t.Context()))
	assert.Equal(t, labels, c.getNodeGPULabels(t.Context()))
	assert.Equal(t, 1, reads)

	// Once the interval elapses, the read fails: the last labels are kept, and
	// the next call does not retry before the interval elapses again.
	c.nodeGPULabelsAt = time.Now().Add(-nodeGPULabelsRefreshInterval)
	readErr = errors.New("cluster agent unavailable")
	assert.Equal(t, labels, c.getNodeGPULabels(t.Context()))
	assert.Equal(t, labels, c.getNodeGPULabels(t.Context()))
	assert.Equal(t, 2, reads)

	// The next successful read replaces them.
	c.nodeGPULabelsAt = time.Now().Add(-nodeGPULabelsRefreshInterval)
	readErr = nil
	labels = map[string]string{"nvidia.com/gpu.sharing-strategy": "mps"}
	assert.Equal(t, labels, c.getNodeGPULabels(t.Context()))
	assert.Equal(t, 3, reads)
}

func TestResolveTimeSlices(t *testing.T) {
	nvmltestutil.SetupMockNVML(t, testutil.WithDeviceCount(2))
	labels := timeSlicedGPUTestLabels()
	replicaID := testutil.GPUUUIDs[0] + "::2"

	t.Run("time-sliced allocation under the default resource name", func(t *testing.T) {
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("gpu-burner-0", allocatedResource{string(gpuutil.GpuNvidiaGeneric), replicaID}))
		c := newTimeSliceCollector(store, labels)

		ts := c.resolveTimeSlices(t.Context())

		want := map[string]int{testutil.GPUUUIDs[0]: 1}
		assert.Equal(t, want, ts.byContainer["gpu-burner-0"])
		assert.Equal(t, map[string]gpuSharing{
			testutil.GPUUUIDs[0]: {mode: workloadmeta.GPUSharingModeTimeSlicing, replicas: 4},
		}, ts.shared)
	})

	t.Run("a container holding several replicas of the same GPU", func(t *testing.T) {
		// A pod requesting 2 shared GPUs on a time-sliced node gets 2
		// replicas of the same base device: they are counted, not overwritten.
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("multi",
			allocatedResource{string(gpuutil.GpuNvidiaGeneric), testutil.GPUUUIDs[0] + "::0"},
			allocatedResource{string(gpuutil.GpuNvidiaGeneric), testutil.GPUUUIDs[0] + "::1"},
		))
		c := newTimeSliceCollector(store, labels)

		ts := c.resolveTimeSlices(t.Context())

		want := map[string]int{testutil.GPUUUIDs[0]: 2}
		assert.Equal(t, want, ts.byContainer["multi"])
	})

	t.Run("time-sliced allocation under a renamed resource", func(t *testing.T) {
		// renameByDefault keeps the device IDs, renames the resource, and
		// GPU Feature Discovery labels it under its original name.
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("gpu-burner-1", allocatedResource{"nvidia.com/gpu.shared", replicaID}))
		c := newTimeSliceCollector(store, labels)

		ts := c.resolveTimeSlices(t.Context())

		assert.Equal(t, gpuSharing{mode: workloadmeta.GPUSharingModeTimeSlicing, replicas: 4}, ts.shared[testutil.GPUUUIDs[0]])
		assert.Contains(t, ts.byContainer["gpu-burner-1"], testutil.GPUUUIDs[0])
	})

	t.Run("mixed MIG strategy: each MIG profile has its own labels", func(t *testing.T) {
		// Labels as GPU Feature Discovery reports them for a GPU split into
		// one 2g.48gb and two 1g.24gb devices, the 2g.48gb profile
		// time-sliced into 4 replicas and the 1g.24gb one into 2. Full GPUs
		// are not shared.
		const mig2g, mig1gA, mig1gB = "MIG-9c178886-4313-5409-a6f1-9b9216b212cb", "MIG-ce2a47ad-d053-57df-8e3c-f51ce368f577", "MIG-9634e88a-8536-5636-bcbc-ccc0a270335f"
		migLabels := map[string]string{
			"nvidia.com/gpu.sharing-strategy":         "none",
			"nvidia.com/gpu.replicas":                 "0",
			"nvidia.com/mig-2g.48gb.sharing-strategy": "time-slicing",
			"nvidia.com/mig-2g.48gb.replicas":         "4",
			"nvidia.com/mig-1g.24gb.sharing-strategy": "time-slicing",
			"nvidia.com/mig-1g.24gb.replicas":         "2",
		}
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("burner-2g", allocatedResource{"nvidia.com/mig-2g.48gb", mig2g + "::3"}))
		store.Set(timeSlicedContainer("burner-1g", allocatedResource{"nvidia.com/mig-1g.24gb.shared", mig1gA + "::1"}))
		store.Set(timeSlicedContainer("exclusive-1g", allocatedResource{"nvidia.com/mig-1g.24gb", mig1gB}))
		c := newTimeSliceCollector(store, migLabels)

		ts := c.resolveTimeSlices(t.Context())

		assert.Equal(t, map[string]gpuSharing{
			mig2g:  {mode: workloadmeta.GPUSharingModeTimeSlicing, replicas: 4},
			mig1gA: {mode: workloadmeta.GPUSharingModeTimeSlicing, replicas: 2},
		}, ts.shared)
	})

	t.Run("without a node label the sharing is unknown, not absent", func(t *testing.T) {
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("gpu-burner-2", allocatedResource{string(gpuutil.GpuNvidiaGeneric), replicaID}))
		c := newTimeSliceCollector(store, nil)

		ts := c.resolveTimeSlices(t.Context())

		assert.Equal(t, gpuSharing{mode: workloadmeta.GPUSharingModeUnknown}, ts.shared[testutil.GPUUUIDs[0]])
	})

	t.Run("MPS allocations use the same IDs, the label disambiguates", func(t *testing.T) {
		mpsLabels := timeSlicedGPUTestLabels()
		mpsLabels["nvidia.com/gpu.sharing-strategy"] = "mps"
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("mps-workload", allocatedResource{string(gpuutil.GpuNvidiaGeneric), replicaID}))
		c := newTimeSliceCollector(store, mpsLabels)

		ts := c.resolveTimeSlices(t.Context())

		assert.Equal(t, workloadmeta.GPUSharingModeMPS, ts.shared[testutil.GPUUUIDs[0]].mode)
	})

	t.Run("exclusive allocations are not replicas", func(t *testing.T) {
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("exclusive", allocatedResource{string(gpuutil.GpuNvidiaGeneric), testutil.GPUUUIDs[0]}))
		c := newTimeSliceCollector(store, labels)

		ts := c.resolveTimeSlices(t.Context())

		assert.Empty(t, ts.byContainer)
		assert.Empty(t, ts.shared)
		assert.True(t, c.nodeGPULabelsAt.IsZero(), "the node labels are not read without a replica")
	})

	t.Run("non-NVIDIA suffixed allocations are not replicas", func(t *testing.T) {
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("other-vendor", allocatedResource{"example.com/fpga", "fpga-42::2"}))
		c := newTimeSliceCollector(store, labels)

		ts := c.resolveTimeSlices(t.Context())

		assert.Empty(t, ts.shared)
	})
}

func TestPullSetsSharingModeOfTimeSlicedGPUs(t *testing.T) {
	sharingInfo := func(t *testing.T, store workloadmeta.Component, uuid string) (workloadmeta.GPUSharingMode, int) {
		gpu, err := store.GetGPU(uuid)
		require.NoError(t, err)
		return gpu.SharingMode, gpu.SharingReplicas
	}
	newPullingCollector := func(t *testing.T, nodeLabels map[string]string) (*collector, workloadmeta.Component, *workloadmeta.Container) {
		nvmltestutil.SetupMockNVML(t, testutil.WithDeviceCount(2))
		store := testutil.GetWorkloadMetaMock(t)
		container := timeSlicedContainer("gpu-burner-0", allocatedResource{string(gpuutil.GpuNvidiaGeneric), testutil.GPUUUIDs[0] + "::2"})
		store.Set(container)
		cfg := config.NewMock(t)
		cfg.SetInTest("gpu.enabled", true)
		c := newCollector(store, cfg, noopsimpl.NewComponent())
		c.fetchNodeLabels = staticNodeLabels(nodeLabels)
		return c, store, container
	}

	t.Run("the shared GPU reports the strategy and replica count", func(t *testing.T) {
		c, store, _ := newPullingCollector(t, timeSlicedGPUTestLabels())

		require.NoError(t, c.Pull(t.Context()))
		mode, replicas := sharingInfo(t, store, testutil.GPUUUIDs[0])
		assert.Equal(t, workloadmeta.GPUSharingModeTimeSlicing, mode)
		assert.Equal(t, 4, replicas)

		// The second GPU of the node is exclusive: unshared, uncounted.
		mode, replicas = sharingInfo(t, store, testutil.GPUUUIDs[1])
		assert.Empty(t, mode)
		assert.Zero(t, replicas)
	})

	t.Run("the container mapping is retracted when the allocation stops", func(t *testing.T) {
		c, _, _ := newPullingCollector(t, timeSlicedGPUTestLabels())

		require.NoError(t, c.Pull(t.Context()))
		require.Contains(t, c.seenContainerGPUs, "gpu-burner-0")

		// The pod goes away: the container, and its replica allocation with
		// it, disappear from the store.
		c.store = testutil.GetWorkloadMetaMock(t)

		events := containerGPUEvents(c, nil)
		require.Len(t, events, 1, "the gone container must be retracted")
		assert.Equal(t, workloadmeta.EventTypeUnset, events[0].Type)
		container, ok := events[0].Entity.(*workloadmeta.Container)
		require.True(t, ok)
		assert.Equal(t, "gpu-burner-0", container.ID)
		assert.Empty(t, c.seenContainerGPUs, "nothing claims once the pod is gone")
	})
}

func TestCreateContainerGPUEventsPublishesTimeSlices(t *testing.T) {
	nvmltestutil.SetupMockNVML(t, testutil.WithDeviceCount(2))
	store := testutil.GetWorkloadMetaMock(t)
	store.Set(timeSlicedContainer("gpu-burner-0", allocatedResource{string(gpuutil.GpuNvidiaGeneric), testutil.GPUUUIDs[0] + "::2"}))
	c := newTimeSliceCollector(store, timeSlicedGPUTestLabels())

	var events []workloadmeta.CollectorEvent
	require.NotPanics(t, func() {
		events = c.createContainerGPUEvents(nil, kaiFractions{}, c.resolveTimeSlices(t.Context()))
	})

	require.Len(t, events, 1, "one event for the one time-sliced container")
	container, ok := events[0].Entity.(*workloadmeta.Container)
	require.True(t, ok)
	assert.Empty(t, container.GPUDeviceIDs, "replicas are resolved from the kubelet allocation, not the mapping")
	want := map[string]int{testutil.GPUUUIDs[0]: 1}
	assert.Equal(t, want, container.GPUDeviceReplicas)
}

func TestCreateContainerGPUEventsKeepsReplicasOutOfTheDRAMapping(t *testing.T) {
	// A container holding a replica and a DRA GPU whose CDI resolution
	// fails. GPU attribution counts the GPUDeviceIDs as resolved DRA devices,
	// so publishing the replica's base UUID there would make the DRA
	// allocation look resolved and drop its GPU.
	nvmltestutil.SetupMockNVML(t, testutil.WithDeviceCount(2))
	store := testutil.GetWorkloadMetaMock(t)
	container := timeSlicedContainer("mixed", allocatedResource{string(gpuutil.GpuNvidiaGeneric), testutil.GPUUUIDs[0] + "::1"})
	container.ResolvedAllocatedResources = append(container.ResolvedAllocatedResources, workloadmeta.ContainerAllocatedResource{
		Name:       nvidiaDRADriver,
		ID:         "gpu-1",
		CdiDevices: []string{"k8s.gpu.nvidia.com/unresolvable"},
	})
	store.Set(container)
	c := newTimeSliceCollector(store, timeSlicedGPUTestLabels())

	events := c.createContainerGPUEvents(nil, kaiFractions{}, c.resolveTimeSlices(t.Context()))

	require.Len(t, events, 1)
	published, ok := events[0].Entity.(*workloadmeta.Container)
	require.True(t, ok)
	assert.Empty(t, published.GPUDeviceIDs, "the unresolved DRA GPU leaves the mapping empty")
	assert.Equal(t, map[string]int{testutil.GPUUUIDs[0]: 1}, published.GPUDeviceReplicas)
}
