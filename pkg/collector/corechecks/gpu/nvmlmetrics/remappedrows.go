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

const remappedRowsMetricsCollectorName = "remapped_rows"
const remappedRowsMetrixPrefix = "remapped_rows"

type remappedRowsMetricsCollector struct {
	device nvml.Device
	tags   []string
}

// newRemappedRowsMetricsCollector creates a new remappedRowsMetricsCollector for the given NVML device.
func newRemappedRowsMetricsCollector(_ nvml.Interface, device nvml.Device, tags []string) (Collector, error) {
	return &remappedRowsMetricsCollector{
		device: device,
		tags:   tags,
	}, nil
}

// Collect collects remapped rows metrics from the NVML device.
func (c *remappedRowsMetricsCollector) Collect() ([]Metric, error) {
	// Collect remapped rows metrics from the NVML device
	correctable, uncorrectable, pending, failed, ret := c.device.GetRemappedRows()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("cannot get remapped rows: %s", nvml.ErrorString(ret))
	}

	metrics := []Metric{
		{Name: fmt.Sprintf("%s.correctable", remappedRowsMetrixPrefix), Value: float64(correctable), Tags: c.tags},
		{Name: fmt.Sprintf("%s.uncorrectable", remappedRowsMetrixPrefix), Value: float64(uncorrectable), Tags: c.tags},
		{Name: fmt.Sprintf("%s.pending", remappedRowsMetrixPrefix), Value: boolToFloat(pending), Tags: c.tags},
		{Name: fmt.Sprintf("%s.failed", remappedRowsMetrixPrefix), Value: boolToFloat(failed), Tags: c.tags},
	}

	return metrics, nil
}

// Close closes the collector and releases any resources it might have allocated (no-op for this collector).
func (c *remappedRowsMetricsCollector) Close() error {
	return nil
}

// Name returns the name of the collector.
func (c *remappedRowsMetricsCollector) Name() string {
	return remappedRowsMetricsCollectorName
}
