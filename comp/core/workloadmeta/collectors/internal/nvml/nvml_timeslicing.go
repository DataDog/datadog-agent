// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"context"
	"strconv"
	"strings"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// GPU Feature Discovery labels each resource as "<resource name><suffix>",
	// e.g. nvidia.com/gpu.replicas or, with the mixed MIG strategy,
	// nvidia.com/mig-1g.10gb.replicas: each resource has its own replica
	// count. gfdSharingStrategySuffix holds the sharing strategy,
	// "time-slicing" or "mps": the only way to tell them apart, as both use
	// the same "::<n>" replica device IDs.
	gfdSharingStrategySuffix = ".sharing-strategy"
	gfdReplicasSuffix        = ".replicas"
	// sharedResourceSuffix is the suffix the device plugin's renameByDefault
	// option adds to the shared resources. GPU Feature Discovery labels them
	// under their original name.
	sharedResourceSuffix = ".shared"

	// nodeGPULabelsRefreshInterval is how often the node labels are read: the
	// GPU Feature Discovery default relabeling interval, so a cached label is
	// at most one relabeling behind.
	nodeGPULabelsRefreshInterval = time.Minute
)

// timeSlices holds the shared NVIDIA device replicas (time-slicing, MPS)
// allocated on this node.
type timeSlices struct {
	// byContainer holds, per container ID, the replica count of each base
	// device UUID it holds.
	byContainer map[string]map[string]int
	// shared holds the sharing of each base device UUID a replica allocation
	// references.
	shared map[string]gpuSharing
}

// gpuSharing is how a device shared by replication is shared, as labeled for
// the resource it is allocated as.
type gpuSharing struct {
	mode     workloadmeta.GPUSharingMode
	replicas int
}

// resolveTimeSlices scans the containers' kubelet-allocated resources for
// replica allocations (device IDs suffixed with "::<n>"), grouped by base
// device, and reads each device's sharing mode and replica count from the
// node labels of its resource.
func (c *collector) resolveTimeSlices(ctx context.Context) timeSlices {
	ts := timeSlices{
		byContainer: make(map[string]map[string]int),
		shared:      make(map[string]gpuSharing),
	}
	// resources holds the resource name each shared base device is labeled
	// under.
	resources := make(map[string]string)

	for _, container := range c.store.ListContainers() {
		for _, resource := range container.ResolvedAllocatedResources {
			if !gpuutil.IsNvidiaKubernetesResource(resource.Name) {
				continue
			}
			replica, ok := gpuutil.ParseNvidiaDeviceReplica(resource.ID)
			if !ok {
				continue // an exclusive allocation
			}
			if ts.byContainer[container.ID] == nil {
				ts.byContainer[container.ID] = make(map[string]int)
			}
			// A container can hold several replicas of the same base device.
			ts.byContainer[container.ID][replica.BaseDeviceID]++
			resources[replica.BaseDeviceID] = strings.TrimSuffix(resource.Name, sharedResourceSuffix)
		}
	}

	// Read the node labels only when a replica is allocated, so a node
	// without shared GPUs pays nothing for the lookup.
	if len(resources) == 0 {
		return ts
	}
	labels := c.getNodeGPULabels(ctx)
	for uuid, resource := range resources {
		sharing := gpuSharing{mode: sharingModeFromStrategyLabel(labels[resource+gfdSharingStrategySuffix])}
		if n, err := strconv.Atoi(labels[resource+gfdReplicasSuffix]); err == nil && n > 0 {
			sharing.replicas = n
		}
		ts.shared[uuid] = sharing
	}
	return ts
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

// getNodeGPULabels returns this node's labels, read at most once per
// nodeGPULabelsRefreshInterval. A failed read keeps the last labels read.
func (c *collector) getNodeGPULabels(ctx context.Context) map[string]string {
	if !c.nodeGPULabelsAt.IsZero() && time.Since(c.nodeGPULabelsAt) < nodeGPULabelsRefreshInterval {
		return c.nodeGPULabels
	}
	c.nodeGPULabelsAt = time.Now()

	labels, err := c.fetchNodeLabels(ctx)
	if err != nil {
		log.Debugf("cannot read the node labels to resolve the GPU sharing strategy: %v", err)
		return c.nodeGPULabels
	}
	c.nodeGPULabels = labels
	return labels
}
