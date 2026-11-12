// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
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
		gfdSharingStrategyLabel:  "time-slicing",
		gfdReplicasLabel:         "4",
		"nvidia.com/gpu.product": "NVIDIA-A10G-SHARED",
	}
}

// newTimeSliceCollector returns a collector reading the given node labels,
// so the tests can pin the label the verdicts are disambiguated with.
func newTimeSliceCollector(store workloadmeta.Component, nodeLabels map[string]string) *collector {
	c := newCollector(store, nil, noopsimpl.NewComponent())
	if nodeLabels != nil {
		c.nodeGPULabels = nodeLabels
		c.nodeGPULabelsAt = time.Now()
	}
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

func TestCountRegisteredReplicas(t *testing.T) {
	ts := timeSlices{replicas: map[string]int{}}
	// RegisteredDevices of a node with one 4-replica time-sliced GPU, plus
	// the other registered devices such a node has.
	countRegisteredReplicas(ts, []string{
		"GPU-170ab964-06dc-a639-af22-0cafd46259c7::0",
		"GPU-170ab964-06dc-a639-af22-0cafd46259c7::1",
		"GPU-170ab964-06dc-a639-af22-0cafd46259c7::2",
		"GPU-170ab964-06dc-a639-af22-0cafd46259c7::3",
		"GPU-8b2f7c14-1e49-4db9-9d1d-b7a62eeb32c9", // exclusive GPU of the same node
		"some-other-plugin-device",                 // another plugin's device
	})

	assert.Equal(t, map[string]int{"GPU-170ab964-06dc-a639-af22-0cafd46259c7": 4}, ts.replicas)
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
		assert.Equal(t, map[string]workloadmeta.GPUSharingMode{
			testutil.GPUUUIDs[0]: workloadmeta.GPUSharingModeTimeSlicing,
		}, ts.sharedGPUs)
		assert.Contains(t, ts.claiming, "gpu-burner-0")
		// No kubelet access in this build, so the replica count comes from the
		// node's replicas label.
		assert.Equal(t, map[string]int{testutil.GPUUUIDs[0]: 4}, ts.replicas)
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
		assert.Contains(t, ts.claiming, "multi")
	})

	t.Run("time-sliced allocation under a renamed resource", func(t *testing.T) {
		// renameByDefault keeps the device IDs, renames the resource.
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("gpu-burner-1", allocatedResource{"nvidia.com/gpu.shared", replicaID}))
		c := newTimeSliceCollector(store, labels)

		ts := c.resolveTimeSlices(t.Context())

		assert.Equal(t, workloadmeta.GPUSharingModeTimeSlicing, ts.sharedGPUs[testutil.GPUUUIDs[0]])
		assert.Contains(t, ts.byContainer["gpu-burner-1"], testutil.GPUUUIDs[0])
	})

	t.Run("without a node label the sharing is unknown, not absent", func(t *testing.T) {
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("gpu-burner-2", allocatedResource{string(gpuutil.GpuNvidiaGeneric), replicaID}))
		c := newTimeSliceCollector(store, nil)

		ts := c.resolveTimeSlices(t.Context())

		assert.Equal(t, workloadmeta.GPUSharingModeUnknown, ts.sharedGPUs[testutil.GPUUUIDs[0]])
	})

	t.Run("MPS allocations use the same IDs, the label disambiguates", func(t *testing.T) {
		mpsLabels := timeSlicedGPUTestLabels()
		mpsLabels[gfdSharingStrategyLabel] = "mps"
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("mps-workload", allocatedResource{string(gpuutil.GpuNvidiaGeneric), replicaID}))
		c := newTimeSliceCollector(store, mpsLabels)

		ts := c.resolveTimeSlices(t.Context())

		assert.Equal(t, workloadmeta.GPUSharingModeMPS, ts.sharedGPUs[testutil.GPUUUIDs[0]])
	})

	t.Run("exclusive allocations are not replicas", func(t *testing.T) {
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("exclusive", allocatedResource{string(gpuutil.GpuNvidiaGeneric), testutil.GPUUUIDs[0]}))
		c := newTimeSliceCollector(store, labels)

		ts := c.resolveTimeSlices(t.Context())

		assert.Empty(t, ts.byContainer)
		assert.Empty(t, ts.sharedGPUs)
		assert.Empty(t, ts.claiming)
		assert.Empty(t, ts.replicas)
	})

	t.Run("non-NVIDIA suffixed allocations are not replicas", func(t *testing.T) {
		store := testutil.GetWorkloadMetaMock(t)
		store.Set(timeSlicedContainer("other-vendor", allocatedResource{"example.com/fpga", "fpga-42::2"}))
		c := newTimeSliceCollector(store, labels)

		ts := c.resolveTimeSlices(t.Context())

		assert.Empty(t, ts.sharedGPUs)
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
		if nodeLabels != nil {
			c.nodeGPULabels = nodeLabels
			c.nodeGPULabelsAt = time.Now()
		}
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
	assert.Equal(t, []string{testutil.GPUUUIDs[0]}, container.GPUDeviceIDs, "the base GPU UUID, without the replica suffix")
	want := map[string]int{testutil.GPUUUIDs[0]: 1}
	assert.Equal(t, want, container.GPUDeviceReplicas)
}
