// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

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
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/nvml"
)

const (
	gpuMetricsNs          = "gpu."
	metricNameCoreUsage   = gpuMetricsNs + "core.usage"
	metricNameCoreLimit   = gpuMetricsNs + "core.limit"
	metricNameMemoryUsage = gpuMetricsNs + "memory.usage"
	metricNameMemoryLimit = gpuMetricsNs + "memory.limit"
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
	deviceTags     map[string][]string     // deviceTags is a map of device UUID to tags
	deviceCache    ddnvml.DeviceCache      // deviceCache is a cache of GPU devices
}

type checkTelemetry struct {
	metricsSent     telemetry.Counter
	collectorErrors telemetry.Counter
	activeMetrics   telemetry.Gauge
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
		metricsSent:     tm.NewCounter(CheckName, "metrics_sent", []string{"collector"}, "Number of GPU metrics sent"),
		collectorErrors: tm.NewCounter(CheckName, "collector_errors", []string{"collector"}, "Number of errors from NVML collectors"),
		activeMetrics:   tm.NewGauge(CheckName, "active_metrics", nil, "Number of active metrics"),
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

func (c *Check) ensureInitDeviceCache() error {
	if c.deviceCache != nil {
		return nil
	}

	if err := c.ensureInitNVML(); err != nil {
		return err
	}

	var err error
	c.deviceCache, err = ddnvml.NewDeviceCacheWithOptions(c.nvmlLib)
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

	collectors, err := nvidia.BuildCollectors(&nvidia.CollectorDependencies{NVML: c.nvmlLib, DeviceCache: c.deviceCache})
	if err != nil {
		return fmt.Errorf("failed to build NVML collectors: %w", err)
	}

	c.collectors = collectors
	c.deviceTags = nvidia.GetDeviceTagsMapping(c.deviceCache, c.tagger)
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
	sentMetrics := 0

	// Always send telemetry metrics
	defer func() {
		c.telemetry.metricsSent.Add(float64(sentMetrics), "system_probe")
		c.telemetry.activeMetrics.Set(float64(len(c.activeMetrics)))
	}()

	if err := c.ensureInitDeviceCache(); err != nil {
		return err
	}

	stats, err := sysprobeclient.GetCheck[model.GPUStats](c.sysProbeClient, sysconfig.GPUMonitoringModule)
	if err != nil {
		return fmt.Errorf("cannot get data from system-probe: %w", err)
	}

	// Set all metrics to inactive, so we can remove the ones that we don't see
	// and send the final metrics
	for key := range c.activeMetrics {
		c.activeMetrics[key] = false
	}

	// map each device UUID to the set of tags corresponding to entities (processes) using it
	activeEntitiesPerDevice := make(map[string]common.StringSet)
	for _, dev := range c.deviceCache.All() {
		activeEntitiesPerDevice[dev.UUID] = common.NewStringSet()
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
		if _, ok := activeEntitiesPerDevice[key.DeviceUUID]; !ok {
			activeEntitiesPerDevice[key.DeviceUUID] = common.NewStringSet()
		}

		for _, t := range processTags {
			activeEntitiesPerDevice[key.DeviceUUID].Add(t)
		}

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
			tags := append(c.getProcessTagsForKey(key), c.deviceTags[key.DeviceUUID]...)
			snd.Gauge(metricNameMemoryUsage, 0, "", tags)
			snd.Gauge(metricNameCoreUsage, 0, "", tags)
			sentMetrics += 2

			delete(c.activeMetrics, key)
		}
	}

	// Now, we report the limit metrics tagged with all the processes that are using them
	// Use the list of active processes from system-probe instead of the ActivePIDs from the
	// workloadmeta store, as the latter might not be up-to-date and we want these limit metrics
	// to match the usage metrics reported above
	for _, dev := range c.deviceCache.All() {
		deviceTags := c.deviceTags[dev.UUID]

		// Retrieve the tags for all the active processes on this device. This will include pid, container
		// tags and will enable matching between the usage of an entity and the corresponding limit.
		activeEntitiesTags := activeEntitiesPerDevice[dev.UUID]
		if activeEntitiesTags == nil {
			// Might be nil if there are no active processes on this device
			activeEntitiesTags = common.NewStringSet()
		}

		// Also, add the tags for all containers that have this GPU allocated. Add to the set to avoid repetitions.
		// Adding this ensures we correctly report utilization even if some of the GPUs allocated to the container
		// are not being used.
		for _, container := range gpuToContainersMap[dev.UUID] {
			for _, tag := range c.getContainerTags(container.EntityID.ID) {
				activeEntitiesTags.Add(tag)
			}
		}

		allTags := append(deviceTags, activeEntitiesTags.GetAll()...)

		snd.Gauge(metricNameCoreLimit, float64(dev.CoreCount), "", allTags)
		snd.Gauge(metricNameMemoryLimit, float64(dev.Memory), "", allTags)
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

	tags = append(tags, c.getContainerTags(key.ContainerID)...)

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

func (c *Check) emitNvmlMetrics(snd sender.Sender, gpuToContainersMap map[string][]*workloadmeta.Container) error {
	err := c.ensureInitCollectors()
	if err != nil {
		return fmt.Errorf("failed to initialize NVML collectors: %w", err)
	}

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
			tags, err := c.tagger.Tag(entityID, taggertypes.ChecksConfigCardinality)
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
				snd.Count(metricName, metric.Value, "", append(c.deviceTags[collector.DeviceUUID()], extraTags...))
			case ddmetrics.GaugeType:
				snd.Gauge(metricName, metric.Value, "", append(c.deviceTags[collector.DeviceUUID()], extraTags...))
			default:
				return fmt.Errorf("unsupported metric type %s for metric %s", metric.Type, metricName)
			}
		}

		c.telemetry.metricsSent.Add(float64(len(metrics)), string(collector.Name()))
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

	c.telemetry.metricsSent.Add(1, "global")

	return nil
}
