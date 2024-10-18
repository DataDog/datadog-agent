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

const clocksMetricsCollectorName = "clocks"
const clocksMetricsPrefix = "clock_throttle_reasons"

// clocksMetricsCollector collects clock metrics from an NVML device.
type clocksMetricsCollector struct {
	device nvml.Device
	tags   []string
}

// newClocksMetricsCollector creates a new clocksMetricsCollector for the given NVML device.
func newClocksMetricsCollector(_ nvml.Interface, device nvml.Device, tags []string) (Collector, error) {
	return &clocksMetricsCollector{
		device: device,
		tags:   tags,
	}, nil
}

// Collect collects clock throttle reason metrics from the NVML device.
func (coll *clocksMetricsCollector) Collect() ([]Metric, error) {
	reasons, ret := coll.device.GetCurrentClocksThrottleReasons()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("cannot get throttle reasons: %s", nvml.ErrorString(ret))
	}

	metrics := make([]Metric, 0, len(allThrottleReasons))
	for reasonName, reasonBit := range allThrottleReasons {
		name, bit := reasonName, reasonBit
		value := float64((reasons & bit) >> bit)
		metric := Metric{
			Name:  fmt.Sprintf("%s.%s", clocksMetricsPrefix, name),
			Value: value,
			Tags:  coll.tags,
		}
		metrics = append(metrics, metric)
	}

	// Return the collected metrics
	return metrics, nil
}

// Close closes the collector and releases any resources it might have allocated (no-op for this collector).
func (coll *clocksMetricsCollector) Close() error {
	return nil
}

// Name returns the name of the collector.
func (coll *clocksMetricsCollector) Name() string {
	return clocksMetricsCollectorName
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
