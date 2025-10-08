// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"
)

// nvlinkSample handles NVLink metrics collection logic
func nvlinkSample(device ddnvml.Device) ([]Metric, uint64, error) {
	// Get total number of NVLinks dynamically
	fields := []nvml.FieldValue{
		{
			FieldId: nvml.FI_DEV_NVLINK_LINK_COUNT,
			ScopeId: 0,
		},
	}

	if err := device.GetFieldValues(fields); err != nil {
		return nil, 0, fmt.Errorf("failed to get nvlink count: %w", err)
	}

	totalNVLinks, convErr := fieldValueToNumber[int](nvml.ValueType(fields[0].ValueType), fields[0].Value)
	if convErr != nil {
		return nil, 0, fmt.Errorf("failed to convert number of nvlinks to integer: %w", convErr)
	}

	// Collect NVLink states
	var multiErr error
	active, inactive := 0, 0

	// Iterate over all existing nvlinks for the device
	for i := 0; i < totalNVLinks; i++ {
		state, err := device.GetNvLinkState(i)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("failed to get NVLink state for link %d: %w", i, err))
			continue
		}

		// Count active and inactive links
		if state == nvml.FEATURE_ENABLED {
			active++
		} else if state == nvml.FEATURE_DISABLED {
			inactive++
		}
	}

	// Return metrics
	allMetrics := []Metric{
		{
			Name:  "nvlink.count.total",
			Value: float64(totalNVLinks),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "nvlink.count.active",
			Value: float64(active),
			Type:  metrics.GaugeType,
		},
		{
			Name:  "nvlink.count.inactive",
			Value: float64(inactive),
			Type:  metrics.GaugeType,
		},
	}

	return allMetrics, 0, multiErr
}

// processMemorySample handles process memory usage collection logic
func processMemorySample(device ddnvml.Device) ([]Metric, uint64, error) {
	procs, err := device.GetComputeRunningProcesses()

	var nsPids nsPidCache
	var processMetrics []Metric
	var allPidTags []string

	if err == nil {
		// Create PID tag for this process, and add NS PID if available
		for _, proc := range procs {
			pidTags := []string{fmt.Sprintf("pid:%d", proc.Pid)}
			if nsPid := nsPids.getHostPidNsPid(proc.Pid); nsPid != 0 {
				pidTags = append(pidTags, fmt.Sprintf("nspid:%d", nsPid))
			}
			allPidTags = append(allPidTags, pidTags...)

			processMetrics = append(processMetrics, Metric{
				Name:     "process.memory.usage",
				Value:    float64(proc.UsedGpuMemory),
				Type:     metrics.GaugeType,
				Priority: High,
				Tags:     pidTags,
			})
		}
	}

	// Add device memory limit
	devInfo := device.GetDeviceInfo()
	processMetrics = append(processMetrics, Metric{
		Name:     "memory.limit",
		Value:    float64(devInfo.Memory),
		Type:     metrics.GaugeType,
		Priority: High,
		Tags:     allPidTags,
	})

	return processMetrics, 0, err
}

// createStatelessAPIs creates API call definitions for all stateless metrics on demand
func createStatelessAPIs() []apiCallInfo {
	apis := []apiCallInfo{
		// Memory collector APIs
		{
			Name: "bar1_memory",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				bar1Info, err := device.GetBAR1MemoryInfo()
				if err != nil {
					return nil, 0, err
				}
				return []Metric{
					{Name: "memory.bar1.total", Value: float64(bar1Info.Bar1Total), Type: metrics.GaugeType},
					{Name: "memory.bar1.free", Value: float64(bar1Info.Bar1Free), Type: metrics.GaugeType},
					{Name: "memory.bar1.used", Value: float64(bar1Info.Bar1Used), Type: metrics.GaugeType},
				}, 0, nil
			},
		},
		{
			Name: "device_memory_v2",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				memInfo, err := device.GetMemoryInfoV2()
				if err != nil {
					return nil, 0, err
				}
				return []Metric{
					{Name: "memory.free", Value: float64(memInfo.Free), Priority: High, Type: metrics.GaugeType},
					{Name: "memory.reserved", Value: float64(memInfo.Reserved), Type: metrics.GaugeType},
				}, 0, nil
			},
		},
		{
			Name: "device_memory_v1",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				memInfo, err := device.GetMemoryInfo()
				if err != nil {
					return nil, 0, err
				}
				return []Metric{
					{Name: "memory.free", Value: float64(memInfo.Free), Type: metrics.GaugeType},
				}, 0, nil
			},
		},
		// Device collector APIs
		{
			Name: "pci_throughput_rx",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				rxTput, err := device.GetPcieThroughput(nvml.PCIE_UTIL_RX_BYTES)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "pci.throughput.rx", Value: float64(rxTput) * 1024, Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "pci_throughput_tx",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				txTput, err := device.GetPcieThroughput(nvml.PCIE_UTIL_TX_BYTES)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "pci.throughput.tx", Value: float64(txTput) * 1024, Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "fan_speed",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				speed, err := device.GetFanSpeed()
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "fan_speed", Value: float64(speed), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "power_management_limit",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				limit, err := device.GetPowerManagementLimit()
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "power.management_limit", Value: float64(limit), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "power_usage",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				power, err := device.GetPowerUsage()
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "power.usage", Value: float64(power), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "performance_state",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				state, err := device.GetPerformanceState()
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "performance_state", Value: float64(state), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "temperature",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				temp, err := device.GetTemperature(nvml.TEMPERATURE_GPU)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "temperature", Value: float64(temp), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "clock_speed_sm",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				smClock, err := device.GetClockInfo(nvml.CLOCK_SM)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "clock.speed.sm", Value: float64(smClock), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "clock_speed_memory",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				memoryClock, err := device.GetClockInfo(nvml.CLOCK_MEM)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "clock.speed.memory", Value: float64(memoryClock), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "clock_speed_graphics",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				graphicsClock, err := device.GetClockInfo(nvml.CLOCK_GRAPHICS)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "clock.speed.graphics", Value: float64(graphicsClock), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "clock_speed_video",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				videoClock, err := device.GetClockInfo(nvml.CLOCK_VIDEO)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "clock.speed.video", Value: float64(videoClock), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "max_clock_speed_sm",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				smClock, err := device.GetMaxClockInfo(nvml.CLOCK_SM)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "clock.speed.sm.max", Value: float64(smClock), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "max_clock_speed_memory",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				memoryClock, err := device.GetMaxClockInfo(nvml.CLOCK_MEM)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "clock.speed.memory.max", Value: float64(memoryClock), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "max_clock_speed_graphics",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				graphicsClock, err := device.GetMaxClockInfo(nvml.CLOCK_GRAPHICS)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "clock.speed.graphics.max", Value: float64(graphicsClock), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "max_clock_speed_video",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				videoClock, err := device.GetMaxClockInfo(nvml.CLOCK_VIDEO)
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "clock.speed.video.max", Value: float64(videoClock), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "energy_consumption",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				energy, err := device.GetTotalEnergyConsumption()
				if err != nil {
					return nil, 0, err
				}
				return []Metric{{Name: "total_energy_consumption", Value: float64(energy), Type: metrics.CountType}}, 0, nil
			},
		},
		{
			Name: "device_count",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				isMig, err := device.IsMigDeviceHandle()
				if err != nil {
					return nil, 0, err
				}
				var count float64
				if !isMig {
					count = 1
				}
				return []Metric{{Name: "device.total", Value: count, Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "clock_throttle_reasons",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				reasons, err := device.GetCurrentClocksThrottleReasons()
				if err != nil {
					return nil, 0, err
				}

				var allMetrics []Metric
				for reasonName, reasonBit := range map[string]uint64{
					"gpu_idle":                    nvml.ClocksEventReasonGpuIdle,
					"applications_clocks_setting": nvml.ClocksEventReasonApplicationsClocksSetting,
					"sw_power_cap":                nvml.ClocksEventReasonSwPowerCap,
					"sync_boost":                  nvml.ClocksEventReasonSyncBoost,
					"sw_thermal_slowdown":         nvml.ClocksEventReasonSwThermalSlowdown,
					"display_clock_setting":       nvml.ClocksEventReasonDisplayClockSetting,
					"none":                        nvml.ClocksEventReasonNone,
				} {
					value := 0.0
					if reasons&reasonBit != 0 {
						value = 1.0
					}
					allMetrics = append(allMetrics, Metric{
						Name:  fmt.Sprintf("clock.throttle_reasons.%s", reasonName),
						Value: value,
						Type:  metrics.GaugeType,
					})
				}

				return allMetrics, 0, nil
			},
		},
		{
			Name: "remapped_rows",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				correctable, uncorrectable, pending, failed, err := device.GetRemappedRows()
				if err != nil {
					return nil, 0, err
				}

				return []Metric{
					{Name: "remapped_rows.correctable", Value: float64(correctable), Type: metrics.CountType},
					{Name: "remapped_rows.uncorrectable", Value: float64(uncorrectable), Type: metrics.CountType},
					{Name: "remapped_rows.pending", Value: boolToFloat(pending), Type: metrics.CountType},
					{Name: "remapped_rows.failed", Value: boolToFloat(failed), Type: metrics.CountType},
				}, 0, nil
			},
		},
		// Process memory APIs (stateless - just current snapshot)
		{
			Name: "process_memory_usage",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				return processMemorySample(device)
			},
		},
		// NVLink collector APIs
		{
			Name: "nvlink_metrics",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				return nvlinkSample(device)
			},
		}}

	return apis
}

// statelessAPIFactory allows overriding API creation for testing
var statelessAPIFactory = createStatelessAPIs

// newStatelessCollector creates a collector that consolidates all stateless collector types
func newStatelessCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	return NewBaseCollector(stateless, device, statelessAPIFactory())
}
