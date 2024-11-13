// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvidia

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

const clocksCollectorName = "clocks"
const clocksMetricsPrefix = "clock_throttle_reasons"

// clocksCollector collects clock metrics from an NVML device.
type clocksCollector struct {
	device nvml.Device
	tags   []string
}

// newClocksCollector creates a new clocksMetricsCollector for the given NVML device.
func newClocksCollector(_ nvml.Interface, device nvml.Device, tags []string) (Collector, error) {
	// Check first if the device supports clock throttle reasons
	_, ret := device.GetCurrentClocksThrottleReasons()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return nil, errUnsupportedDevice
	} else if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("cannot check clock throttle reasons support: %s", nvml.ErrorString(ret))
	}

	return &clocksCollector{
		device: device,
		tags:   tags,
	}, nil
}

// Collect collects clock throttle reason metrics from the NVML device.
func (c *clocksCollector) Collect() ([]Metric, error) {
	allReasons, ret := c.device.GetCurrentClocksThrottleReasons()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("cannot get throttle reasons: %s", nvml.ErrorString(ret))
	}

	metrics := make([]Metric, 0, len(allThrottleReasons))
	for name, bit := range allThrottleReasons {
		value := boolToFloat((allReasons & bit) != 0)
		metric := Metric{
			Name:  fmt.Sprintf("%s.%s", clocksMetricsPrefix, name),
			Value: value,
			Tags:  c.tags,
		}
		metrics = append(metrics, metric)
	}

	// Return the collected metrics
	return metrics, nil
}

// Close closes the collector and releases any resources it might have allocated (no-op for this collector).
func (c *clocksCollector) Close() error {
	return nil
}

// Name returns the name of the collector.
func (c *clocksCollector) Name() string {
	return clocksCollectorName
}

var allThrottleReasons = map[string]uint64{
	"gpu_idle":                    nvml.ClocksEventReasonGpuIdle,
	"applications_clocks_setting": nvml.ClocksEventReasonApplicationsClocksSetting,
	"sw_power_cap":                nvml.ClocksEventReasonSwPowerCap,
	"sync_boost":                  nvml.ClocksEventReasonSyncBoost,
	"sw_thermal_slowdown":         nvml.ClocksEventReasonSwThermalSlowdown,
	"display_clock_setting":       nvml.ClocksEventReasonDisplayClockSetting,
	"none":                        nvml.ClocksEventReasonNone,
}
