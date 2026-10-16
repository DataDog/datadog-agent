// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// notAllocatableTag is the unavailable_reason of a GPU the Kubernetes device
	// plugin no longer advertises as allocatable.
	notAllocatableTag = "unavailable_reason:not_allocatable"

	// replicaSeparator separates a device ID from its replica number in the IDs
	// the NVIDIA device plugin advertises for time-sliced devices (<id>::<replica>).
	// See AnnotatedID in https://github.com/NVIDIA/k8s-device-plugin/blob/main/internal/rm/devices.go
	replicaSeparator = "::"

	allocatableQueryTimeout = 2 * time.Second
)

// errAllocatableNotApplicable means the host cannot report allocatable devices
// (not Kubernetes, or no PodResources socket). It is the normal case on bare
// metal, so callers must not log it.
var errAllocatableNotApplicable = errors.New("kubelet allocatable devices are not available on this host")

// allocatableIDsProvider returns the IDs of the devices the kubelet reports as allocatable.
type allocatableIDsProvider func(ctx context.Context) ([]string, error)

// allocatableGPU is a physical GPU as seen by the device plugin: the GPU itself
// plus the MIG slices that can be advertised in its place.
type allocatableGPU struct {
	uuid       string
	migEnabled bool
	migUUIDs   []string
}

// notAllocatableByGPU maps the physical GPU UUID to whether the device plugin has
// marked the GPU unhealthy, given the device IDs the kubelet reports as
// allocatable. It returns nil when no GPU can be tied to an allocatable ID: that
// is either "no device plugin on this host" (DRA, bare metal) or "every GPU is
// unhealthy" and the two cannot be told apart.
//
// Otherwise a GPU is not allocatable when it is advertised neither as a whole
// GPU (also covers time-slicing replicas and MIG strategy "none") nor, for MIG,
// through all of its slices. This assumes the device plugin manages every GPU
// on the node.
//
// IDs that match no known GPU or slice are ignored.
// TODO: support DEVICE_ID_STRATEGY=index, where IDs are GPU indexes instead of UUIDs.
func notAllocatableByGPU(gpus []allocatableGPU, allocatableIDs []string) map[string]bool {
	allocatable := make(map[string]struct{}, len(allocatableIDs))
	for _, id := range allocatableIDs {
		id, _, _ = strings.Cut(id, replicaSeparator)
		allocatable[strings.ToLower(id)] = struct{}{}
	}
	has := func(uuid string) bool {
		_, ok := allocatable[strings.ToLower(uuid)]
		return ok
	}

	managed := false
	result := make(map[string]bool, len(gpus))
	for _, gpu := range gpus {
		whole := has(gpu.uuid)
		missingSlice := false
		anySlice := false
		for _, migUUID := range gpu.migUUIDs {
			if has(migUUID) {
				anySlice = true
			} else {
				missingSlice = true
			}
		}
		managed = managed || whole || anySlice

		switch {
		case whole:
			result[gpu.uuid] = false
		case len(gpu.migUUIDs) > 0:
			result[gpu.uuid] = missingSlice
		case gpu.migEnabled:
			// MIG enabled without slices: there is nothing the plugin could advertise
			result[gpu.uuid] = false
		default:
			result[gpu.uuid] = true
		}
	}

	if !managed {
		return nil
	}
	return result
}

// kubeletAllocatableDeviceIDs returns the device IDs the kubelet reports as allocatable.
func (c *Check) kubeletAllocatableDeviceIDs(ctx context.Context) ([]string, error) {
	if !env.IsFeaturePresent(env.Kubernetes) || !env.IsFeaturePresent(env.PodResources) {
		return nil, errAllocatableNotApplicable
	}
	nodeInfo := c.getNodeInfo()
	if nodeInfo == nil {
		return nil, errors.New("node info is not available")
	}
	return nodeInfo.GetAllocatableDeviceIDs(ctx)
}

// emitNotAllocatable emits gpu.device.unavailable{unavailable_reason:not_allocatable}
// for every physical GPU, or nothing when the allocatable devices cannot be determined.
func (c *Check) emitNotAllocatable(snd sender.Sender, gpuToContainersMap map[string][]*workloadmeta.Container, gpuToProcessesMap map[string][]int32, currentExecutionTime time.Time) error {
	if c.allocatableIDsFn == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), allocatableQueryTimeout)
	defer cancel()

	allocatableIDs, err := c.allocatableIDsFn(ctx)
	if err != nil {
		if !errors.Is(err, errAllocatableNotApplicable) && logLimitCheck.ShouldLog() {
			log.Debugf("not reporting %s: cannot get the allocatable devices from the kubelet: %v", notAllocatableTag, err)
		}
		return nil
	}

	devices, err := c.deviceCache.AllPhysicalDevices()
	if err != nil {
		return fmt.Errorf("failed to retrieve physical devices: %w", err)
	}
	gpus := make([]allocatableGPU, 0, len(devices))
	for _, device := range devices {
		physical, ok := device.(*ddnvml.PhysicalDevice)
		if !ok {
			continue
		}
		gpu := allocatableGPU{uuid: physical.GetDeviceInfo().UUID, migEnabled: physical.HasMIGFeatureEnabled}
		for _, child := range physical.MIGChildren {
			gpu.migUUIDs = append(gpu.migUUIDs, child.GetDeviceInfo().UUID)
		}
		gpus = append(gpus, gpu)
	}

	var multiErr []error
	notAllocatable := notAllocatableByGPU(gpus, allocatableIDs)
	for _, gpu := range gpus {
		value, ok := notAllocatable[gpu.uuid]
		if !ok || c.isDeviceExcluded(gpu.uuid) {
			continue
		}
		sample := nvidia.NewMetric("device.unavailable", boolToFloat(value), metrics.GaugeType, nvidia.Low, []string{notAllocatableTag}, nil)
		if err := c.emitSample(sample, snd, currentExecutionTime, gpuToContainersMap[gpu.uuid], gpuToProcessesMap[gpu.uuid], c.deviceTags[gpu.uuid]); err != nil {
			multiErr = append(multiErr, fmt.Errorf("error emitting sample %s: %w", sample.Key(), err))
		}
	}
	return errors.Join(multiErr...)
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
