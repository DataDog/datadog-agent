// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"math"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type processCollector struct {
	device        ddnvml.SafeDevice
	lastTimestamp uint64
}

func newProcessCollector(device ddnvml.SafeDevice) (Collector, error) {
	c := &processCollector{device: device}

	// Test if GetProcessUtilization is supported
	_, err := device.GetProcessUtilization(0)
	if err != nil && ddnvml.IsUnsupported(err) {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *processCollector) DeviceUUID() string {
	uuid, _ := c.device.GetUUID()
	return uuid
}

func (c *processCollector) Name() CollectorName {
	return process
}

func (c *processCollector) Collect() ([]Metric, error) {
	processSamples, err := c.device.GetProcessUtilization(c.lastTimestamp)
	if err != nil {
		// Handle ERROR_NOT_FOUND as a valid scenario when no process utilization data is available
		var nvmlErr *ddnvml.NvmlAPIError
		if errors.As(err, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_NOT_FOUND) {
			// No process data available, return 0 for sm_active
			return []Metric{
				{Name: "sm_active", Value: 0.0, Type: metrics.GaugeType},
			}, nil
		}
		return nil, err
	}

	if len(processSamples) == 0 {
		// No processes running, return 0 for sm_active
		return []Metric{
			{Name: "sm_active", Value: 0.0, Type: metrics.GaugeType},
		}, nil
	}

	// Calculate sm_active using median approach
	var sum, maxSm float64
	for _, sample := range processSamples {
		smUtil := float64(sample.SmUtil)
		sum += smUtil
		if smUtil > maxSm {
			maxSm = smUtil
		}
		// Update timestamp
		if sample.TimeStamp > c.lastTimestamp {
			c.lastTimestamp = sample.TimeStamp
		}
	}

	// Apply your formula: median(max, min(sum, 100))
	cappedSum := math.Min(sum, 100.0)
	smActive := (maxSm + cappedSum) / 2.0

	return []Metric{
		{Name: "sm_active", Value: smActive, Type: metrics.GaugeType},
	}, nil
}
