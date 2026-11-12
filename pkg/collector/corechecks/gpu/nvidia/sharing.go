// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	// sharingFractionMetricName is the metric holding the fraction of a device
	// assigned to a container.
	sharingFractionMetricName = "device.sharing_fraction"
	// sharingMemoryMetricName is the metric holding the memory of a device
	// that a container is allowed to use.
	sharingMemoryMetricName = "device.sharing_memory"
	// sharingReplicasMetricName is the metric holding the number of device
	// replicas the device plugin registered of a device shared by
	// replication: how many workloads can share it.
	sharingReplicasMetricName = "device.sharing_replicas"
	// sharingReplicasUsedMetricName is the metric holding the number of
	// replicas of a device shared by replication that a container holds. A
	// container can hold several of them, e.g. requesting two shared GPUs
	// that are both replicas of one.
	sharingReplicasUsedMetricName = "device.sharing_replicas_used"
)

// sharingCollector reports the share of its device assigned to each container
// that shares it with other workloads: the fraction and the memory of it, as
// the NVML workloadmeta collector publishes them in
// workloadmeta.Container.GPUShares for containers sharing a device by a
// mechanism that assigns each of them a share, such as KAI Scheduler GPU
// fractions; and the number of device replicas each container holds, as
// published in workloadmeta.Container.GPUDeviceReplicas for containers
// holding replicas of a device shared by replication (time-slicing, MPS).
type sharingCollector struct {
	device ddnvml.Device
	wmeta  workloadmeta.Component
}

func newSharingCollector(device ddnvml.Device, deps *CollectorDependencies) (Collector, error) {
	if deps.Workloadmeta == nil {
		return nil, errors.New("workloadmeta cannot be nil")
	}

	return &sharingCollector{
		device: device,
		wmeta:  deps.Workloadmeta,
	}, nil
}

// Name returns the collector name.
func (c *sharingCollector) Name() CollectorName {
	return sharing
}

// Device returns the device this collector monitors.
func (c *sharingCollector) Device() ddnvml.Device {
	return c.device
}

// Collect returns, for each container sharing the device, samples with its
// share of it, associated with that container only, so that each share is
// attributable to one container. Containers without a known share get
// nothing: they either have the device to themselves, or share it by
// replication, whose per-container quantity -- the number of replicas held --
// is reported instead. The sharing factor of the device itself, when it is
// shared by replication, is reported as one sample on the device's tags.
func (c *sharingCollector) Collect() ([]Sample, error) {
	deviceUUID := c.device.GetDeviceInfo().UUID

	// The device's sharing factor: how many replicas the device plugin
	// registered of it. A device-tags-only sample: the factor describes the
	// device, not any of its workloads.
	var samples []Sample
	if gpu, err := c.wmeta.GetGPU(deviceUUID); err == nil && gpu.SharingReplicas > 0 {
		samples = append(samples, NewMetricWithNoWorkloadTags(sharingReplicasMetricName, float64(gpu.SharingReplicas), metrics.GaugeType, Low, nil))
	}

	for _, container := range c.wmeta.ListContainersWithFilter(hasGPUSharing) {
		workloads := []workloadmeta.EntityID{container.EntityID}

		if share := container.GPUShares[deviceUUID]; share.Fraction > 0 || share.Memory > 0 {
			if share.Fraction > 0 {
				samples = append(samples, NewMetric(sharingFractionMetricName, share.Fraction, metrics.GaugeType, Low, nil, workloads))
			}
			if share.Memory > 0 {
				samples = append(samples, NewMetric(sharingMemoryMetricName, float64(share.Memory), metrics.GaugeType, Low, nil, workloads))
			}
		}

		if replicas, shared := container.GPUDeviceReplicas[deviceUUID]; shared && replicas > 0 {
			samples = append(samples, NewMetric(sharingReplicasUsedMetricName, float64(replicas), metrics.GaugeType, Low, nil, workloads))
		}
	}
	return samples, nil
}

func hasGPUSharing(container *workloadmeta.Container) bool {
	return len(container.GPUShares) > 0 || len(container.GPUDeviceReplicas) > 0
}
