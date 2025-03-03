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
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	gpuMetricsNs          = "gpu."
	metricNameUtil        = gpuMetricsNs + "utilization"
	metricNameMemory      = gpuMetricsNs + "memory.used"
	metricNameMemoryPerc  = gpuMetricsNs + "memory.utilization"
	metricNameDeviceTotal = gpuMetricsNs + "device.total"
)

// Check represents the GPU check that will be periodically executed via the Run() function
type Check struct {
	core.CheckBase
	config         *CheckConfig            // config for the check
	sysProbeClient *http.Client            // sysProbeClient is used to communicate with system probe
	activeMetrics  map[model.StatsKey]bool // activeMetrics is a set of metrics that have been seen in the current check run
	collectors     []nvidia.Collector      // collectors for NVML metrics
	nvmlLib        nvml.Interface          // NVML library interface
	tagger         tagger.Component        // Tagger instance to add tags to outgoing metrics
	telemetry      *checkTelemetry         // Telemetry component to emit internal telemetry
	wmeta          workloadmeta.Component  // Workloadmeta store to get the list of containers
}

type checkTelemetry struct {
	nvmlMetricsSent     telemetry.Counter
	collectorErrors     telemetry.Counter
	activeMetrics       telemetry.Gauge
	sysprobeMetricsSent telemetry.Counter
}

// Factory creates a new check factory
func Factory(tagger tagger.Component, telemetry telemetry.Component, wmeta workloadmeta.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger, telemetry, wmeta)
	})
}

func newCheck(tagger tagger.Component, telemetry telemetry.Component, wmeta workloadmeta.Component) check.Check {
	return &Check{
		CheckBase:     core.NewCheckBase(CheckName),
		config:        &CheckConfig{},
		activeMetrics: make(map[model.StatsKey]bool),
		tagger:        tagger,
		telemetry:     newCheckTelemetry(telemetry),
		wmeta:         wmeta,
	}
}

func newCheckTelemetry(tm telemetry.Component) *checkTelemetry {
	return &checkTelemetry{
		nvmlMetricsSent:     tm.NewCounter(CheckName, "nvml_metrics_sent", []string{"collector"}, "Number of NVML metrics sent"),
		collectorErrors:     tm.NewCounter(CheckName, "collector_errors", []string{"collector"}, "Number of errors from NVML collectors"),
		activeMetrics:       tm.NewGauge(CheckName, "active_metrics", nil, "Number of active metrics"),
		sysprobeMetricsSent: tm.NewCounter(CheckName, "sysprobe_metrics_sent", nil, "Number of metrics sent based on system probe data"),
	}
}

// Configure parses the check configuration and init the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	if err := yaml.Unmarshal(config, c.config); err != nil {
		return fmt.Errorf("invalid gpu check config: %w", err)
	}

	c.sysProbeClient = sysprobeclient.Get(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"))
	return nil
}

func (c *Check) ensureInitNVML() error {
	if c.nvmlLib != nil {
		return nil
	}

	// Initialize NVML library. if the config parameter doesn't exist or is
	// empty string, the default value is used as defined in go-nvml library
	// https://github.com/NVIDIA/go-nvml/blob/main/pkg/nvml/lib.go#L30
	nvmlLib := nvml.New(nvml.WithLibraryPath(c.config.NVMLLibraryPath))
	ret := nvmlLib.Init()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to initialize NVML library: %s", nvml.ErrorString(ret))
	}

	c.nvmlLib = nvmlLib
	return nil
}

// ensureInitCollectors initializes the NVML library and the collectors if they are not already initialized.
// It returns an error if the initialization fails.
func (c *Check) ensureInitCollectors() error {
	if c.collectors != nil {
		return nil
	}

	if err := c.ensureInitNVML(); err != nil {
		return err
	}

	collectors, err := nvidia.BuildCollectors(&nvidia.CollectorDependencies{NVML: c.nvmlLib, Tagger: c.tagger})
	if err != nil {
		return fmt.Errorf("failed to build NVML collectors: %w", err)
	}

	c.collectors = collectors
	return nil
}

// Cancel stops the check
func (c *Check) Cancel() {
	if c.nvmlLib != nil {
		_ = c.nvmlLib.Shutdown()
	}

	c.CheckBase.Cancel()
}

// Run executes the check
func (c *Check) Run() error {
	snd, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}
	// Commit the metrics even in case of an error
	defer snd.Commit()

	if err := c.emitSysprobeMetrics(snd); err != nil {
		log.Warnf("error while sending sysprobe metrics: %s", err)
	}

	if err := c.emitNvmlMetrics(snd); err != nil {
		log.Warnf("error while sending NVML metrics: %s", err)
	}

	return nil
}

func (c *Check) emitSysprobeMetrics(snd sender.Sender) error {
	stats, err := sysprobeclient.GetCheck[model.GPUStats](c.sysProbeClient, sysconfig.GPUMonitoringModule)
	if err != nil {
		return fmt.Errorf("cannot get data from system-probe: %w", err)
	}

	// Set all metrics to inactive, so we can remove the ones that we don't see
	// and send the final metrics
	for key := range c.activeMetrics {
		c.activeMetrics[key] = false
	}

	for _, entry := range stats.Metrics {
		key := entry.Key
		metrics := entry.UtilizationMetrics
		tags := c.getTagsForKey(key)
		snd.Gauge(metricNameUtil, metrics.UtilizationPercentage, "", tags)
		snd.Gauge(metricNameMemory, float64(metrics.Memory.CurrentBytes), "", tags)
		snd.Gauge(metricNameMemoryPerc, metrics.Memory.CurrentBytesPercentage, "", tags)

		c.activeMetrics[key] = true
	}

	c.telemetry.sysprobeMetricsSent.Add(float64(3 * len(stats.Metrics)))

	// Remove the PIDs that we didn't see in this check
	for key, active := range c.activeMetrics {
		if !active {
			tags := c.getTagsForKey(key)
			snd.Gauge(metricNameMemory, 0, "", tags)
			snd.Gauge(metricNameUtil, 0, "", tags)

			delete(c.activeMetrics, key)
		}
	}

	c.telemetry.activeMetrics.Set(float64(len(c.activeMetrics)))

	return nil
}

func (c *Check) getTagsForKey(key model.StatsKey) []string {
	// PID is always added
	tags := []string{
		// Per-PID metrics are subject to change due to high cardinality
		fmt.Sprintf("pid:%d", key.PID),
	}

	// Container ID tag will be added or not depending on the tagger configuration
	containerEntityID := taggertypes.NewEntityID(taggertypes.ContainerID, key.ContainerID)
	containerTags, err := c.tagger.Tag(containerEntityID, c.tagger.ChecksCardinality())
	if err != nil {
		log.Errorf("Error collecting container tags for process %d: %s", key.PID, err)
	} else {
		tags = append(tags, containerTags...)
	}

	gpuEntityID := taggertypes.NewEntityID(taggertypes.GPU, key.DeviceUUID)
	gpuTags, err := c.tagger.Tag(gpuEntityID, c.tagger.ChecksCardinality())
	if err != nil {
		log.Errorf("Error collecting GPU tags for process %d: %s", key.PID, err)
	} else {
		tags = append(tags, gpuTags...)
	}

	return tags
}

func (c *Check) getGPUToContainersMap() map[string][]*workloadmeta.Container {
	containers := c.wmeta.ListContainersWithFilter(func(cont *workloadmeta.Container) bool {
		return len(cont.AllocatedResources) > 0
	})

	gpuToContainers := make(map[string][]*workloadmeta.Container)

	for _, container := range containers {
		for _, resource := range container.AllocatedResources {
			if resource.Name == "nvidia.com/gpu" {
				gpuToContainers[resource.ID] = append(gpuToContainers[resource.ID], container)
			}
		}
	}

	return gpuToContainers
}

func (c *Check) emitNvmlMetrics(snd sender.Sender) error {
	err := c.ensureInitCollectors()
	if err != nil {
		return fmt.Errorf("failed to initialize NVML collectors: %w", err)
	}

	gpuToContainersMap := c.getGPUToContainersMap()
	for _, collector := range c.collectors {
		log.Debugf("Collecting metrics from NVML collector: %s", collector.Name())
		metrics, collectErr := collector.Collect()
		if collectErr != nil {
			c.telemetry.collectorErrors.Add(1, string(collector.Name()))
			err = multierror.Append(err, fmt.Errorf("collector %s failed. %w", collector.Name(), collectErr))
		}

		var extraTags []string
		for _, container := range gpuToContainersMap[collector.DeviceUUID()] {
			entityID := taggertypes.NewEntityID(taggertypes.ContainerID, container.EntityID.ID)
			tags, err := c.tagger.Tag(entityID, c.tagger.ChecksCardinality())
			if err != nil {
				log.Warnf("Error collecting container tags for GPU %s: %s", collector.DeviceUUID(), err)
				continue
			}

			extraTags = append(extraTags, tags...)
		}

		for _, metric := range metrics {
			metricName := gpuMetricsNs + metric.Name
			switch metric.Type {
			case ddmetrics.CountType:
				snd.Count(metricName, metric.Value, "", append(metric.Tags, extraTags...))
			case ddmetrics.GaugeType:
				snd.Gauge(metricName, metric.Value, "", append(metric.Tags, extraTags...))
			default:
				return fmt.Errorf("Unsupported metric type %s for metric %s", metric.Type, metricName)
			}
		}

		c.telemetry.nvmlMetricsSent.Add(float64(len(metrics)), string(collector.Name()))
	}

	return c.emitGlobalNvmlMetrics(snd)
}

func (c *Check) emitGlobalNvmlMetrics(snd sender.Sender) error {
	// Collect global metrics such as device count
	devCount, ret := c.nvmlLib.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get device count: %s", nvml.ErrorString(ret))
	}

	snd.Gauge(metricNameDeviceTotal, float64(devCount), "", nil)

	c.telemetry.nvmlMetricsSent.Add(1, "global")

	return nil
}
