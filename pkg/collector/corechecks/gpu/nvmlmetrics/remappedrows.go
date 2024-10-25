// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvmlmetrics

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const remappedRowsCollectorName = "remapped_rows"
const remappedRowsMetricPrefix = "remapped_rows"

type remappedRowsCollector struct {
	device nvml.Device
	tags   []string
}

// newRemappedRowsCollector creates a new remappedRowsMetricsCollector for the given NVML device.
func newRemappedRowsCollector(_ nvml.Interface, device nvml.Device, tags []string) (Collector, error) {
	return &remappedRowsCollector{
		device: device,
		tags:   tags,
	}, nil
}

// Collect collects remapped rows metrics from the NVML device.
func (c *remappedRowsCollector) Collect() ([]Metric, error) {
	// Collect remapped rows metrics from the NVML device
	correctable, uncorrectable, pending, failed, ret := c.device.GetRemappedRows()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("cannot get remapped rows: %s", nvml.ErrorString(ret))
	}

	metrics := []Metric{
		{Name: fmt.Sprintf("%s.correctable", remappedRowsMetricPrefix), Value: float64(correctable), Tags: c.tags},
		{Name: fmt.Sprintf("%s.uncorrectable", remappedRowsMetricPrefix), Value: float64(uncorrectable), Tags: c.tags},
		{Name: fmt.Sprintf("%s.pending", remappedRowsMetricPrefix), Value: boolToFloat(pending), Tags: c.tags},
		{Name: fmt.Sprintf("%s.failed", remappedRowsMetricPrefix), Value: boolToFloat(failed), Tags: c.tags},
	}

	return metrics, nil
}

// Close closes the collector and releases any resources it might have allocated (no-op for this collector).
func (c *remappedRowsCollector) Close() error {
	return nil
}

// Name returns the name of the collector.
func (c *remappedRowsCollector) Name() string {
	return remappedRowsCollectorName
}
