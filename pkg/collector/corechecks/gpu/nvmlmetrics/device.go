// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024_present Datadog, Inc.

//go:build linux

package nvmlmetrics

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"
)

const deviceMetricsCollectorName = "device"

type deviceMetricsCollector struct {
}

func newDeviceMetricsCollector(_ nvml.Interface, _ []nvml.Device) (subsystemCollector, error) {
	return &deviceMetricsCollector{}, nil
}

type deviceMetricGetter func(nvml.Device) ([]deviceMetricValue, nvml.Return)

// deviceMetric represents a metric that can be collected from an NVML device, using the NVML
// API on that specific device.
type deviceMetric struct {
	name   string
	getter deviceMetricGetter
}

type deviceMetricValue struct {
	value  float64
	suffix string
}

// collectAllDeviceMetrics collects all the metrics from the given NVML device.
func (coll *deviceMetricsCollector) collectMetrics(dev nvml.Device) ([]Metric, error) {
	var err error

	values := make([]Metric, 0, len(allDeviceMetrics)) // preallocate to reduce allocations
	for _, metric := range allDeviceMetrics {
		metricValues, ret := metric.getter(dev)
		if ret != nvml.SUCCESS {
			err = multierror.Append(err, fmt.Errorf("failed to get metric %s: %s", metric.name, nvml.ErrorString(ret)))
			continue
		}

		for _, metricVal := range metricValues {
			metricName := metric.name
			if metricVal.suffix != "" {
				metricName += "." + metricVal.suffix
			}

			values = append(values, Metric{
				Name:  metricName,
				Value: metricVal.value,
			})
		}
	}

	return values, err
}

func (coll *deviceMetricsCollector) close() error {
	return nil
}

func (coll *deviceMetricsCollector) name() string {
	return deviceMetricsCollectorName
}

var allDeviceMetrics = []deviceMetric{
	{"pci.throughput.tx", getTxPciThroughput},
	{"pci.throughput.rx", getRxPciThroughput},
	{"clock_throttle_reasons", getClockThrottleReasons},
	{"remapped_rows", getRemappedRows},
	{"decoder_utiliation", getDecoderUtilization},
	{"dram_active", getDramActive},
	{"encoder_utilization", getEncoderUtilization},
	{"fan_speed", getFanSpeed},
	{"power.management_limit", getPowerManagementLimit},
	{"power.usage", getPowerUsage},
	{"performance_state", getPerformanceState},
	{"clock_speed.sm", getSMClockSpeed},
	{"clock_speed.memory", getMemoryClockSpeed},
	{"clock_speed.graphics", getGraphicsClockSpeed},
	{"clock_speed.video", getVideoClockSpeed},
	{"temperature", getTemperature},
	{"total_energy_consumption", getTotalEnergyConsumption},
}

func getRxPciThroughput(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	tput, ret := dev.GetPcieThroughput(nvml.PCIE_UTIL_RX_BYTES)
	return []deviceMetricValue{{float64(tput), ""}}, ret
}

func getTxPciThroughput(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	tput, ret := dev.GetPcieThroughput(nvml.PCIE_UTIL_TX_BYTES)
	return []deviceMetricValue{{float64(tput), ""}}, ret
}

func getClockThrottleReasons(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	allThrottleReasons := map[string]uint64{
		"gpu_idle":                    nvml.ClocksEventReasonGpuIdle,
		"applications_clocks_setting": nvml.ClocksEventReasonApplicationsClocksSetting,
		"sw_power_cap":                nvml.ClocksEventReasonSwPowerCap,
		"sync_boost":                  nvml.ClocksEventReasonSyncBoost,
		"sw_thermal_slowdown":         nvml.ClocksEventReasonSwThermalSlowdown,
		"display_clock_setting":       nvml.ClocksEventReasonDisplayClockSetting,
		"none":                        nvml.ClocksEventReasonNone,
	}

	reasons, ret := dev.GetCurrentClocksThrottleReasons()
	values := make([]deviceMetricValue, 0, len(allThrottleReasons))
	for reasonName, reasonBit := range allThrottleReasons {
		name, bit := reasonName, reasonBit
		values = append(values, deviceMetricValue{float64((reasons & bit) >> bit), name})
	}

	return values, ret
}

func getRemappedRows(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	correctable, uncorrectable, pending, failed, ret := dev.GetRemappedRows()
	return []deviceMetricValue{
		{float64(correctable), "correctable"},
		{float64(uncorrectable), "uncorrectable"},
		{boolToFloat(pending), "has_pending_corrections"},
		{boolToFloat(failed), "has_failed_corrections"},
	}, ret
}

func getDecoderUtilization(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	util, _, ret := dev.GetDecoderUtilization()
	return []deviceMetricValue{{float64(util), ""}}, ret
}

func getDramActive(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	util, ret := dev.GetUtilizationRates()
	return []deviceMetricValue{{float64(util.Memory), ""}}, ret
}

func getEncoderUtilization(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	util, _, ret := dev.GetEncoderUtilization()
	return []deviceMetricValue{{float64(util), ""}}, ret
}

func getFanSpeed(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	speed, ret := dev.GetFanSpeed()
	return []deviceMetricValue{{float64(speed), ""}}, ret
}

func getPowerManagementLimit(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	limit, ret := dev.GetPowerManagementLimit()
	return []deviceMetricValue{{float64(limit), ""}}, ret
}

func getPowerUsage(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	power, ret := dev.GetPowerUsage()
	return []deviceMetricValue{{float64(power), ""}}, ret
}

func getPerformanceState(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	state, ret := dev.GetPerformanceState()
	return []deviceMetricValue{{float64(state), ""}}, ret
}

func getSMClockSpeed(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_SM)
	return []deviceMetricValue{{float64(speed), ""}}, ret
}

func getMemoryClockSpeed(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_MEM)
	return []deviceMetricValue{{float64(speed), ""}}, ret
}

func getGraphicsClockSpeed(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_GRAPHICS)
	return []deviceMetricValue{{float64(speed), ""}}, ret
}

func getVideoClockSpeed(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_VIDEO)
	return []deviceMetricValue{{float64(speed), ""}}, ret
}

func getTemperature(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	temp, ret := dev.GetTemperature(nvml.TEMPERATURE_GPU)
	return []deviceMetricValue{{float64(temp), ""}}, ret
}

func getTotalEnergyConsumption(dev nvml.Device) ([]deviceMetricValue, nvml.Return) {
	energy, ret := dev.GetTotalEnergyConsumption()
	return []deviceMetricValue{{float64(energy), ""}}, ret
}
