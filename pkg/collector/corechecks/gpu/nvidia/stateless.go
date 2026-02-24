// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"
	"strconv"
	"unsafe"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
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

type processMemoryUsageData struct {
	pid           uint32
	usedGpuMemory uint64
}

func processMemoryUsage(device ddnvml.Device, usage []processMemoryUsageData, priority MetricPriority) []Metric {
	var processMetrics []Metric
	var allWorkloadIDs []workloadmeta.EntityID

	for _, usage := range usage {
		workloads := []workloadmeta.EntityID{{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(usage.pid)),
		}}
		allWorkloadIDs = append(allWorkloadIDs, workloads...)

		processMetrics = append(processMetrics, Metric{
			Name:                "process.memory.usage",
			Value:               float64(usage.usedGpuMemory),
			Type:                metrics.GaugeType,
			Priority:            priority,
			AssociatedWorkloads: workloads,
		})
	}

	// This covers for the edge case where the higher-priority API is returning
	// an error but the lower-priority API is still returning metrics. More
	// detailed explanation follows:
	//
	// First, we want to always emit memory.limit even if the process-level API
	// returns an error. The limit is always retrievable, and this way we have a
	// consistent limit that can be shown in the UI or dashboards.
	//
	// Second, we have two different APIs for getting the process-level memory
	// usage. One is higher-priority as it's more reliable, but it's not
	// available on all architectures, so we need to keep the lower-priority API
	// as a fallback.
	//
	// The edge case is when the higher-priority API returns an error but the
	// lower-priority API is still returning metrics. In that case, the
	// higher-priority API would still try to send the corresponding
	// memory.limit metric with high priority, which would not have all the
	// workload tags because we don't have the process data. The lower-priority
	// API would have all the workload tags, but the memory.limit metric would
	// be emitted with low priority and get overridden by the higher-priority
	// API's memory.limit metric. The end result is that we would have
	// process.memory.usage metrics tagged with PIDs, but no memory.limit metric
	// with corresponding tags.
	//
	// The solution is the following change: the priority for memory.limit is
	// set to low if there are no workloads associated with the metric. It fixes
	// the edge case described above. In the case that all APIs are returning no
	// workloads, the memory.limit will have the same tag and value so it
	// doesn't matter which one we choose. If there are conflicts between the
	// data reported by the two APIs, we will still keep the high-priority
	// metric, consistently sending the highest-priority metric for both
	// memory.limit and process.memory.usage.
	//
	// We set MediumLow as the priority as we still want to ensure that any of the APIs
	// for the stateless collector are higher priority than the eBPF collector, which should
	// only emit metrics if neither of the NVML APIs are supported.
	metricLimitPriority := priority
	if len(allWorkloadIDs) == 0 {
		metricLimitPriority = MediumLow
	}

	// Add device memory limit
	devInfo := device.GetDeviceInfo()
	processMetrics = append(processMetrics, Metric{
		Name:                "memory.limit",
		Value:               float64(devInfo.Memory),
		Type:                metrics.GaugeType,
		Priority:            metricLimitPriority,
		AssociatedWorkloads: allWorkloadIDs,
	})

	return processMetrics
}

// processMemorySample handles process memory usage collection logic
func processMemorySample(device ddnvml.Device) ([]Metric, uint64, error) {
	procs, err := device.GetComputeRunningProcesses()
	var usage []processMemoryUsageData
	if err == nil {
		for _, proc := range procs {
			usage = append(usage, processMemoryUsageData{
				pid:           proc.Pid,
				usedGpuMemory: proc.UsedGpuMemory,
			})
		}
	}

	return processMemoryUsage(device, usage, Medium), 0, err
}

func processDetailListSample(device ddnvml.Device) ([]Metric, uint64, error) {
	if device.GetDeviceInfo().Architecture < nvml.DEVICE_ARCH_HOPPER {
		// This API is only supported on Hopper and later, but it doesn't return "not supported" error,
		// instead it reports "Argument version mismatch", so just do the check here.
		return nil, 0, ddnvml.NewNvmlAPIErrorOrNil("GetRunningProcessDetailList", nvml.ERROR_NOT_SUPPORTED)
	}

	detail, err := device.GetRunningProcessDetailList()
	var usage []processMemoryUsageData
	if err == nil {
		// procs.ProcArray is a pointer to an array of ProcessDetail_v1, in C-style pointer+length mode,
		// so convert it to a slice:
		procs := unsafe.Slice(detail.ProcArray, detail.NumProcArrayEntries)
		for _, proc := range procs {
			usage = append(usage, processMemoryUsageData{
				pid:           proc.Pid,
				usedGpuMemory: proc.UsedGpuMemory,
			})
		}
	}

	return processMemoryUsage(device, usage, High), 0, err
}

// createStatelessAPIs creates API call definitions for all stateless metrics on demand
func createStatelessAPIs(deps *CollectorDependencies) []apiCallInfo {
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
					{Name: "memory.free", Value: float64(memInfo.Free), Priority: Medium, Type: metrics.GaugeType},
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
				return []Metric{{Name: "total_energy_consumption", Value: float64(energy), Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "device_count",
			Handler: func(_ ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				return []Metric{{Name: "device.total", Value: 1, Type: metrics.GaugeType}}, 0, nil
			},
		},
		{
			Name: "device_unhealthy_count",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				gpu, err := deps.Workloadmeta.GetGPU(device.GetDeviceInfo().UUID)
				if err != nil {
					return nil, 0, err
				}
				var count float64
				if !gpu.Healthy {
					count = 1
				}
				return []Metric{{Name: "device.unhealthy", Value: count, Type: metrics.GaugeType}}, 0, nil
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
					if reasons&reasonBit != 0 || (reasons == 0 && reasonBit == 0) {
						value = 1.0
					}
					allMetrics = append(allMetrics, Metric{
						Name:  "clock.throttle_reasons." + reasonName,
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
					{Name: "remapped_rows.correctable", Value: float64(correctable), Type: metrics.GaugeType},
					{Name: "remapped_rows.uncorrectable", Value: float64(uncorrectable), Type: metrics.GaugeType},
					{Name: "remapped_rows.pending", Value: boolToFloat(pending), Type: metrics.GaugeType},
					{Name: "remapped_rows.failed", Value: boolToFloat(failed), Type: metrics.GaugeType},
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
		// similar to process_memory_usage, but works with MIG. However, only supported on Hopper and later.
		{
			Name: "process_detail_list",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				return processDetailListSample(device)
			},
		},
		// NVLink collector APIs
		{
			Name: "nvlink_metrics",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				return nvlinkSample(device)
			},
		},
	}

	// Create APIs for ECC errors
	for errorType, errorTypeName := range eccErrorTypeToName {
		for memoryLocation, memoryLocationName := range memoryLocationToName {
			apis = append(apis, apiCallInfo{
				Name: fmt.Sprintf("ecc_errors.%s.%s", errorTypeName, memoryLocationName),
				Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
					count, err := device.GetMemoryErrorCounter(errorType, nvml.AGGREGATE_ECC, memoryLocation)
					if err != nil {
						return nil, 0, err
					}
					return []Metric{{
						Name:  fmt.Sprintf("errors.ecc.%s.total", errorTypeName),
						Value: float64(count),
						Type:  metrics.GaugeType,
						Tags: []string{
							"memory_location:" + memoryLocationName,
						},
					}}, 0, nil
				},
			})
		}
	}

	return apis
}

// statelessAPIFactory allows overriding API creation for testing
var statelessAPIFactory = createStatelessAPIs

// newStatelessCollector creates a collector that consolidates all stateless collector types
func newStatelessCollector(device ddnvml.Device, deps *CollectorDependencies) (Collector, error) {
	return NewBaseCollector(stateless, device, statelessAPIFactory(deps))
}
