// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"math"

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
	supportedAPICalls []apiCallInfo
}

func newProcessCollector(device ddnvml.Device) (Collector, error) {
	c := &processCollector{device: device}

	c.removeUnsupportedMetrics()
	if len(c.supportedAPICalls) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *processCollector) removeUnsupportedMetrics() {
	for _, apiCall := range apiCallFactory {
		err := apiCall.testFunc(c.device)
		if err == nil || !ddnvml.IsUnsupported(err) {
			c.supportedAPICalls = append(c.supportedAPICalls, apiCall)
		}
	}
}

func (c *processCollector) DeviceUUID() string {
	return c.device.GetDeviceInfo().UUID
}

func (c *processCollector) Name() CollectorName {
	return process
}

func (c *processCollector) Collect() ([]Metric, error) {
	var allMetrics []Metric
	var multiErr error

	for _, apiCall := range c.supportedAPICalls {
		collectedMetrics, err := apiCall.callFunc(c)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("%s returned an error: %w", apiCall.name, err))
		}

		allMetrics = append(allMetrics, collectedMetrics...)
	}

	return allMetrics, multiErr
}

// Helper methods for metric collection
// memory.usage and memory.limit metrics gets higher priority from process collector than from ebpf collector
func (c *processCollector) collectComputeProcesses() ([]Metric, error) {
	var processMetrics []Metric
	var allPidTags []string

	procs, err := c.device.GetComputeRunningProcesses()
	// we don't check for error here, as the loop simply will be skipped.
	for _, proc := range procs {
		pidTag := fmt.Sprintf("pid:%d", proc.Pid)
		// Only emit memory.usage per process
		processMetrics = append(processMetrics,
			Metric{Name: "memory.usage", Value: float64(proc.UsedGpuMemory), Type: metrics.GaugeType, Priority: 10, Tags: []string{pidTag}},
		)
		// Collect PID tags for aggregated limit metrics
		allPidTags = append(allPidTags, pidTag)
	}

	devInfo := c.device.GetDeviceInfo()
	processMetrics = append(processMetrics,
		Metric{Name: "memory.limit", Value: float64(devInfo.Memory), Type: metrics.GaugeType, Priority: 10, Tags: allPidTags},
	)

	return processMetrics, err // Return the original error if there was one
}

func (c *processCollector) collectProcessUtilization() (utilizationMetrics []Metric, err error) {
	var allPidTags []string
	var totalSmUtil, maxSmUtil float64

	coreCount := c.device.GetDeviceInfo().CoreCount

	// Defer function to ensure global metrics are always emitted
	defer func() {
		// Calculate sm_active using median approach: median(max, min(sum, 100))
		cappedSum := math.Min(totalSmUtil, 100.0)
		smActive := (maxSmUtil + cappedSum) / 2.0

		utilizationMetrics = append(utilizationMetrics,
			Metric{Name: "core.limit", Value: float64(coreCount), Type: metrics.GaugeType, Tags: allPidTags},
			Metric{Name: "sm_active", Value: smActive, Type: metrics.GaugeType},
		)
	}()

	processSamples, err := c.device.GetProcessUtilization(c.lastTimestamp)
	if err != nil {
		var nvmlErr *ddnvml.NvmlAPIError
		// Handle ERROR_NOT_FOUND as a valid scenario when no process utilization data is available
		if errors.As(err, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_NOT_FOUND) {
			err = nil // Clear the error for NOT_FOUND case
		}
		return // Return with either nil (NOT_FOUND) or original error (other cases)
	}
	for _, sample := range processSamples {
		pidTag := []string{fmt.Sprintf("pid:%d", sample.Pid)}
		smUtil := float64(sample.SmUtil)

		utilizationMetrics = append(utilizationMetrics,
			Metric{Name: "process.sm_active", Value: smUtil, Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "process.dram_active", Value: float64(sample.MemUtil), Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "process.encoder_utilization", Value: float64(sample.EncUtil), Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "process.decoder_utilization", Value: float64(sample.DecUtil), Type: metrics.GaugeType, Tags: pidTag},
		)

		// Collect PID tags for aggregated metrics
		allPidTags = append(allPidTags, fmt.Sprintf("pid:%d", sample.Pid))

		// Track sm_active calculation variables
		totalSmUtil += smUtil
		if smUtil > maxSmUtil {
			maxSmUtil = smUtil
		}

		//update the last timestamp if the current sample's timestamp is greater
		if sample.TimeStamp > c.lastTimestamp {
			c.lastTimestamp = sample.TimeStamp
		}
	}

	return
}
