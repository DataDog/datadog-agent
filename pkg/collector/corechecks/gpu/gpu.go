// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

// Package gpu contains gpum core-check implementation.
package gpu

import (
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/containers"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName defines the name of the GPU check
	CheckName             = "gpu"
	gpuMetricsNs          = "gpu."
	metricNameCoreUsage   = gpuMetricsNs + "core.usage"
	metricNameCoreLimit   = gpuMetricsNs + "core.limit"
	metricNameMemoryUsage = gpuMetricsNs + "memory.usage"
	metricNameMemoryLimit = gpuMetricsNs + "memory.limit"
)

// logLimitCheck is used to limit the number of times we log messages about streams and cuda events, as that can be very verbose
var logLimitCheck = log.NewLogLimit(20, 10*time.Minute)

// Check represents the GPU check that will be periodically executed via the Run() function
type Check struct {
	core.CheckBase
	collectors                   []nvidia.Collector       // collectors for NVML metrics
	tagger                       tagger.Component         // Tagger instance to add tags to outgoing metrics
	telemetry                    *checkTelemetry          // Telemetry component to emit internal telemetry
	wmeta                        workloadmeta.Component   // Workloadmeta store to get the list of containers
	deviceTags                   map[string][]string      // deviceTags is a map of device UUID to tags
	deviceCache                  ddnvml.DeviceCache       // deviceCache is a cache of GPU devices
	useSystemProbeProcessMetrics bool                     // useSystemProbeProcessMetrics determines if SP process metrics are preferred over NVML process collectors
	spCache                      *nvidia.SystemProbeCache // spCache manages system-probe GPU stats and client (only initialized when useSystemProbeProcessMetrics is true)
}

type checkTelemetry struct {
	metricsSent      telemetry.Counter
	duplicateMetrics telemetry.Counter
	collectorErrors  telemetry.Counter
	activeMetrics    telemetry.Gauge
}

// Factory creates a new check factory
func Factory(tagger tagger.Component, telemetry telemetry.Component, wmeta workloadmeta.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger, telemetry, wmeta)
	})
}

func newCheck(tagger tagger.Component, telemetry telemetry.Component, wmeta workloadmeta.Component) check.Check {
	return &Check{
		CheckBase:  core.NewCheckBase(CheckName),
		tagger:     tagger,
		telemetry:  newCheckTelemetry(telemetry),
		wmeta:      wmeta,
		deviceTags: make(map[string][]string),
	}
}

func newCheckTelemetry(tm telemetry.Component) *checkTelemetry {
	return &checkTelemetry{
		metricsSent:      tm.NewCounter(CheckName, "metrics_sent", []string{"collector"}, "Number of GPU metrics sent"),
		collectorErrors:  tm.NewCounter(CheckName, "collector_errors", []string{"collector"}, "Number of errors from NVML collectors"),
		activeMetrics:    tm.NewGauge(CheckName, "active_metrics", nil, "Number of active metrics"),
		duplicateMetrics: tm.NewCounter(CheckName, "duplicate_metrics", []string{"device"}, "Number of duplicate metrics removed from NVML collectors due to priority de-duplication"),
	}
}

// Configure parses the check configuration and init the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	// Check if GPU check is enabled (follows SBOM pattern)
	if !pkgconfigsetup.Datadog().GetBool("gpum.enabled") {
		return fmt.Errorf("GPU check is disabled")
	}

	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	// Compute whether we should prefer system-probe process metrics
	gpuProbeEnabled := pkgconfigsetup.SystemProbe().GetBool("gpu_monitoring.enabled")
	preferSP := pkgconfigsetup.Datadog().GetBool("gpum.use_sp_process_metrics")
	c.useSystemProbeProcessMetrics = gpuProbeEnabled && preferSP

	// Initialize system-probe cache only if we actually want to use SP process metrics
	if c.useSystemProbeProcessMetrics {
		c.spCache = nvidia.NewSystemProbeCache()
	}

	return nil
}

func (c *Check) ensureInitDeviceCache() error {
	if c.deviceCache != nil {
		return nil
	}

	var err error
	c.deviceCache, err = ddnvml.NewDeviceCache()
	if err != nil {
		return fmt.Errorf("failed to initialize device cache: %w", err)
	}

	return nil
}

// ensureInitCollectors initializes the NVML library and the collectors if they are not already initialized.
// It returns an error if the initialization fails.
func (c *Check) ensureInitCollectors() error {
	//TODO: in the future we need to support hot-plugging of GPU devices,
	// as we currently create a collector per GPU device.
	// also we map the device tags in this function only once, so new hot-lugged devices won't have the tags
	if c.collectors != nil {
		return nil
	}

	if err := c.ensureInitDeviceCache(); err != nil {
		return fmt.Errorf("failed to initialize device cache: %w", err)
	}

	collectors, err := nvidia.BuildCollectors(&nvidia.CollectorDependencies{DeviceCache: c.deviceCache}, c.spCache)
	if err != nil {
		return fmt.Errorf("failed to build NVML collectors: %w", err)
	}

	c.collectors = collectors
	c.deviceTags = nvidia.GetDeviceTagsMapping(c.deviceCache, c.tagger)
	return nil
}

// Cancel stops the check
func (c *Check) Cancel() {
	if lib, err := ddnvml.GetSafeNvmlLib(); err == nil {
		_ = lib.Shutdown()
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

	if err := c.ensureInitDeviceCache(); err != nil {
		return fmt.Errorf("failed to initialize device cache: %w", err)
	}

	// Refresh SP cache before collecting metrics (if using SP process metrics)
	if c.useSystemProbeProcessMetrics && c.spCache != nil {
		if err := c.spCache.Refresh(); err != nil {
			log.Warnf("error refreshing system-probe cache: %v", err)
			// Continue with NVML-only metrics, SP collectors will return empty metrics
		}
	}

	// build the mapping of GPU devices -> containers to allow tagging device
	// metrics with the tags of containers that are using them
	gpuToContainersMap := c.getGPUToContainersMap()

	if err := c.emitMetrics(snd, gpuToContainersMap); err != nil {
		log.Warnf("error while sending gpum metrics: %s", err)
	}

	return nil
}

func (c *Check) getGPUToContainersMap() map[string]*workloadmeta.Container {
	gpuToContainers := make(map[string]*workloadmeta.Container)

	for _, container := range c.wmeta.ListContainersWithFilter(containers.HasGPUs) {
		containerDevices, err := containers.MatchContainerDevices(container, c.deviceCache.All())
		if err != nil && logLimitCheck.ShouldLog() {
			log.Warnf("error matching container devices: %s. Will continue with the available devices", err)
		}

		// despite an error, we still might have some devices assigned to the container
		for _, device := range containerDevices {
			gpuToContainers[device.GetDeviceInfo().UUID] = container
		}
	}

	return gpuToContainers
}

func (c *Check) emitMetrics(snd sender.Sender, gpuToContainersMap map[string]*workloadmeta.Container) error {
	err := c.ensureInitCollectors()
	if err != nil {
		return fmt.Errorf("failed to initialize NVML collectors: %w", err)
	}

	perDeviceMetrics := make(map[string][]nvidia.Metric)

	var multiErr error
	for _, collector := range c.collectors {
		log.Debugf("Collecting metrics from NVML collector: %s", collector.Name())
		metrics, collectErr := collector.Collect()
		if collectErr != nil {
			c.telemetry.collectorErrors.Add(1, string(collector.Name()))
			multiErr = multierror.Append(multiErr, fmt.Errorf("collector %s failed. %w", collector.Name(), collectErr))
		}

		if len(metrics) > 0 {
			perDeviceMetrics[collector.DeviceUUID()] = append(perDeviceMetrics[collector.DeviceUUID()], metrics...)
		}

		c.telemetry.metricsSent.Add(float64(len(metrics)), string(collector.Name()))
	}

	// Cache container tags to avoid repeated tagger calls for the same container
	containerTagsCache := make(map[string][]string)

	//iterate through devices to emit its metrics
	for deviceUUID, metrics := range perDeviceMetrics {
		//filter out same metric with lower priority
		deduplicatedMetrics := nvidia.RemoveDuplicateMetrics(metrics)
		c.telemetry.duplicateMetrics.Add(float64(len(metrics)-len(deduplicatedMetrics)), deviceUUID)

		var containerTags []string
		if container := gpuToContainersMap[deviceUUID]; container != nil {
			containerID := container.EntityID.ID

			// Check cache first
			if cachedTags, exists := containerTagsCache[containerID]; exists {
				containerTags = cachedTags // Direct reference, no copy
			} else {
				// Fetch and cache container tags
				entityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)
				// we use orchestrator cardinality here to ensure we get the pod_name tag
				// ref: https://docs.datadoghq.com/containers/kubernetes/tag/?tab=datadogoperator#out-of-the-box-tags
				tags, err := c.tagger.Tag(entityID, taggertypes.OrchestratorCardinality)
				if err != nil {
					multiErr = multierror.Append(multiErr, fmt.Errorf("error collecting container tags for GPU %s: %w", deviceUUID, err))
					containerTagsCache[containerID] = nil // Cache the error state to avoid repeated calls
				} else {
					containerTagsCache[containerID] = tags
					containerTags = tags // Direct reference, no copy
				}
			}
		}

		// iterate through filtered metrics and emit them with the tags
		for _, metric := range deduplicatedMetrics {
			metricName := gpuMetricsNs + metric.Name
			allTags := append(append(c.deviceTags[deviceUUID], containerTags...), metric.Tags...)

			switch metric.Type {
			case ddmetrics.CountType:
				snd.Count(metricName, metric.Value, "", allTags)
			case ddmetrics.GaugeType:
				snd.Gauge(metricName, metric.Value, "", allTags)
			default:
				multiErr = multierror.Append(multiErr, fmt.Errorf("unsupported metric type %s for metric %s", metric.Type, metricName))
				continue
			}
		}
	}

	return multiErr
}
