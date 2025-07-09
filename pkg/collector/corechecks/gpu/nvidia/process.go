// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"math"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type apiCallInfo struct {
	name     string
	testFunc func(ddnvml.Device) error
	callFunc func(*processCollector) ([]Metric, error)
}

var apiCallFactory = []apiCallInfo{
	{
		name: "memory_usage",
		testFunc: func(d ddnvml.Device) error {
			_, err := d.GetComputeRunningProcesses()
			return err
		},
		callFunc: (*processCollector).collectComputeProcesses,
	},
	{
		name: "process_utilization",
		testFunc: func(d ddnvml.Device) error {
			_, err := d.GetProcessUtilization(0)
			return err
		},
		callFunc: (*processCollector).collectProcessUtilization,
	},
}

type processCollector struct {
	device            ddnvml.Device
	lastTimestamp     uint64
	supportedApiCalls []apiCallInfo
}

func newProcessCollector(device ddnvml.Device) (Collector, error) {
	c := &processCollector{device: device}

	c.removeUnsupportedMetrics()
	if len(c.supportedApiCalls) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *processCollector) removeUnsupportedMetrics() {
	for _, apiCall := range apiCallFactory {
		err := apiCall.testFunc(c.device)
		if err == nil || !ddnvml.IsUnsupported(err) {
			c.supportedApiCalls = append(c.supportedApiCalls, apiCall)
		}
	}
}

func (c *processCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

func (c *processCollector) Name() CollectorName {
	return process
}

func (c *processCollector) Collect2() ([]Metric, error) {
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

func (c *processCollector) Collect() ([]Metric, error) {
	var allMetrics []Metric
	var multiErr error

	for _, apiCall := range c.supportedApiCalls {
		collectedMetrics, err := apiCall.callFunc(c)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("failed to call %s: %w", apiCall.name, err))
			continue
		}

		allMetrics = append(allMetrics, collectedMetrics...)
	}

	return allMetrics, multiErr
}

// Helper methods for metric collection
// memory.usage and memory.limit metrics gets higher priority from process collector than from ebpf collector
func (c *processCollector) collectComputeProcesses() ([]Metric, error) {
	procs, err := c.device.GetComputeRunningProcesses()
	if err != nil {
		return nil, err
	}

	devInfo := c.device.GetDeviceInfo()
	var processMetrics []Metric
	var allPidTags []string

	// Collect per-process memory.usage metrics and aggregate PID tags
	for _, proc := range procs {
		pidTag := []string{fmt.Sprintf("pid:%d", proc.Pid)}
		// Only emit memory.usage per process
		processMetrics = append(processMetrics,
			Metric{Name: "memory.usage", Value: float64(proc.UsedGpuMemory), Type: metrics.GaugeType, Priority: 1, Tags: pidTag},
		)
		// Collect PID tags for aggregated limit metrics
		allPidTags = append(allPidTags, fmt.Sprintf("pid:%d", proc.Pid))
	}

	// Emit memory.limit once per device with all PID tags
	processMetrics = append(processMetrics,
		Metric{Name: "memory.limit", Value: float64(devInfo.Memory), Type: metrics.GaugeType, Priority: 1, Tags: allPidTags},
	)

	return processMetrics, nil
}

func (c *processCollector) collectProcessUtilization() ([]Metric, error) {
	processSamples, err := c.device.GetProcessUtilization(c.lastTimestamp)
	if err != nil {
		return nil, err
	}

	var utilizationMetrics []Metric
	var allPidTags []string

	// Collect per-process utilization metrics and aggregate PID tags
	for _, sample := range processSamples {
		pidTag := []string{fmt.Sprintf("pid:%d", sample.Pid)}
		utilizationMetrics = append(utilizationMetrics,
			Metric{Name: "core.usage", Value: float64(sample.SmUtil), Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "dram_active", Value: float64(sample.MemUtil), Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "encoder_utilization", Value: float64(sample.EncUtil), Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "decoder_utilization", Value: float64(sample.DecUtil), Type: metrics.GaugeType, Tags: pidTag},
		)

		// Collect PID tags for aggregated limit metrics
		allPidTags = append(allPidTags, fmt.Sprintf("pid:%d", sample.Pid))

		//update the last timestamp if the current sample's timestamp is greater
		if sample.TimeStamp > c.lastTimestamp {
			c.lastTimestamp = sample.TimeStamp
		}
	}

	// Emit core.limit once per device with all PID tags
	devInfo := c.device.GetDeviceInfo()
	utilizationMetrics = append(utilizationMetrics,
		Metric{Name: "core.limit", Value: float64(devInfo.CoreCount), Type: metrics.GaugeType, Tags: allPidTags},
	)

	return utilizationMetrics, nil
}
