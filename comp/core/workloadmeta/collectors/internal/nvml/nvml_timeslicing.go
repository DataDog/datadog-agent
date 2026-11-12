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
	// byContainer holds, per container ID, the replica count of each
	// base device UUID it holds.
	byContainer map[string]map[string]int
	// sharedGPUs holds the sharing mode of each base device UUID a
	// replica allocation references.
	sharedGPUs map[string]workloadmeta.GPUSharingMode
	// replicas holds the registered replica count of each base device.
	replicas map[string]int
	// claiming holds the containers holding a replica allocation this
	// pull, for the caller's retraction.
	claiming map[string]struct{}
}

// resolveTimeSlices scans the containers' kubelet-allocated resources for
// replica allocations (device IDs suffixed with "::<n>"), grouped by base
// device. The node's sharing-strategy label disambiguates time-slicing from
// MPS, which share the same ID scheme; without it the verdict is unknown.
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

// sharingModeFromNode returns the sharing mechanism from the GPU Feature
// Discovery sharing-strategy node label, unknown when it is missing.
func (c *collector) sharingModeFromNode(ctx context.Context) workloadmeta.GPUSharingMode {
	labels, err := c.getNodeGPULabels(ctx)
	if err != nil {
		log.Debugf("cannot read the node labels to resolve the GPU sharing strategy: %v", err)
		return workloadmeta.GPUSharingModeUnknown
	}
	return sharingModeFromStrategyLabel(labels[gfdSharingStrategyLabel])
}

// sharingModeFromStrategyLabel maps a sharing-strategy label value to its
// sharing mode, unknown when unrecognized.
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

// getNodeGPULabels returns this node's labels, cached for
// nodeGPULabelsCacheDuration: the read goes to the cluster agent or the
// API server, and the labels change rarely.
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

// countReplicas fills ts.replicas from the kubelet's registered devices
// (one per replica), falling back to the node's replicas label.
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
		if labels, err := c.getNodeGPULabels(ctx); err == nil {
			if n, err := strconv.Atoi(labels[gfdReplicasLabel]); err == nil && n > 0 {
				ts.replicas[base] = n
			}
		}
	}
}

// countRegisteredReplicas counts, per base device, the registered IDs
// carrying its replica suffix: one registered device per replica.
func countRegisteredReplicas(ts timeSlices, registeredIDs []string) {
	for _, id := range registeredIDs {
		if replica, ok := gpuutil.ParseNvidiaDeviceReplica(id); ok {
			ts.replicas[replica.BaseDeviceID]++
		}
	}
}

// gpuSharingInfo returns the sharing mode and replica count of the given
// GPU, if a replica allocation references it.
func (ts timeSlices) gpuSharingInfo(uuid string) (workloadmeta.GPUSharingMode, int, bool) {
	mode, shared := ts.sharedGPUs[uuid]
	if !shared {
		return "", 0, false
	}
	return mode, ts.replicas[uuid], true
}

// containerReplicaUUIDs returns the base device UUIDs of the given
// container's replicas, sorted for a stable GPUDeviceIDs order.
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
