// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvidia

import (
	"fmt"
	"slices"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

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
}

type deviceCollector struct {
	device        nvml.Device
	metricGetters []deviceMetric
}

func newDeviceCollector(device nvml.Device) (Collector, error) {
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
		_, ret := metric.getter(c.device)
		if ret == nvml.ERROR_NOT_SUPPORTED {
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
type deviceMetricGetter func(nvml.Device) (float64, nvml.Return)

// deviceMetric represents a metric that can be collected from an NVML device, using the NVML
// API on that specific device.
type deviceMetric struct {
	name       string
	getter     deviceMetricGetter
	metricType metrics.MetricType
}

// Collect collects all the metrics from the given NVML device.
func (c *deviceCollector) Collect() ([]Metric, error) {
	var err error

	values := make([]Metric, 0, len(c.metricGetters)) // preallocate to reduce allocations
	for _, metric := range c.metricGetters {
		value, ret := metric.getter(c.device)
		if ret != nvml.SUCCESS {
			err = multierror.Append(err, fmt.Errorf("failed to get metric %s: %s", metric.name, nvml.ErrorString(ret)))
			continue
		}

		values = append(values, Metric{
			Name:  metric.name,
			Value: value,
			Type:  metric.metricType,
		})

	}

	return values, err
}

// Name returns the name of the collector.
func (c *deviceCollector) Name() CollectorName {
	return device
}

func getRxPciThroughput(dev nvml.Device) (float64, nvml.Return) {
	// Output in KB/s
	tput, ret := dev.GetPcieThroughput(nvml.PCIE_UTIL_RX_BYTES)
	// Convert to B/S
	return float64(tput) * 1024, ret
}

func getTxPciThroughput(dev nvml.Device) (float64, nvml.Return) {
	// Output in KB/s
	tput, ret := dev.GetPcieThroughput(nvml.PCIE_UTIL_TX_BYTES)
	// Convert to B/S
	return float64(tput) * 1024, ret
}

func getDecoderUtilization(dev nvml.Device) (float64, nvml.Return) {
	// returns utilization from 0-100
	util, _, ret := dev.GetDecoderUtilization()
	return float64(util), ret
}

func getDramActive(dev nvml.Device) (float64, nvml.Return) {
	// returns utilization from 0-100
	util, ret := dev.GetUtilizationRates()
	return float64(util.Memory), ret
}

func getSMActive(dev nvml.Device) (float64, nvml.Return) {
	util, ret := dev.GetUtilizationRates()
	return float64(util.Gpu), ret
}

func getEncoderUtilization(dev nvml.Device) (float64, nvml.Return) {
	// returns utilization from 0-100
	util, _, ret := dev.GetEncoderUtilization()
	return float64(util), ret
}

func getFanSpeed(dev nvml.Device) (float64, nvml.Return) {
	// returns percentage from 0-100 (0 = fan off)
	speed, ret := dev.GetFanSpeed()
	return float64(speed), ret
}

func getPowerManagementLimit(dev nvml.Device) (float64, nvml.Return) {
	// returns power limit in milliwatts
	limit, ret := dev.GetPowerManagementLimit()
	return float64(limit), ret
}

func getPowerUsage(dev nvml.Device) (float64, nvml.Return) {
	// returns power usage in milliwatts
	power, ret := dev.GetPowerUsage()
	return float64(power), ret
}

func getPerformanceState(dev nvml.Device) (float64, nvml.Return) {
	state, ret := dev.GetPerformanceState()
	return float64(state), ret
}

func getCurrentSMClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetClockInfo(nvml.CLOCK_SM)
	return float64(speed), ret
}

func getCurrentMemoryClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetClockInfo(nvml.CLOCK_MEM)
	return float64(speed), ret
}

func getCurrentGraphicsClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetClockInfo(nvml.CLOCK_GRAPHICS)
	return float64(speed), ret
}

func getCurrentVideoClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetClockInfo(nvml.CLOCK_VIDEO)
	return float64(speed), ret
}

func getMaxSMClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_SM)
	return float64(speed), ret
}

func getMaxMemoryClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_MEM)
	return float64(speed), ret
}

func getMaxGraphicsClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_GRAPHICS)
	return float64(speed), ret
}

func getMaxVideoClockSpeed(dev nvml.Device) (float64, nvml.Return) {
	speed, ret := dev.GetMaxClockInfo(nvml.CLOCK_VIDEO)
	return float64(speed), ret
}

func getTemperature(dev nvml.Device) (float64, nvml.Return) {
	temp, ret := dev.GetTemperature(nvml.TEMPERATURE_GPU)
	return float64(temp), ret
}

func getTotalEnergyConsumption(dev nvml.Device) (float64, nvml.Return) {
	// returns energy in millijoules
	energy, ret := dev.GetTotalEnergyConsumption()
	return float64(energy), ret
}
