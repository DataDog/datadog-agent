// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package amd

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	amdgpu "github.com/DataDog/datadog-agent/pkg/gpu/amd"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

const (
	// exporterCollectorName is the collector name used in telemetry for the
	// AMD device metrics exporter.
	exporterCollectorName nvidia.CollectorName = "amd_exporter"
	// defaultExporterTimeout bounds a /gpumetrics request.
	defaultExporterTimeout = 5 * time.Second
	// maxExporterResponseBytes bounds a /gpumetrics response, which is a few
	// KB per GPU.
	maxExporterResponseBytes = 4 << 20
)

// exporterInstanceConfig is the instance configuration of the
// amd_gpu_exporter check.
type exporterInstanceConfig struct {
	// GPUMetricsURL is the /gpumetrics endpoint of the AMD device metrics
	// exporter, e.g. http://localhost:5000/gpumetrics.
	GPUMetricsURL string `yaml:"gpumetrics_url"`
	// Timeout is the request timeout in seconds.
	Timeout float64 `yaml:"timeout"`
}

func parseExporterInstanceConfig(data []byte) (exporterInstanceConfig, error) {
	var cfg exporterInstanceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse instance configuration: %w", err)
	}
	if cfg.GPUMetricsURL == "" {
		return cfg, errors.New("gpumetrics_url is required")
	}
	return cfg, nil
}

// throttleReason is a throttling cause reported in the violation statistics.
type throttleReason struct {
	tag       string
	residency func(*amdgpu.ViolationStats) uint64
}

var throttleReasons = []throttleReason{
	{"ppt", func(v *amdgpu.ViolationStats) uint64 { return v.PPTResidencyAccumulated }},
	{"socket_thermal", func(v *amdgpu.ViolationStats) uint64 { return v.SocketThermalResidencyAccumulated }},
	{"hbm_thermal", func(v *amdgpu.ViolationStats) uint64 { return v.HBMThermalResidencyAccumulated }},
	{"vr_thermal", func(v *amdgpu.ViolationStats) uint64 { return v.VRThermalResidencyAccumulated }},
	{"processor_hot", func(v *amdgpu.ViolationStats) uint64 { return v.ProcessorHotResidencyAccumulated }},
}

// exporterCollector reads the XGMI and throttling statistics of AMD GPUs from
// the /gpumetrics endpoint of the AMD device metrics exporter.
type exporterCollector struct {
	url        string
	client     *http.Client
	violations map[string]amdgpu.ViolationStats // violations holds the last violation statistics, by device UUID
}

func newExporterCollector(cfg exporterInstanceConfig) *exporterCollector {
	timeout := defaultExporterTimeout
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout * float64(time.Second))
	}
	return &exporterCollector{
		url:        cfg.GPUMetricsURL,
		client:     &http.Client{Timeout: timeout},
		violations: make(map[string]amdgpu.ViolationStats),
	}
}

func (e *exporterCollector) fetch() (*amdgpu.GPUMetrics, error) {
	resp, err := e.client.Get(e.url)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", e.url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query %s: %s", e.url, resp.Status)
	}
	return amdgpu.ParseGPUMetrics(http.MaxBytesReader(nil, resp.Body, maxExporterResponseBytes))
}

// collect returns the exporter samples of the devices, joined to the exporter
// GPUs by PCI address. A failed request is reported on the first device.
func (e *exporterCollector) collect(devices []*amdgpu.Device) []gpu.CollectorSamples {
	if len(devices) == 0 {
		return nil
	}
	start := time.Now()
	response, err := e.fetch()
	duration := time.Since(start)

	var byBDF map[string]*amdgpu.GPUMetricsEntry
	if response != nil {
		byBDF = response.ByPCIBusID()
	}
	results := make([]gpu.CollectorSamples, 0, len(devices))
	reported := make(map[string]struct{}, len(devices))
	for i, dev := range devices {
		result := gpu.CollectorSamples{
			Name:          exporterCollectorName,
			DeviceUUID:    dev.UUID,
			TelemetryTags: exporterTelemetryTags(dev),
		}
		if i == 0 {
			result.Err = err
			result.Duration = duration
		}
		if entry, ok := byBDF[dev.PCIBusID]; ok {
			result.Samples = append(xgmiSamples(dev, entry), e.throttleSamples(dev.UUID, entry)...)
			reported[dev.UUID] = struct{}{}
		}
		results = append(results, result)
	}
	if err == nil {
		// Forget devices that are gone, so that a returning one starts a new baseline.
		for uuid := range e.violations {
			if _, ok := reported[uuid]; !ok {
				delete(e.violations, uuid)
			}
		}
	}
	return results
}

// xgmiSamples returns the per-link and total XGMI throughput of a device in a
// hive with at least one peer. The counters are accumulated KB: the rate
// calculator turns them into KB/s.
func xgmiSamples(dev *amdgpu.Device, entry *amdgpu.GPUMetricsEntry) []nvidia.Sample {
	if dev.XGMIHiveSize() < 2 || len(entry.Stats.XGMILinkStats) == 0 {
		return nil
	}
	var samples []nvidia.Sample
	var rxTotal, txTotal uint64
	var rxSeen, txSeen bool
	for i, link := range entry.Stats.XGMILinkStats {
		tags := []string{"xgmi_link:" + strconv.Itoa(i)}
		if amdgpu.Supported(link.DataRead) {
			samples = append(samples, rateMetric("xgmi.throughput.raw.rx", link.DataRead, tags))
			rxTotal += link.DataRead
			rxSeen = true
		}
		if amdgpu.Supported(link.DataWrite) {
			samples = append(samples, rateMetric("xgmi.throughput.raw.tx", link.DataWrite, tags))
			txTotal += link.DataWrite
			txSeen = true
		}
	}
	if rxSeen {
		samples = append(samples, rateMetric("xgmi.throughput.raw.rx.total", rxTotal, nil))
	}
	if txSeen {
		samples = append(samples, rateMetric("xgmi.throughput.raw.tx.total", txTotal, nil))
	}
	return samples
}

func rateMetric(name string, value uint64, tags []string) *nvidia.Metric {
	m := nvidia.NewMetric(name, float64(value), metrics.GaugeType, nvidia.Low, tags, nil)
	m.RateCalculationMode = nvidia.PerSecondRateCalculation
	return m
}

// throttleSamples returns the share of the interval, in percent, during which
// the device was throttled for each reason. The first reading only sets the
// baseline.
func (e *exporterCollector) throttleSamples(deviceUUID string, entry *amdgpu.GPUMetricsEntry) []nvidia.Sample {
	current := entry.Stats.ViolationStats
	if current == nil || !amdgpu.Supported(current.CurrentAccumulatedCounter) {
		return nil
	}
	previous, seen := e.violations[deviceUUID]
	e.violations[deviceUUID] = *current
	if !seen || current.CurrentAccumulatedCounter <= previous.CurrentAccumulatedCounter {
		return nil
	}
	elapsed := float64(current.CurrentAccumulatedCounter - previous.CurrentAccumulatedCounter)

	var samples []nvidia.Sample
	for _, reason := range throttleReasons {
		cur, prev := reason.residency(current), reason.residency(&previous)
		if !amdgpu.Supported(cur) || !amdgpu.Supported(prev) || cur < prev {
			continue
		}
		pct := min(float64(cur-prev)/elapsed*100, 100)
		samples = append(samples, nvidia.NewMetric("throttle.residency", pct, metrics.GaugeType, nvidia.Low, []string{"reason:" + reason.tag}, nil))
	}
	return samples
}

// exporterTelemetryTags returns the collector telemetry tag values of a device
// for the exporter collector.
func exporterTelemetryTags(dev *amdgpu.Device) []string {
	tags := amdTelemetryTags(dev)
	tags[0] = string(exporterCollectorName)
	return tags
}
