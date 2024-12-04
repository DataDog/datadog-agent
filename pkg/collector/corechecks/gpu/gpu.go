// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"fmt"
	"net/http"

	"gopkg.in/yaml.v2"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/hashicorp/go-multierror"

	sysprobeclient "github.com/DataDog/datadog-agent/cmd/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	gpuMetricsNs     = "gpu."
	metricNameMemory = gpuMetricsNs + "memory"
	metricNameUtil   = gpuMetricsNs + "utilization"
	metricNameMaxMem = gpuMetricsNs + "memory.max"
)

// Check represents the GPU check that will be periodically executed via the Run() function
type Check struct {
	core.CheckBase
	config         *CheckConfig            // config for the check
	sysProbeClient *http.Client            // sysProbeClient is used to communicate with system probe
	activeMetrics  map[model.StatsKey]bool // activeMetrics is a set of metrics that have been seen in the current check run
	collectors     []nvidia.Collector      // collectors for NVML metrics
	nvmlLib        nvml.Interface          // NVML library interface
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase:     core.NewCheckBase(CheckName),
		config:        &CheckConfig{},
		activeMetrics: make(map[model.StatsKey]bool),
	}
}

// Configure parses the check configuration and init the check
func (m *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := m.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	if err := yaml.Unmarshal(config, m.config); err != nil {
		return fmt.Errorf("invalid gpu check config: %w", err)
	}

	// Initialize NVML collectors. if the config parameter doesn't exist or is
	// empty string, the default value is used as defined in go-nvml library
	// https://github.com/NVIDIA/go-nvml/blob/main/pkg/nvml/lib.go#L30
	m.nvmlLib = nvml.New(nvml.WithLibraryPath(m.config.NVMLLibraryPath))
	ret := m.nvmlLib.Init()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to initialize NVML library: %s", nvml.ErrorString(ret))
	}

	var err error
	m.collectors, err = nvidia.BuildCollectors(m.nvmlLib)
	if err != nil {
		return fmt.Errorf("failed to build NVML collectors: %w", err)
	}

	m.sysProbeClient = sysprobeclient.Get(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"))
	return nil
}

// Cancel stops the check
func (m *Check) Cancel() {
	if m.nvmlLib != nil {
		_ = m.nvmlLib.Shutdown()
	}

	m.CheckBase.Cancel()
}

// Run executes the check
func (m *Check) Run() error {
	snd, err := m.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}
	// Commit the metrics even in case of an error
	defer snd.Commit()

	if err := m.emitSysprobeMetrics(snd); err != nil {
		log.Warnf("error while sending sysprobe metrics: %s", err)
	}

	if err := m.emitNvmlMetrics(snd); err != nil {
		log.Warnf("error while sending NVML metrics: %s", err)
	}

	return nil
}

func (m *Check) emitSysprobeMetrics(snd sender.Sender) error {
	stats, err := sysprobeclient.GetCheck[model.GPUStats](m.sysProbeClient, sysconfig.GPUMonitoringModule)
	if err != nil {
		return fmt.Errorf("cannot get data from system-probe: %w", err)
	}

	// Set all metrics to inactive, so we can remove the ones that we don't see
	// and send the final metrics
	for key := range m.activeMetrics {
		m.activeMetrics[key] = false
	}

	for _, entry := range stats.Metrics {
		key := entry.Key
		metrics := entry.UtilizationMetrics
		tags := getTagsForKey(key)
		snd.Gauge(metricNameUtil, metrics.UtilizationPercentage, "", tags)
		snd.Gauge(metricNameMemory, float64(metrics.Memory.CurrentBytes), "", tags)
		snd.Gauge(metricNameMaxMem, float64(metrics.Memory.MaxBytes), "", tags)

		m.activeMetrics[key] = true
	}

	// Remove the PIDs that we didn't see in this check
	for key, active := range m.activeMetrics {
		if !active {
			tags := getTagsForKey(key)
			snd.Gauge(metricNameMemory, 0, "", tags)
			snd.Gauge(metricNameMaxMem, 0, "", tags)
			snd.Gauge(metricNameUtil, 0, "", tags)

			delete(m.activeMetrics, key)
		}
	}

	return nil
}

func getTagsForKey(key model.StatsKey) []string {
	// Per-PID metrics are subject to change due to high cardinality
	return []string{
		fmt.Sprintf("pid:%d", key.PID),
		fmt.Sprintf("gpu_uuid:%s", key.DeviceUUID),
	}
}

func (m *Check) emitNvmlMetrics(snd sender.Sender) error {
	var err error

	for _, collector := range m.collectors {
		log.Debugf("Collecting metrics from NVML collector: %s", collector.Name())
		metrics, collectErr := collector.Collect()
		if collectErr != nil {
			err = multierror.Append(err, fmt.Errorf("collector %s failed. %w", collector.Name(), collectErr))
		}

		for _, metric := range metrics {
			metricName := gpuMetricsNs + metric.Name
			snd.Gauge(metricName, metric.Value, "", metric.Tags)
		}
	}

	return err
}
