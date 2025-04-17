// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const clocksMetricsPrefix = "clock.throttle_reasons"

// clocksCollector collects clock metrics from an NVML device.
type clocksCollector struct {
	device ddnvml.SafeDevice
}

// newClocksCollector creates a new clocksMetricsCollector for the given NVML device.
func newClocksCollector(device ddnvml.SafeDevice) (Collector, error) {
	// Check first if the device supports clock throttle reasons
	_, err := device.GetCurrentClocksThrottleReasons()

	if err != nil && ddnvml.IsUnsupported(err) {
		// Only return unsupported device if the API is not supported or symbol not found
		return nil, fmt.Errorf("%w: %w", errUnsupportedDevice, err)
	}

	return &clocksCollector{
		device: device,
	}, nil
}

func (c *clocksCollector) DeviceUUID() string {
	uuid, _ := c.device.GetUUID()
	return uuid
}

// Collect collects clock throttle reason metrics from the NVML device.
func (c *clocksCollector) Collect() ([]Metric, error) {
	allReasons, err := c.device.GetCurrentClocksThrottleReasons()
	if err != nil {
		return nil, err
	}

	metricValues := make([]Metric, 0, len(allThrottleReasons))
	for name, bit := range allThrottleReasons {
		value := boolToFloat((allReasons & bit) != 0)
		metric := Metric{
			Name:  fmt.Sprintf("%s.%s", clocksMetricsPrefix, name),
			Value: value,
			Type:  metrics.GaugeType,
		}
		metricValues = append(metricValues, metric)
	}

	// Return the collected metrics
	return metricValues, nil
}

// Name returns the name of the collector.
func (c *clocksCollector) Name() CollectorName {
	return clock
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
