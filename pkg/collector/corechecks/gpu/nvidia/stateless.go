// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"strconv"
	"unsafe"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// nvlinkSample handles NVLink metrics collection logic
func nvlinkSample(device ddnvml.Device) ([]Metric, uint64, error) {
	totalNVLinks, err := GetNVLinkCount(device)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get nvlink count: %w", err)
	}

	// Collect NVLink states
	var multiErr []error
	active, inactive := 0, 0

	// Iterate over all existing nvlinks for the device
	for i := 0; i < totalNVLinks; i++ {
		state, err := device.GetNvLinkState(i)
		if err != nil {
			multiErr = append(multiErr, fmt.Errorf("failed to get NVLink state for link %d: %w", i, err))
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

	return allMetrics, 0, errors.Join(multiErr...)
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
	var nvmlErr *ddnvml.NvmlAPIError
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
	} else if errors.As(err, &nvmlErr) && nvmlErr.NvmlErrorCode == nvml.ERROR_INSUFFICIENT_SIZE {
		// Depending on the NVML implementation, there might be an issue with the size of the array being passed.
		// This PR seems related https://github.com/NVIDIA/go-nvml/pull/165 but for now we will suppress the error
		// and continue with the collection.
		// In this case, if we get no metrics, processMemoryUsage will emit a memory.limit metric with low priority
		// so that it can be overridden by alternative APIs if available.
		err = nil
	}

	return processMemoryUsage(device, usage, High), 0, err
}

func shouldSkipLegacyEccMetric(device ddnvml.Device, errorType nvml.MemoryErrorType, memoryLocation nvml.MemoryLocation) bool {
	if device.GetDeviceInfo().Architecture < nvml.DEVICE_ARCH_AMPERE {
		return false
	}

	if memoryLocation == nvml.MEMORY_LOCATION_SRAM {
		return true
	}

	return errorType == nvml.MEMORY_ERROR_TYPE_UNCORRECTED && memoryLocation == nvml.MEMORY_LOCATION_L2_CACHE
}

func sramEccErrorStatusSample(device ddnvml.Device) ([]Metric, uint64, error) {
	// SRAM ECC error status is only supported on Ampere and later. Some of the metrics
	// overlap with the legacy ECC metrics, so we need to check the architecture and return an error
	if device.GetDeviceInfo().Architecture < nvml.DEVICE_ARCH_AMPERE {
		return nil, 0, ddnvml.NewNvmlAPIErrorOrNil("GetSramEccErrorStatus", nvml.ERROR_NOT_SUPPORTED)
	}

	status, err := device.GetSramEccErrorStatus()
	if err != nil {
		return nil, 0, err
	}

	metricsOut := []Metric{
		{
			Name:  "errors.ecc.corrected.total",
			Value: float64(status.AggregateCor),
			Type:  metrics.GaugeType,
			Tags:  []string{"memory_location:sram"},
		},
		{
			Name:  "errors.ecc.sram.uncorrected_by_subtype.total",
			Value: float64(status.AggregateUncParity),
			Type:  metrics.GaugeType,
			Tags:  []string{"memory_location:sram", "error_subtype:parity"},
		},
		{
			Name:  "errors.ecc.sram.uncorrected_by_subtype.total",
			Value: float64(status.AggregateUncSecDed),
			Type:  metrics.GaugeType,
			Tags:  []string{"memory_location:sram", "error_subtype:secded"},
		},
		{
			Name:  "errors.ecc.uncorrected.total",
			Value: float64(status.AggregateUncBucketL2),
			Type:  metrics.GaugeType,
			Tags:  []string{"memory_location:l2_cache"},
		},
		{
			Name:  "errors.ecc.uncorrected.total",
			Value: float64(status.AggregateUncBucketSm),
			Type:  metrics.GaugeType,
			Tags:  []string{"memory_location:sm"},
		},
		{
			Name:  "errors.ecc.uncorrected.total",
			Value: float64(status.AggregateUncBucketPcie),
			Type:  metrics.GaugeType,
			Tags:  []string{"memory_location:pcie"},
		},
		{
			Name:  "errors.ecc.uncorrected.total",
			Value: float64(status.AggregateUncBucketMcu),
			Type:  metrics.GaugeType,
			Tags:  []string{"memory_location:microcontroller"},
		},
		{
			Name:  "errors.ecc.uncorrected.total",
			Value: float64(status.AggregateUncBucketOther),
			Type:  metrics.GaugeType,
			Tags:  []string{"memory_location:other"},
		},
		{
			Name:  "errors.ecc.sram.threshold_exceeded",
			Value: boolToFloat(status.BThresholdExceeded != 0),
			Type:  metrics.GaugeType,
		},
	}

	return metricsOut, 0, nil
}

// gpuRecoveryActionToTag maps the NVML GPU recovery action enum to the
// recovery_action tag value used on the device.needs_recovery metric.
var gpuRecoveryActionToTag = map[nvml.DeviceGpuRecoveryAction]string{
	nvml.GPU_RECOVERY_ACTION_NONE:            "none",
	nvml.GPU_RECOVERY_ACTION_GPU_RESET:       "reset",
	nvml.GPU_RECOVERY_ACTION_NODE_REBOOT:     "reboot",
	nvml.GPU_RECOVERY_ACTION_DRAIN_P2P:       "drain",
	nvml.GPU_RECOVERY_ACTION_DRAIN_AND_RESET: "drain_and_reset",
}

// needsRecoverySample queries the GPU recovery action field and emits a single
// device.needs_recovery metric: 0 when no action is required, 1 otherwise. The
// metric is tagged with the specific recovery action.
func needsRecoverySample(device ddnvml.Device) ([]Metric, uint64, error) {
	action, err := fieldValueForField(device, nvml.FI_DEV_GET_GPU_RECOVERY_ACTION, "FI_DEV_GET_GPU_RECOVERY_ACTION")
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get GPU recovery action: %w", err)
	}

	recoveryAction := nvml.DeviceGpuRecoveryAction(action)
	actionTag, ok := gpuRecoveryActionToTag[recoveryAction]
	if !ok {
		// Unknown future action value: still report that recovery is needed,
		// tagging it with the raw value so it's not silently dropped.
		actionTag = "unknown_" + strconv.Itoa(action)
	}

	return []Metric{{
		Name:  "device.needs_recovery",
		Value: boolToFloat(recoveryAction != nvml.GPU_RECOVERY_ACTION_NONE),
		Type:  metrics.GaugeType,
		Tags:  []string{"recovery_action:" + actionTag},
	}}, 0, nil
}

// pcieGenSpec describes the physical-layer characteristics of a PCIe generation. Values come from the PCI-SIG base
// specification and are fixed by the standard.
type pcieGenSpec struct {
	// gtPerSecondPerLane is the raw per-lane transfer rate in GT/s.
	gtPerSecondPerLane float64
	// encodedBytesPerTransfer is the useful data bytes carried per raw transfer,
	// accounting for line-coding overhead (8b/10b, 128b/130b, etc.).
	encodedBytesPerTransfer float64
}

// pcieGenTable provides a lookup for encoding keyed off of the PCIe generation.
var pcieGenTable = map[uint32]pcieGenSpec{
	// Source: PCI Express Base Specification 1.0a (PCI-SIG, 2003).
	// 2.5 GT/s per lane, 8b/10b encoding → 250 MB/s per lane.
	1: {gtPerSecondPerLane: 2.5, encodedBytesPerTransfer: 8.0 / 10.0 / 8.0},
	// Source: PCI Express Base Specification 2.0 (PCI-SIG, 2007).
	// 5.0 GT/s per lane, 8b/10b encoding → 500 MB/s per lane.
	2: {gtPerSecondPerLane: 5.0, encodedBytesPerTransfer: 8.0 / 10.0 / 8.0},
	// Source: PCI Express Base Specification 3.0 (PCI-SIG, 2010).
	// 8.0 GT/s per lane, 128b/130b encoding → 985 MB/s per lane.
	3: {gtPerSecondPerLane: 8.0, encodedBytesPerTransfer: 128.0 / 130.0 / 8.0},
	// Source: PCI Express Base Specification 4.0 (PCI-SIG, 2017).
	// 16.0 GT/s per lane, 128b/130b encoding → ~1.969 GB/s per lane.
	4: {gtPerSecondPerLane: 16.0, encodedBytesPerTransfer: 128.0 / 130.0 / 8.0},
	// Source: PCI Express Base Specification 5.0 (PCI-SIG, 2019).
	// 32.0 GT/s per lane, 128b/130b NRZ encoding → ~3.938 GB/s per lane.
	5: {gtPerSecondPerLane: 32.0, encodedBytesPerTransfer: 128.0 / 130.0 / 8.0},
	// Source: PCI Express Base Specification 6.0 (PCI-SIG, 2022).
	// 64.0 GT/s per lane with PAM4 signaling, no line-coding overhead (1b/1b).
	// FLIT mode frames data as 256-byte FLITs with 14 bytes of CRC+FEC+framing
	// overhead, giving 242 bytes of usable payload per 256 wire bytes
	// → ~7.563 GB/s per lane of usable bandwidth.
	6: {gtPerSecondPerLane: 64.0, encodedBytesPerTransfer: 242.0 / 256.0 / 8.0},
}

// pcieLinkBytesPerSecond returns the usable bandwidth for a given generation and lane width.
func pcieLinkBytesPerSecond(gen int, width int) (float64, error) {
	spec, ok := pcieGenTable[uint32(gen)]
	if !ok {
		return 0, fmt.Errorf("unknown PCIe generation %d (extend pcieGenTable)", gen)
	}
	if width < 1 {
		return 0, fmt.Errorf("invalid PCIe link width: %d", width)
	}

	// bytes/sec = GT/s/lane * 1e9 transfers/sec/GT * bytes/transfer * lanes
	bps := spec.gtPerSecondPerLane * 1e9 * spec.encodedBytesPerTransfer * float64(width)
	return bps, nil
}

func pcieLinkMetrics(device ddnvml.Device) ([]Metric, uint64, error) {
	var metricsOut []Metric

	currentWidth, err := device.GetCurrPcieLinkWidth()
	if err != nil {
		return metricsOut, 0, fmt.Errorf("get current PCIe link width: %w", err)
	}
	metricsOut = append(metricsOut, Metric{Name: "pci.link.width.current", Value: float64(currentWidth), Type: metrics.GaugeType})

	maxWidth, err := device.GetMaxPcieLinkWidth()
	if err != nil {
		return metricsOut, 0, fmt.Errorf("get max PCIe link width: %w", err)
	}
	metricsOut = append(metricsOut, Metric{Name: "pci.link.width.max", Value: float64(maxWidth), Type: metrics.GaugeType})
	metricsOut = append(metricsOut, Metric{Name: "pci.link.width.degraded", Value: boolToFloat(currentWidth < maxWidth), Type: metrics.GaugeType})

	currentGeneration, err := device.GetCurrPcieLinkGeneration()
	if err != nil {
		return metricsOut, 0, fmt.Errorf("get current PCIe link generation: %w", err)
	}
	currentSpeed, err := pcieLinkBytesPerSecond(currentGeneration, currentWidth)
	if err != nil {
		return metricsOut, 0, fmt.Errorf("compute current PCIe link speed: %w", err)
	}
	metricsOut = append(metricsOut, Metric{Name: "pci.link.speed.current", Value: currentSpeed, Type: metrics.GaugeType})

	maxGeneration, err := device.GetMaxPcieLinkGeneration()
	if err != nil {
		return metricsOut, 0, fmt.Errorf("get max PCIe link generation: %w", err)
	}
	maxSpeed, err := pcieLinkBytesPerSecond(maxGeneration, maxWidth)
	if err != nil {
		return metricsOut, 0, fmt.Errorf("compute max PCIe link speed: %w", err)
	}
	metricsOut = append(metricsOut, Metric{Name: "pci.link.speed.max", Value: maxSpeed, Type: metrics.GaugeType})
	metricsOut = append(metricsOut, Metric{Name: "pci.link.speed.degraded", Value: boolToFloat(currentSpeed < maxSpeed), Type: metrics.GaugeType})
	return metricsOut, 0, nil
}

type clockThrottleReason struct {
	name string
	bit  uint64
}

var clockThrottleReasons = []clockThrottleReason{
	{name: "gpu_idle", bit: nvml.ClocksEventReasonGpuIdle},
	{name: "applications_clocks_setting", bit: nvml.ClocksEventReasonApplicationsClocksSetting},
	{name: "sw_power_cap", bit: nvml.ClocksEventReasonSwPowerCap},
	{name: "hw_slowdown", bit: nvml.ClocksThrottleReasonHwSlowdown},
	{name: "sync_boost", bit: nvml.ClocksEventReasonSyncBoost},
	{name: "sw_thermal_slowdown", bit: nvml.ClocksEventReasonSwThermalSlowdown},
	{name: "hw_thermal_slowdown", bit: nvml.ClocksThrottleReasonHwThermalSlowdown},
	{name: "hw_power_brake_slowdown", bit: nvml.ClocksThrottleReasonHwPowerBrakeSlowdown},
	{name: "display_clock_setting", bit: nvml.ClocksEventReasonDisplayClockSetting},
	{name: "none", bit: nvml.ClocksEventReasonNone},
}

const notThrottledReason = "not_throttled"
const throttleReasonTag = "throttle_reason"

func clockThrottleReasonMetrics(reasons uint64) []Metric {
	allMetrics := make([]Metric, 0, len(clockThrottleReasons)*2)

	// emit per-reason metrics
	throttledWhileActiveValueReason := notThrottledReason
	for _, reason := range clockThrottleReasons {
		throttleReasonValue := 0.0

		// check if reason is set
		if reasons&reason.bit != 0 || (reasons == 0 && reason.bit == 0) {
			throttleReasonValue = 1.0

			// if the reason is not idle or "none" (i.e., not throttled), set the reason for the throttledWhileActive metric
			// note that usually, only one reason is set, so we don't care about overwriting it.
			if reason.name != "gpu_idle" && reason.name != "none" {
				throttledWhileActiveValueReason = reason.name
			}
		}

		allMetrics = append(allMetrics, Metric{
			Name:  "clock.throttle_reasons." + reason.name,
			Value: throttleReasonValue,
			Type:  metrics.GaugeType,
		})
	}

	throttledWhileActiveValue := 0.0
	if throttledWhileActiveValueReason != notThrottledReason {
		throttledWhileActiveValue = 1.0
	}

	allMetrics = append(allMetrics, Metric{
		Name:  "clock.throttled_while_active",
		Value: throttledWhileActiveValue,
		Type:  metrics.GaugeType,
		Tags:  []string{throttleReasonTag + ":" + throttledWhileActiveValueReason},
	})

	return allMetrics
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
				// Prevent division by zero if the total is zero.
				memoryUtilization := 0.0
				if memInfo.Total > 0 {
					memoryUtilization = float64(memInfo.Used) / float64(memInfo.Total)
				}
				return []Metric{
					{Name: "memory.free", Value: float64(memInfo.Free), Priority: Medium, Type: metrics.GaugeType},
					{Name: "memory.reserved", Value: float64(memInfo.Reserved), Type: metrics.GaugeType},
					{Name: "memory.utilization", Value: memoryUtilization, Type: metrics.GaugeType},
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
			Name: "pci_link",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				return pcieLinkMetrics(device)
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
			Name: "fan_speed_v2",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				var output []Metric
				numFans, err := device.GetNumFans()
				if err != nil {
					return nil, 0, fmt.Errorf("failed to get number of fans: %w", err)
				}

				var multiErr []error
				for i := 0; i < numFans; i++ {
					speed, err := device.GetFanSpeed_v2(i)
					if err != nil {
						multiErr = append(multiErr, fmt.Errorf("failed to get fan speed for fan %d: %w", i, err))
					} else {
						output = append(output, Metric{
							Name:     "fan_speed",
							Value:    float64(speed),
							Type:     metrics.GaugeType,
							Priority: Medium,
							Tags:     []string{"fan_index:" + strconv.Itoa(i)},
						})
					}
				}

				return output, 0, errors.Join(multiErr...)
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
				if !env.IsFeaturePresent(env.KubernetesDevicePlugins) {
					return nil, 0, ddnvml.NewNvmlAPIErrorOrNil("GetUnhealthyDevices", nvml.ERROR_NOT_SUPPORTED)
				}

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

				return clockThrottleReasonMetrics(reasons), 0, nil
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
		{
			Name: "repair_status",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				repairStatus, err := device.GetRepairStatus()
				if err != nil {
					return nil, 0, err
				}

				return []Metric{
					{
						Name:  "ecc.repair_pending.channel",
						Value: float64(repairStatus.BChannelRepairPending),
						Type:  metrics.GaugeType,
					},
					{
						Name:  "ecc.repair_pending.tpc",
						Value: float64(repairStatus.BTpcRepairPending),
						Type:  metrics.GaugeType,
					},
				}, 0, nil
			},
		},
		{
			Name: "sram_ecc_error_status",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				return sramEccErrorStatusSample(device)
			},
		},
		{
			Name: "needs_recovery",
			Handler: func(device ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
				return needsRecoverySample(device)
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
					if shouldSkipLegacyEccMetric(device, errorType, memoryLocation) {
						return nil, 0, ddnvml.NewNvmlAPIErrorOrNil("GetMemoryErrorCounter", nvml.ERROR_NOT_SUPPORTED)
					}

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
