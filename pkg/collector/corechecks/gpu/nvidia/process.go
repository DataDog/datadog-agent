// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	c := &processCollector{device: device,
		lastTimestamp: uint64(time.Now().Unix())}

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
	log.Debugf("GetComputeRunningProcesses returned %d processes with error: %v", len(procs), err)
	if err == nil {
		for _, proc := range procs {
			pidTag := fmt.Sprintf("pid:%d", proc.Pid)
			// Only emit memory.usage per process
			processMetrics = append(processMetrics,
				Metric{
					Name:     "memory.usage",
					Value:    float64(proc.UsedGpuMemory),
					Type:     metrics.GaugeType,
					Priority: 10,
					Tags:     []string{pidTag},
				},
			)
			// Collect PID tags for aggregated limit metrics
			allPidTags = append(allPidTags, pidTag)
		}
	}
	devInfo := c.device.GetDeviceInfo()
	processMetrics = append(processMetrics,
		Metric{
			Name:     "memory.limit",
			Value:    float64(devInfo.Memory),
			Type:     metrics.GaugeType,
			Priority: 10,
			Tags:     allPidTags,
		},
	)

	return processMetrics, err // Return the original error if there was one
}

func (c *processCollector) collectProcessUtilization() ([]Metric, error) {
	var allPidTags []string
	var allMetrics []Metric
	var err error
	var maxSmUtil, sumSmUtil uint32

	// Record timestamp before API call to ensure accurate sampling window
	currentTimestamp := uint64(time.Now().Unix())
	processSamples, err := c.device.GetProcessUtilization(c.lastTimestamp)
	log.Debugf("GetProcessUtilization returned %d samples with error: %v", len(processSamples), err)
	// Update timestamp regardless of whether processes are found
	c.lastTimestamp = currentTimestamp

	// Handle ERROR_NOT_FOUND as a valid scenario when no process utilization data is available
	if err != nil {
		var nvmlErr *ddnvml.NvmlAPIError
		if errors.As(err, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_NOT_FOUND) {
			err = nil // Clear the error for NOT_FOUND case
		}
	} else {
		for _, sample := range processSamples {
			pidTag := []string{fmt.Sprintf("pid:%d", sample.Pid)}

			allMetrics = append(allMetrics,
				Metric{
					Name:  "process.sm_active",
					Value: float64(sample.SmUtil),
					Type:  metrics.GaugeType,
					Tags:  pidTag,
				},
				Metric{
					Name:  "process.dram_active",
					Value: float64(sample.MemUtil),
					Type:  metrics.GaugeType,
					Tags:  pidTag,
				},
				Metric{
					Name:  "process.encoder_utilization",
					Value: float64(sample.EncUtil),
					Type:  metrics.GaugeType,
					Tags:  pidTag,
				},
				Metric{
					Name:  "process.decoder_utilization",
					Value: float64(sample.DecUtil),
					Type:  metrics.GaugeType,
					Tags:  pidTag,
				},
			)

			// Track SM utilization for device-wide calculation
			if sample.SmUtil > maxSmUtil {
				maxSmUtil = sample.SmUtil
			}
			sumSmUtil += sample.SmUtil
			// Collect PID tags for aggregated metrics
			allPidTags = append(allPidTags, fmt.Sprintf("pid:%d", sample.Pid))
		}

	}

	// Emit device-wide sm_active metric using average of max and sum capped at 100
	if sumSmUtil > 100 {
		sumSmUtil = 100
	}
	deviceSmActive := float64(maxSmUtil+sumSmUtil) / 2.0

	allMetrics = append(allMetrics,
		Metric{
			Name:  "sm_active",
			Value: deviceSmActive,
			Type:  metrics.GaugeType,
		},
	)

	allMetrics = append(allMetrics,
		Metric{
			Name:  "core.limit",
			Value: float64(c.device.GetDeviceInfo().CoreCount),
			Type:  metrics.GaugeType,
			Tags:  allPidTags,
		},
	)

	return allMetrics, err
}
