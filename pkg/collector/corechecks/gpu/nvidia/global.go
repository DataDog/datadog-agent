// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// globalCollector is responsible for emitting global metrics (like device.total)
// but in the context of a specific device for tagging purposes.
type globalCollector struct {
	deviceUUID       string
	totalDeviceCount int
}

// newGlobalCollector creates a new collector that reports the total device count,
// associated with a specific deviceUUID for tagging.
func newGlobalCollector(deviceUUID string, totalDeviceCount int) (Collector, error) {
	return &globalCollector{
		deviceUUID:       deviceUUID,
		totalDeviceCount: totalDeviceCount,
	}, nil
}

// Name returns the name of the collector.
func (c *globalCollector) Name() CollectorName {
	return global // Use the constant 'global' defined in collector.go
}

// DeviceUUID returns the UUID of the device this collector is associated with for tagging.
func (c *globalCollector) DeviceUUID() string {
	return c.deviceUUID
}

// Collect emits the "device.total" metric.
func (c *globalCollector) Collect() ([]Metric, error) {
	return []Metric{
		{
			Name:  "device.total",
			Value: float64(c.totalDeviceCount),
			Type:  metrics.GaugeType,
		},
	}, nil
}
