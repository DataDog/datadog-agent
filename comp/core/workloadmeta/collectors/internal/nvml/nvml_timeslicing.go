// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"context"
	"slices"
	"strconv"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// gfdSharingStrategyLabel is the GPU Feature Discovery label (from the
	// GPU operator) holding how the node's GPUs are shared: "time-slicing"
	// or "mps". It is what disambiguates the two, because their allocations
	// use the same "::<n>" replica device IDs.
	gfdSharingStrategyLabel = "nvidia.com/gpu.sharing-strategy"
	// gfdReplicasLabel is the GPU Feature Discovery label holding the number
	// of time-sliced or MPS replicas the node registers per GPU.
	gfdReplicasLabel = "nvidia.com/gpu.replicas"

	// nodeGPULabelsCacheDuration bounds how long the node labels are reused
	// between pulls. They are read through the cluster agent or the API
	// server and change only when the operator's sharing configuration does.
	nodeGPULabelsCacheDuration = 5 * time.Minute
)

// timeSlices is what one pull could establish about the shared NVIDIA device
// replicas (time-slicing or MPS) allocated on this node.
type timeSlices struct {
	// byContainer holds, per container, the number of shared-device
	// replicas of each base device it holds, keyed by container ID and
	// then by base device UUID.
	byContainer map[string]map[string]int
	// sharedGPUs holds the sharing verdict of each base device UUID that at
	// least one replica allocation references: the device is shared by
	// replication, whether or not more than one replica is currently held.
	sharedGPUs map[string]workloadmeta.GPUSharingMode
	// replicas holds the number of registered replicas of each base device
	// UUID, 0 when it could not be counted.
	replicas map[string]int
	// claiming holds every container that holds a shared-replica allocation
	// this pull, so that the caller retracts the mapping of the ones that
	// stop holding one.
	claiming map[string]struct{}
}

// resolveTimeSlices scans the containers' kubelet-allocated resources for
// shared NVIDIA device replicas. A replica allocation carries the device
// plugin's replica suffix on its ID ("GPU-<uuid>::<n>", "MIG-<uuid>::<n>"),
// whatever the resource name is: the suffix means the allocation is one of
// the replicas the plugin registered of the base device, so the container
// sees the whole base device and shares it with the holders of the other
// replicas. The suffix is a device-plugin convention rather than a stable
// API, so an unrecognized ID shape is simply not a replica.
//
// The node's sharing-strategy label tells which mechanism shares the device,
// because time-slicing and MPS use the same ID scheme. Without the label the
// verdict is "shared, mechanism unknown": the strategy is only labeled where
// the GPU operator runs GPU Feature Discovery.
func (c *collector) resolveTimeSlices(ctx context.Context) timeSlices {
	ts := timeSlices{
		byContainer: make(map[string]map[string]int),
		sharedGPUs:  make(map[string]workloadmeta.GPUSharingMode),
		replicas:    make(map[string]int),
		claiming:    make(map[string]struct{}),
	}

	var mode workloadmeta.GPUSharingMode
	for _, container := range c.store.ListContainers() {
		for _, resource := range container.ResolvedAllocatedResources {
			// Keyed off the ID rather than the resource name: a shared GPU
			// may be exposed as nvidia.com/gpu or as the plugin's default
			// rename nvidia.com/gpu.shared, while the device ID keeps its
			// NVIDIA shape.
			if !gpuutil.IsNvidiaKubernetesResource(resource.Name) {
				continue
			}
			replica, ok := gpuutil.ParseNvidiaDeviceReplica(resource.ID)
			if !ok {
				continue // an exclusive allocation, or an unrecognized format
			}

			if mode == "" {
				// Read the node labels once, on the first replica seen, so
				// a node without shared GPUs pays nothing for the lookup.
				mode = c.sharingModeFromNode(ctx)
			}
			base := replica.BaseDeviceID
			if ts.byContainer[container.ID] == nil {
				ts.byContainer[container.ID] = make(map[string]int)
			}
			// A container can hold several replicas of the same base device
			// (e.g. requesting 2 shared GPUs that are both replicas of one),
			// so they are counted rather than overwritten.
			ts.byContainer[container.ID][base]++
			ts.claiming[container.ID] = struct{}{}
			ts.sharedGPUs[base] = mode
		}
	}

	if len(ts.sharedGPUs) > 0 {
		c.countReplicas(ctx, ts)
	}
	return ts
}

// sharingModeFromNode returns the sharing mechanism the node's GPUs are
// shared by, from the GPU Feature Discovery sharing-strategy label. Time-
// slicing and MPS allocate with the same replica device IDs, so the label is
// what tells them apart. Without the label, the sharing is still detected
// but its mechanism is unknown.
func (c *collector) sharingModeFromNode(ctx context.Context) workloadmeta.GPUSharingMode {
	labels, err := c.getNodeGPULabels(ctx)
	if err != nil {
		// A read failure is reported as unknown rather than retried within
		// the pull: the sharing is a fact of the allocation whatever the
		// label says, and the label read is cached, so the next pull with a
		// working read refines the verdict.
		log.Debugf("cannot read the node labels to resolve the GPU sharing strategy: %v", err)
		return workloadmeta.GPUSharingModeUnknown
	}
	return sharingModeFromStrategyLabel(labels[gfdSharingStrategyLabel])
}

// sharingModeFromStrategyLabel maps a GPU Feature Discovery sharing-strategy
// label value to its sharing mode. An unknown or missing value does not
// contradict the sharing itself -- the allocation IDs say the devices are
// shared -- so it yields unknown rather than none.
func sharingModeFromStrategyLabel(strategy string) workloadmeta.GPUSharingMode {
	switch strategy {
	case "time-slicing":
		return workloadmeta.GPUSharingModeTimeSlicing
	case "mps":
		return workloadmeta.GPUSharingModeMPS
	default:
		return workloadmeta.GPUSharingModeUnknown
	}
}

// getNodeGPULabels returns the labels of this node, as the GPU sharing
// strategy resolution needs them (GPU Feature Discovery, from the GPU
// operator, labels the node's sharing strategy and replica count). The
// result is cached: the read goes to the cluster agent or the API server,
// and the labels change rarely.
func (c *collector) getNodeGPULabels(ctx context.Context) (map[string]string, error) {
	if c.nodeGPULabels != nil && time.Since(c.nodeGPULabelsAt) < nodeGPULabelsCacheDuration {
		return c.nodeGPULabels, nil
	}

	labels, err := c.fetchNodeGPULabels(ctx)
	if err != nil {
		return nil, err
	}

	c.nodeGPULabels = labels
	c.nodeGPULabelsAt = time.Now()
	return labels, nil
}

// countReplicas fills ts.replicas with the number of device replicas the
// kubelet registered of each shared base device, from the registered device
// IDs (one registered device per replica). Falls back to the node's replicas
// label, which applies to every shared device, when the registered IDs cannot
// be read.
func (c *collector) countReplicas(ctx context.Context, ts timeSlices) {
	ids, err := c.getKubeletDeviceIDs(ctx)
	if err != nil {
		log.Debugf("cannot read the kubelet registered devices to count shared GPU replicas: %v", err)
		ids = nil
	}
	countRegisteredReplicas(ts, ids)

	for base := range ts.sharedGPUs {
		if ts.replicas[base] > 0 {
			continue
		}
		// No registered ID for this base device: either the registered
		// devices could not be read, or the replicas were unregistered. Use
		// the node's replicas label, which counts the replicas of every
		// shared GPU of the node.
		if labels, err := c.getNodeGPULabels(ctx); err == nil {
			if n, err := strconv.Atoi(labels[gfdReplicasLabel]); err == nil && n > 0 {
				ts.replicas[base] = n
			}
		}
	}
}

// countRegisteredReplicas counts, per base device, the registered device IDs
// that carry that device's replica suffix. A device shared by replication is
// one registered device per replica, so the count is the number of replicas
// of the device. IDs of other shapes (exclusive GPUs, other plugins) are
// not replicas and do not count.
func countRegisteredReplicas(ts timeSlices, registeredIDs []string) {
	for _, id := range registeredIDs {
		if replica, ok := gpuutil.ParseNvidiaDeviceReplica(id); ok {
			ts.replicas[replica.BaseDeviceID]++
		}
	}
}

// gpuSharingInfo returns the sharing verdict and replica count of the GPU
// with the given UUID, if any shared-replica allocation references it.
func (ts timeSlices) gpuSharingInfo(uuid string) (workloadmeta.GPUSharingMode, int, bool) {
	mode, shared := ts.sharedGPUs[uuid]
	if !shared {
		return "", 0, false
	}
	return mode, ts.replicas[uuid], true
}

// containerReplicaUUIDs returns the base device UUIDs of the shared replicas
// allocated to the given container, in a stable order, so that the
// container's published GPUDeviceIDs does not change between pulls.
func (ts timeSlices) containerReplicaUUIDs(containerID string) []string {
	replicas := ts.byContainer[containerID]
	if len(replicas) == 0 {
		return nil
	}
	uuids := make([]string, 0, len(replicas))
	for base := range replicas {
		uuids = append(uuids, base)
	}
	slices.Sort(uuids)
	return uuids
}
