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
)

// sharingCollector reports the share of its device assigned to each container
// that shares it with other workloads, as the NVML workloadmeta collector
// publishes it in workloadmeta.Container.GPUShares: its fraction of the device
// and the memory it is allowed to use.
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

// Collect returns, for each container assigned a share of the device, samples
// with its fraction and its memory, associated with that container only, so
// that each share is attributable to one container. Containers without a known
// share get nothing: whether they have the device to themselves or share it
// some other way, such as time-slicing, cannot be told apart reliably.
func (c *sharingCollector) Collect() ([]Sample, error) {
	deviceUUID := c.device.GetDeviceInfo().UUID

	var samples []Sample
	for _, container := range c.wmeta.ListContainersWithFilter(hasGPUShares) {
		share := container.GPUShares[deviceUUID]
		workloads := []workloadmeta.EntityID{container.EntityID}
		if share.Fraction > 0 {
			samples = append(samples, NewMetric(sharingFractionMetricName, share.Fraction, metrics.GaugeType, Low, nil, workloads))
		}
		if share.Memory > 0 {
			samples = append(samples, NewMetric(sharingMemoryMetricName, float64(share.Memory), metrics.GaugeType, Low, nil, workloads))
		}
	}
	return samples, nil
}

func hasGPUShares(container *workloadmeta.Container) bool {
	return len(container.GPUShares) > 0
}
