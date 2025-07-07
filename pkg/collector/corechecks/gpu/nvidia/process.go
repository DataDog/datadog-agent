//go:build linux && nvml

package nvidia

import (
	"fmt"

	"github.com/hashicorp/go-multierror"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type apiCallInfo struct {
	name     string
	testFunc func(ddnvml.Device) error
	callFunc func(*processCollector) ([]Metric, error)
}

var apiCallFactory = []apiCallInfo{
	{
		name: "compute_processes",
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
	supportedApiCalls []apiCallInfo
}

func newProcessCollector(device ddnvml.Device) (Collector, error) {
	c := &processCollector{device: device}

	c.removeUnsupportedMetrics()
	if len(c.supportedApiCalls) == 0 {
		return nil, errUnsupportedDevice
	}

	return c, nil
}

func (c *processCollector) removeUnsupportedMetrics() {
	for _, apiCall := range apiCallFactory {
		err := apiCall.testFunc(c.device)
		if err == nil || !ddnvml.IsUnsupported(err) {
			c.supportedApiCalls = append(c.supportedApiCalls, apiCall)
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

	for _, apiCall := range c.supportedApiCalls {
		collectedMetrics, err := apiCall.callFunc(c)
		if err != nil {
			multiErr = multierror.Append(multiErr, fmt.Errorf("failed to call %s: %w", apiCall.name, err))
			continue
		}

		allMetrics = append(allMetrics, collectedMetrics...)
	}

	return allMetrics, multiErr
}

// Helper methods for metric collection
func (c *processCollector) collectComputeProcesses() ([]Metric, error) {
	procs, err := c.device.GetComputeRunningProcesses()
	if err != nil {
		return nil, err
	}

	devInfo := c.device.GetDeviceInfo()
	var processMetrics []Metric
	for _, proc := range procs {
		pidTag := []string{fmt.Sprintf("pid:%d", proc.Pid)}
		processMetrics = append(processMetrics,
			Metric{Name: "memory.usage", Value: float64(proc.UsedGpuMemory), Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "memory.limit", Value: float64(devInfo.Memory), Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "core.limit", Value: float64(devInfo.CoreCount), Type: metrics.GaugeType, Tags: pidTag},
		)
	}
	return processMetrics, nil
}

func (c *processCollector) collectProcessUtilization() ([]Metric, error) {
	processSamples, err := c.device.GetProcessUtilization(c.lastTimestamp)
	if err != nil {
		return nil, err
	}

	var utilizationMetrics []Metric
	for _, sample := range processSamples {
		pidTag := []string{fmt.Sprintf("pid:%d", sample.Pid)}
		utilizationMetrics = append(utilizationMetrics,
			Metric{Name: "core.utilization", Value: float64(sample.SmUtil), Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "dram_active", Value: float64(sample.MemUtil), Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "encoder_utilization", Value: float64(sample.EncUtil), Type: metrics.GaugeType, Tags: pidTag},
			Metric{Name: "decoder_utilization", Value: float64(sample.DecUtil), Type: metrics.GaugeType, Tags: pidTag},
		)

		//update the last timestamp if the current sample's timestamp is greater
		if sample.TimeStamp > c.lastTimestamp {
			c.lastTimestamp = sample.TimeStamp
		}
	}
	return utilizationMetrics, nil
}
