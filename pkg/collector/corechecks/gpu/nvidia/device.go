// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"
	"slices"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/common"
)

var allDeviceMetrics = []deviceMetric{
	{"pci.throughput.tx", getTxPciThroughput, metrics.GaugeType},
	{"pci.throughput.rx", getRxPciThroughput, metrics.GaugeType},
	{"decoder_utilization", getDecoderUtilization, metrics.GaugeType},
	{"dram_active", getDramActive, metrics.GaugeType},
	{"encoder_utilization", getEncoderUtilization, metrics.GaugeType},
	{"fan_speed", getFanSpeed, metrics.GaugeType},
	{"power.management_limit", getPowerManagementLimit, metrics.GaugeType},
	{"power.usage", getPowerUsage, metrics.GaugeType},
	{"performance_state", getPerformanceState, metrics.GaugeType},
	{"clock.speed.sm", getCurrentSMClockSpeed, metrics.GaugeType},
	{"clock.speed.memory", getCurrentMemoryClockSpeed, metrics.GaugeType},
	{"clock.speed.graphics", getCurrentGraphicsClockSpeed, metrics.GaugeType},
	{"clock.speed.video", getCurrentVideoClockSpeed, metrics.GaugeType},
	{"clock.speed.sm.max", getMaxSMClockSpeed, metrics.GaugeType},
	{"clock.speed.memory.max", getMaxMemoryClockSpeed, metrics.GaugeType},
	{"clock.speed.graphics.max", getMaxGraphicsClockSpeed, metrics.GaugeType},
	{"clock.speed.video.max", getMaxVideoClockSpeed, metrics.GaugeType},
	{"temperature", getTemperature, metrics.GaugeType},
	{"total_energy_consumption", getTotalEnergyConsumption, metrics.CountType},
	{"sm_active", getSMActive, metrics.GaugeType},
	{"device.total", getDeviceCount, metrics.GaugeType},
}

type deviceCollector struct {
	device        ddnvml.SafeDevice
	metricGetters []deviceMetric
}

func newDeviceCollector(device ddnvml.SafeDevice) (Collector, error) {
	c := &deviceCollector{
		device: device,
	}
	c.metricGetters = append(c.metricGetters, allDeviceMetrics...) // copy all metrics to avoid modifying the original slice

	c.removeUnsupportedGetters()
	if len(c.metricGetters) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *deviceCollector) DeviceUUID() string {
	uuid, _ := c.device.GetUUID()
	return uuid
}

func (c *deviceCollector) removeUnsupportedGetters() {
	metricsToRemove := common.StringSet{}

	for _, metric := range c.metricGetters {
		_, err := metric.getter(c.device)
		// Only remove metrics if the API is not supported or symbol not found
		if err != nil && ddnvml.IsUnsupported(err) {
			metricsToRemove.Add(metric.name)
		}
	}

	for toRemove := range metricsToRemove {
		c.metricGetters = slices.DeleteFunc(c.metricGetters, func(m deviceMetric) bool {
			return m.name == toRemove
		})
	}
}

// deviceMetricGetter is a function type that receives a NVML device and returns one or more values
type deviceMetricGetter func(ddnvml.SafeDevice) (float64, error)

// deviceMetric represents a metric that can be collected from an NVML device, using the NVML
// API on that specific device.
type deviceMetric struct {
	name       string
	getter     deviceMetricGetter
	metricType metrics.MetricType
}

// Collect collects all the metrics from the given NVML device.
func (c *deviceCollector) Collect() ([]Metric, error) {
	var multiErr error

	values := make([]Metric, 0, len(c.metricGetters)) // preallocate to reduce allocations
	for _, metric := range c.metricGetters {
		value, err := metric.getter(c.device)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("failed to get metric %s: %w", metric.name, err))
			continue
		}

		values = append(values, Metric{
			Name:  metric.name,
			Value: value,
			Type:  metric.metricType,
		})

	}

	return values, multiErr
}

// Name returns the name of the collector.
func (c *deviceCollector) Name() CollectorName {
	return device
}

func getRxPciThroughput(dev ddnvml.SafeDevice) (float64, error) {
	// Output in KB/s
	tput, err := dev.GetPcieThroughput(nvml.PCIE_UTIL_RX_BYTES)
	// Convert to B/S
	return float64(tput) * 1024, err
}

func getTxPciThroughput(dev ddnvml.SafeDevice) (float64, error) {
	// Output in KB/s
	tput, err := dev.GetPcieThroughput(nvml.PCIE_UTIL_TX_BYTES)
	// Convert to B/S
	return float64(tput) * 1024, err
}

func getDecoderUtilization(dev ddnvml.SafeDevice) (float64, error) {
	// returns utilization from 0-100
	util, _, err := dev.GetDecoderUtilization()
	return float64(util), err
}

func getDramActive(dev ddnvml.SafeDevice) (float64, error) {
	// returns utilization from 0-100
	util, err := dev.GetUtilizationRates()
	return float64(util.Memory), err
}

func getSMActive(dev ddnvml.SafeDevice) (float64, error) {
	util, err := dev.GetUtilizationRates()
	return float64(util.Gpu), err
}

func getEncoderUtilization(dev ddnvml.SafeDevice) (float64, error) {
	// returns utilization from 0-100
	util, _, err := dev.GetEncoderUtilization()
	return float64(util), err
}

func getFanSpeed(dev ddnvml.SafeDevice) (float64, error) {
	// returns percentage from 0-100 (0 = fan off)
	speed, err := dev.GetFanSpeed()
	return float64(speed), err
}

func getPowerManagementLimit(dev ddnvml.SafeDevice) (float64, error) {
	// returns power limit in milliwatts
	limit, err := dev.GetPowerManagementLimit()
	return float64(limit), err
}

func getPowerUsage(dev ddnvml.SafeDevice) (float64, error) {
	// returns power usage in milliwatts
	power, err := dev.GetPowerUsage()
	return float64(power), err
}

func getPerformanceState(dev ddnvml.SafeDevice) (float64, error) {
	state, err := dev.GetPerformanceState()
	return float64(state), err
}

func getCurrentSMClockSpeed(dev ddnvml.SafeDevice) (float64, error) {
	speed, err := dev.GetClockInfo(nvml.CLOCK_SM)
	return float64(speed), err
}

func getCurrentMemoryClockSpeed(dev ddnvml.SafeDevice) (float64, error) {
	speed, err := dev.GetClockInfo(nvml.CLOCK_MEM)
	return float64(speed), err
}

func getCurrentGraphicsClockSpeed(dev ddnvml.SafeDevice) (float64, error) {
	speed, err := dev.GetClockInfo(nvml.CLOCK_GRAPHICS)
	return float64(speed), err
}

func getCurrentVideoClockSpeed(dev ddnvml.SafeDevice) (float64, error) {
	speed, err := dev.GetClockInfo(nvml.CLOCK_VIDEO)
	return float64(speed), err
}

func getMaxSMClockSpeed(dev ddnvml.SafeDevice) (float64, error) {
	speed, err := dev.GetMaxClockInfo(nvml.CLOCK_SM)
	return float64(speed), err
}

func getMaxMemoryClockSpeed(dev ddnvml.SafeDevice) (float64, error) {
	speed, err := dev.GetMaxClockInfo(nvml.CLOCK_MEM)
	return float64(speed), err
}

func getMaxGraphicsClockSpeed(dev ddnvml.SafeDevice) (float64, error) {
	speed, err := dev.GetMaxClockInfo(nvml.CLOCK_GRAPHICS)
	return float64(speed), err
}

func getMaxVideoClockSpeed(dev ddnvml.SafeDevice) (float64, error) {
	speed, err := dev.GetMaxClockInfo(nvml.CLOCK_VIDEO)
	return float64(speed), err
}

func getTemperature(dev ddnvml.SafeDevice) (float64, error) {
	temp, err := dev.GetTemperature(nvml.TEMPERATURE_GPU)
	return float64(temp), err
}

func getTotalEnergyConsumption(dev ddnvml.SafeDevice) (float64, error) {
	// returns energy in millijoules
	energy, err := dev.GetTotalEnergyConsumption()
	return float64(energy), err
}

func getDeviceCount(dev ddnvml.SafeDevice) (float64, error) {
	r, err := dev.IsMigDeviceHandle()
	if r || err != nil {
		return float64(0), err
	}
	return float64(1), nil
}
