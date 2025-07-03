// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/hashicorp/go-multierror"

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
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	gpuMetricsNs          = "gpu."
	metricNameCoreUsage   = gpuMetricsNs + "core.usage"
	metricNameCoreLimit   = gpuMetricsNs + "core.limit"
	metricNameMemoryUsage = gpuMetricsNs + "memory.usage"
	metricNameMemoryLimit = gpuMetricsNs + "memory.limit"
)

// Check represents the GPU check that will be periodically executed via the Run() function
type Check struct {
	core.CheckBase
	config         *CheckConfig                // config for the check
	sysProbeClient *sysprobeclient.CheckClient // sysProbeClient is used to communicate with system probe
	activeMetrics  map[model.StatsKey]bool     // activeMetrics is a set of metrics that have been seen in the current check run
	collectors     []nvidia.Collector          // collectors for NVML metrics
	tagger         tagger.Component            // Tagger instance to add tags to outgoing metrics
	telemetry      *checkTelemetry             // Telemetry component to emit internal telemetry
	wmeta          workloadmeta.Component      // Workloadmeta store to get the list of containers
	deviceTags     map[string][]string         // deviceTags is a map of device UUID to tags
	deviceCache    ddnvml.DeviceCache          // deviceCache is a cache of GPU devices
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
		CheckBase:     core.NewCheckBase(CheckName),
		config:        &CheckConfig{},
		activeMetrics: make(map[model.StatsKey]bool),
		tagger:        tagger,
		telemetry:     newCheckTelemetry(telemetry),
		wmeta:         wmeta,
		deviceTags:    make(map[string][]string),
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
	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	if err := yaml.Unmarshal(config, c.config); err != nil {
		return fmt.Errorf("invalid gpu check config: %w", err)
	}

	c.sysProbeClient = sysprobeclient.GetCheckClient(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"))
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
		return err
	}

	collectors, err := nvidia.BuildCollectors(&nvidia.CollectorDependencies{DeviceCache: c.deviceCache})
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

	// build the mapping of GPU devices -> containers to allow tagging device
	// metrics with the tags of containers that are using them
	gpuToContainersMap := c.getGPUToContainersMap()

	if err := c.emitSysprobeMetrics(snd, gpuToContainersMap); err != nil {
		log.Warnf("error while sending sysprobe metrics: %s", err)
	}

	if err := c.emitNvmlMetrics(snd, gpuToContainersMap); err != nil {
		log.Warnf("error while sending NVML metrics: %s", err)
	}

	return nil
}

func (c *Check) emitSysprobeMetrics(snd sender.Sender, gpuToContainersMap map[string][]*workloadmeta.Container) error {
	if err := c.ensureInitDeviceCache(); err != nil {
		return err
	}

	stats, err := sysprobeclient.GetCheck[model.GPUStats](c.sysProbeClient, sysconfig.GPUMonitoringModule)
	if err != nil {
		if sysprobeclient.IgnoreStartupError(err) == nil {
			return nil
		}
		return fmt.Errorf("cannot get data from system-probe: %w", err)
	}

	return c.processSysprobeStats(snd, stats, gpuToContainersMap)
}

func addToActiveEntitiesPerDevice(activeEntitiesPerDevice map[string]common.StringSet, key model.StatsKey, processTags []string) {
	if _, ok := activeEntitiesPerDevice[key.DeviceUUID]; !ok {
		activeEntitiesPerDevice[key.DeviceUUID] = common.NewStringSet()
	}

	for _, t := range processTags {
		activeEntitiesPerDevice[key.DeviceUUID].Add(t)
	}
}

func (c *Check) processSysprobeStats(snd sender.Sender, stats model.GPUStats, gpuToContainersMap map[string][]*workloadmeta.Container) error {
	sentMetrics := 0

	// Always send telemetry metrics
	defer func() {
		c.telemetry.metricsSent.Add(float64(sentMetrics), "system_probe")
		c.telemetry.activeMetrics.Set(float64(len(c.activeMetrics)))
	}()

	// Set all metrics to inactive, so we can remove the ones that we don't see
	// and send the final metrics
	for key := range c.activeMetrics {
		c.activeMetrics[key] = false
	}

	// map each device UUID to the set of tags corresponding to entities (processes) using it
	activeEntitiesPerDevice := make(map[string]common.StringSet)
	for _, dev := range c.deviceCache.All() {
		activeEntitiesPerDevice[dev.GetDeviceInfo().UUID] = common.NewStringSet()
	}

	// Emit the usage metrics
	for _, entry := range stats.Metrics {
		key := entry.Key
		metrics := entry.UtilizationMetrics

		// Get the tags for this metric. We split it between "process" and "device" tags
		// so that we can store which processes are using which devices. That way we will later
		// be able to tag the limit metrics (GPU memory capacity, GPU core count) with the
		// tags of the processes using them.
		processTags := c.getProcessTagsForKey(key)
		deviceTags := c.deviceTags[key.DeviceUUID]

		// Add the process tags to the active entities for the device, using a set to avoid duplicates
		addToActiveEntitiesPerDevice(activeEntitiesPerDevice, key, processTags)

		allTags := append(processTags, deviceTags...)

		snd.Gauge(metricNameCoreUsage, metrics.UsedCores, "", allTags)
		snd.Gauge(metricNameMemoryUsage, float64(metrics.Memory.CurrentBytes), "", allTags)
		sentMetrics += 2

		c.activeMetrics[key] = true
	}

	// Remove the PIDs that we didn't see in this check, and send a metric with a value
	// of zero to ensure it's reset and the previous value doesn't linger on for longer than necessary.
	for key, active := range c.activeMetrics {
		if !active {
			processTags := c.getProcessTagsForKey(key)
			tags := append(processTags, c.deviceTags[key.DeviceUUID]...)
			snd.Gauge(metricNameMemoryUsage, 0, "", tags)
			snd.Gauge(metricNameCoreUsage, 0, "", tags)
			sentMetrics += 2

			// Here we also need to mark these entities as active. If we don't, the limit metrics won't have
			// the tags and utilization will not be reported for them, as the limit metric won't match
			addToActiveEntitiesPerDevice(activeEntitiesPerDevice, key, processTags)

			delete(c.activeMetrics, key)
		}
	}

	// Now, we report the limit metrics tagged with all the processes that are using them
	// Use the list of active processes from system-probe instead of the ActivePIDs from the
	// workloadmeta store, as the latter might not be up-to-date and we want these limit metrics
	// to match the usage metrics reported above
	for _, dev := range c.deviceCache.All() {
		devInfo := dev.GetDeviceInfo()
		deviceTags := c.deviceTags[devInfo.UUID]

		// Retrieve the tags for all the active processes on this device. This will include pid, container
		// tags and will enable matching between the usage of an entity and the corresponding limit.
		activeEntitiesTags := activeEntitiesPerDevice[devInfo.UUID]
		if activeEntitiesTags == nil {
			// Might be nil if there are no active processes on this device
			activeEntitiesTags = common.NewStringSet()
		}

		// Also, add the tags for all containers that have this GPU allocated. Add to the set to avoid repetitions.
		// Adding this ensures we correctly report utilization even if some of the GPUs allocated to the container
		// are not being used.
		for _, container := range gpuToContainersMap[devInfo.UUID] {
			for _, tag := range c.getContainerTags(container.EntityID.ID) {
				activeEntitiesTags.Add(tag)
			}
		}

		allTags := append(deviceTags, activeEntitiesTags.GetAll()...)

		snd.Gauge(metricNameCoreLimit, float64(devInfo.CoreCount), "", allTags)
		snd.Gauge(metricNameMemoryLimit, float64(devInfo.Memory), "", allTags)
	}

	return nil
}

// getProcessTagsForKey returns the process-related tags (PID, containerID) for a given key.
func (c *Check) getProcessTagsForKey(key model.StatsKey) []string {
	// PID is always added
	tags := []string{
		// Per-PID metrics are subject to change due to high cardinality
		fmt.Sprintf("pid:%d", key.PID),
	}

	if key.ContainerID != "" {
		tags = append(tags, c.getContainerTags(key.ContainerID)...)
	}

	return tags
}

func (c *Check) getContainerTags(containerID string) []string {
	// Container ID tag will be added or not depending on the tagger configuration
	containerEntityID := taggertypes.NewEntityID(taggertypes.ContainerID, containerID)
	containerTags, err := c.tagger.Tag(containerEntityID, taggertypes.ChecksConfigCardinality)
	if err != nil {
		log.Errorf("Error collecting container tags for container %s: %s", containerID, err)
	}

	return containerTags
}

func (c *Check) getGPUToContainersMap() map[string][]*workloadmeta.Container {
	containers := c.wmeta.ListContainersWithFilter(func(cont *workloadmeta.Container) bool {
		return len(cont.ResolvedAllocatedResources) > 0
	})

	gpuToContainers := make(map[string][]*workloadmeta.Container)

	for _, container := range containers {
		for _, resource := range container.ResolvedAllocatedResources {
			if gpuutil.IsNvidiaKubernetesResource(resource.Name) {
				gpuToContainers[resource.ID] = append(gpuToContainers[resource.ID], container)
			}
		}
	}

	return gpuToContainers
}

func (c *Check) emitNvmlMetrics(snd sender.Sender, gpuToContainersMap map[string][]*workloadmeta.Container) error {
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

	for deviceUUID, metrics := range perDeviceMetrics {
		deduplicatedMetrics := nvidia.RemoveDuplicateMetrics(metrics)
		c.telemetry.duplicateMetrics.Add(float64(len(metrics)-len(deduplicatedMetrics)), deviceUUID)

		var extraTags []string
		for _, container := range gpuToContainersMap[deviceUUID] {
			entityID := taggertypes.NewEntityID(taggertypes.ContainerID, container.EntityID.ID)
			tags, err := c.tagger.Tag(entityID, taggertypes.ChecksConfigCardinality)
			if err != nil {
				multiErr = multierror.Append(multiErr, fmt.Errorf("error collecting container tags for GPU %s: %w", deviceUUID, err))
				continue
			}

			extraTags = append(extraTags, tags...)
		}

		for _, metric := range deduplicatedMetrics {
			metricName := gpuMetricsNs + metric.Name
			switch metric.Type {
			case ddmetrics.CountType:
				snd.Count(metricName, metric.Value, "", append(c.deviceTags[deviceUUID], extraTags...))
			case ddmetrics.GaugeType:
				snd.Gauge(metricName, metric.Value, "", append(c.deviceTags[deviceUUID], extraTags...))
			default:
				multiErr = multierror.Append(multiErr, fmt.Errorf("unsupported metric type %s for metric %s", metric.Type, metricName))
				continue
			}
		}
	}

	return multiErr
}
