// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

// Package gpu contains gpu core-check implementation.
package gpu

import (
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	agenterrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/gpu/containers"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	gpuMetricsNs = "gpu."
)

// logLimitCheck is used to limit the number of times we log messages about streams and cuda events, as that can be very verbose
var logLimitCheck = log.NewLogLimit(20, 10*time.Minute)

// Check represents the GPU check that will be periodically executed via the Run() function
type Check struct {
	core.CheckBase
	collectors         []nvidia.Collector               // collectors for NVML metrics
	disabledCollectors []string                         // disabledCollectors is a list of collector names that should not be created
	tagger             tagger.Component                 // Tagger instance to add tags to outgoing metrics
	telemetry          *checkTelemetry                  // Internal telemetry metrics for the check
	wmeta              workloadmeta.Component           // Workloadmeta store to get the list of containers
	deviceTags         map[string][]string              // deviceTags is a map of device UUID to tags
	deviceCache        ddnvml.DeviceCache               // deviceCache is a cache of GPU devices
	spCache            *nvidia.SystemProbeCache         // spCache manages system-probe GPU stats and client (only initialized when gpu_monitoring is enabled in system-probe)
	deviceEvtGatherer  *nvidia.DeviceEventsGatherer     // deviceEvtGatherer asynchronously listens for device events and gathers them
	workloadTagCache   *WorkloadTagCache                // workloadTagCache caches workload tags for GPU metrics
	containerProvider  proccontainers.ContainerProvider // containerProvider is used as a fallback to get a PID -> CID mapping when workloadmeta does not have the process data
}

type checkTelemetry struct {
	collectorTelemetry *nvidia.CollectorTelemetry // collectorTelemetry holds specific telemetry for the collectors, it will also be passed to the collector dependencies
	metrics            *checkTelemetryMetrics     // metrics holds the metrics for the check
	component          telemetry.Component        // telemetry component, used to create the telemetry metrics
	nvmlState          *ddnvml.NvmlStateTelemetry // nvmlState tracks the state of the NVML library
}

type checkTelemetryMetrics struct {
	metricsSent                telemetry.Counter
	duplicateMetrics           telemetry.Counter
	activeMetrics              telemetry.Gauge
	missingContainerGpuMapping telemetry.Counter
	deviceCount                telemetry.Gauge // emitted as a telemetry metric too in order to send it through COAT
}

// Factory creates a new check factory
func Factory(tagger tagger.Component, telemetry telemetry.Component, wmeta workloadmeta.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger, telemetry, wmeta)
	})
}

func newCheck(tagger tagger.Component, telemetry telemetry.Component, wmeta workloadmeta.Component) check.Check {
	return &Check{
		CheckBase:   core.NewCheckBase(CheckName),
		tagger:      tagger,
		telemetry:   newCheckTelemetry(telemetry),
		wmeta:       wmeta,
		deviceTags:  make(map[string][]string),
		deviceCache: ddnvml.NewDeviceCache(),
	}
}

// NewCheck creates a new GPU check instance. This is exported for integration testing.
func NewCheck(tagger tagger.Component, telemetry telemetry.Component, wmeta workloadmeta.Component) check.Check {
	return newCheck(tagger, telemetry, wmeta)
}

// SetContainerProvider sets the container provider on the Check.
// This is exported for integration testing.
func (c *Check) SetContainerProvider(provider proccontainers.ContainerProvider) {
	c.containerProvider = provider
}

func newCheckTelemetry(tm telemetry.Component) *checkTelemetry {
	return &checkTelemetry{
		metrics:            newCheckTelemetryMetrics(tm),
		component:          tm,
		nvmlState:          ddnvml.NewNvmlStateTelemetry(tm),
		collectorTelemetry: nvidia.NewCollectorTelemetry(tm),
	}
}

func newCheckTelemetryMetrics(tm telemetry.Component) *checkTelemetryMetrics {
	return &checkTelemetryMetrics{
		metricsSent:                tm.NewCounter(CheckName, "metrics_sent", []string{"collector"}, "Number of GPU metrics sent"),
		activeMetrics:              tm.NewGauge(CheckName, "active_metrics", nil, "Number of active metrics"),
		duplicateMetrics:           tm.NewCounter(CheckName, "duplicate_metrics", []string{"device"}, "Number of duplicate metrics removed from NVML collectors due to priority de-duplication"),
		missingContainerGpuMapping: tm.NewCounter(CheckName, "missing_container_gpu_mapping", []string{"container_name"}, "Number of containers with no matching GPU device"),
		deviceCount:                tm.NewGauge(CheckName, "device_total", nil, "Number of GPU devices"),
	}
}

// Configure parses the check configuration and init the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	// Check if GPU check is enabled (follows SBOM pattern)
	if !pkgconfigsetup.Datadog().GetBool("gpu.enabled") {
		return errors.New("GPU check is disabled")
	}

	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	// Get the list of disabled collectors from global configuration
	c.disabledCollectors = pkgconfigsetup.Datadog().GetStringSlice("gpu.disabled_collectors")
	for _, collectorName := range c.disabledCollectors {
		log.Infof("Collector %s is disabled by configuration", collectorName)
	}

	if c.containerProvider == nil {
		// Do not re-set the container provider if it is already set. It would be better to have it as an argument like the tagger and wmeta,
		// but because it's not componentized yet with FX, we need to do it this way (see service discovery check for a similar pattern).
		containerProvider, err := proccontainers.GetSharedContainerProvider()
		if err != nil {
			// Do not return an error here, as it would prevent the check from running in standalone mode (with `agent check run`)
			log.Errorf("failed to get shared container provider: %v", err)
		}
		c.containerProvider = containerProvider
	}

	workloadTagCacheSize := pkgconfigsetup.Datadog().GetInt("gpu.workload_tag_cache_size")
	workloadTagCache, err := NewWorkloadTagCache(c.tagger, c.wmeta, c.containerProvider, c.telemetry.component, workloadTagCacheSize)
	if err != nil {
		return fmt.Errorf("error creating workload tag cache: %w", err)
	}
	c.workloadTagCache = workloadTagCache
	c.deviceEvtGatherer = nvidia.NewDeviceEventsGatherer()

	// Compute whether we should prefer system-probe process metrics
	if pkgconfigsetup.SystemProbe().GetBool("gpu_monitoring.enabled") {
		c.spCache = nvidia.NewSystemProbeCache()
	}

	return nil
}

// ensureInitCollectors initializes the NVML library and the collectors if they are not already initialized.
// It returns an error if the initialization fails.
func (c *Check) ensureInitCollectors() error {
	// the list of devices can change over time, so grab the latest and:
	// - remove collectors for the devices that are not present anymore
	// - collect devices for which a new collector must be added
	devices, err := c.deviceCache.All()
	if err != nil {
		return fmt.Errorf("failed to retrieve physical devices: %w", err)
	}
	curDevices := map[string]ddnvml.Device{}
	for _, d := range devices {
		curDevices[d.GetDeviceInfo().UUID] = d
	}

	// discard collectors of devices that are no more available
	collectors := []nvidia.Collector{}
	collectorUUIDs := map[string]struct{}{}
	for _, c := range c.collectors {
		if _, ok := curDevices[c.DeviceUUID()]; ok {
			collectors = append(collectors, c)
			collectorUUIDs[c.DeviceUUID()] = struct{}{}
		}
	}

	// figure out which devices do not have a collector yet and build new ones accordingly
	missingDevices := []ddnvml.Device{}
	for uuid, d := range curDevices {
		if _, ok := collectorUUIDs[uuid]; !ok {
			missingDevices = append(missingDevices, d)
		}
	}
	if len(missingDevices) > 0 {
		newCollectors, err := nvidia.BuildCollectors(
			missingDevices,
			&nvidia.CollectorDependencies{
				DeviceEventsGatherer: c.deviceEvtGatherer,
				SystemProbeCache:     c.spCache,
				Telemetry:            c.telemetry.collectorTelemetry,
				Workloadmeta:         c.wmeta,
			},
			c.disabledCollectors)
		if err != nil {
			return fmt.Errorf("failed to build NVML collectors: %w", err)
		}
		collectors = append(collectors, newCollectors...)
	}

	c.collectors = collectors
	c.deviceTags = nvidia.GetDeviceTagsMapping(c.deviceCache, c.tagger)
	return nil
}

// Cancel stops the check
func (c *Check) Cancel() {
	if err := c.deviceEvtGatherer.Stop(); err != nil {
		log.Warnf("error stopping event set gatherer: %v", err)
	}

	if lib, err := ddnvml.GetSafeNvmlLib(); err == nil {
		if err := lib.Shutdown(); err != nil {
			log.Warnf("error shutting down NVML lib: %v", err)
		}
	}

	c.CheckBase.Cancel()
}

// Run executes the check. Configure must have been called before and returned no errors, otherwise
// we will panic here as we assume certain components have been initialized.
func (c *Check) Run() error {
	currentExecutionTime := time.Now()

	snd, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}
	// Commit the metrics even in case of an error
	defer snd.Commit()

	// Check the state of the NVML library for telemetry
	c.telemetry.nvmlState.Check()

	if err := c.deviceCache.Refresh(); err != nil {
		return fmt.Errorf("failed to refresh device cache: %w", err)
	}

	deviceCount, err := c.deviceCache.Count()
	if err != nil {
		if logLimitCheck.ShouldLog() {
			log.Warnf("failed to get device count: %v", err)
		}
		deviceCount = 0
	}
	c.telemetry.metrics.deviceCount.Set(float64(deviceCount))

	// Refresh SP cache before collecting metrics, if it is available
	if c.spCache != nil {
		if err := c.spCache.Refresh(); err != nil && logLimitCheck.ShouldLog() {
			if logLimitCheck.ShouldLog() {
				log.Warnf("error refreshing system-probe cache: %v", err)
			}
			// Continue with NVML-only metrics, SP collectors will return empty metrics
		}
	}

	// start device event gatherer if we have not already
	if !c.deviceEvtGatherer.Started() {
		if err := c.deviceEvtGatherer.Start(); err != nil {
			log.Warnf("error starting device events collection: %v", err)
		}
	}

	// Attempt refreshing device events
	if err := c.deviceEvtGatherer.Refresh(); err != nil && logLimitCheck.ShouldLog() {
		log.Warnf("error refreshing device events cache: %v", err)
		// Might cause empty metrics in collectors depending on device events
	}

	// Make sure workload tag resolution attempts retrieving the most up to date values.
	// Stale cache entries (from previous runs) might still be used as a fallback.
	c.workloadTagCache.MarkStale()

	// build the mapping of GPU devices -> containers to allow tagging device
	// metrics with the tags of containers that are using them
	gpuToContainersMap := c.getGPUToContainersMap()

	if err := c.emitMetrics(snd, gpuToContainersMap, currentExecutionTime); err != nil && logLimitCheck.ShouldLog() {
		log.Warnf("error while sending gpu metrics: %s", err)
	}

	return nil
}

func (c *Check) getGPUToContainersMap() map[string][]*workloadmeta.Container {
	allPhysicalDevices, err := c.deviceCache.AllPhysicalDevices()
	if err != nil {
		if logLimitCheck.ShouldLog() {
			log.Warnf("Error getting all physical devices: %s", err)
		}
		return nil
	}
	gpuToContainers := make(map[string][]*workloadmeta.Container, len(allPhysicalDevices))

	for _, container := range c.wmeta.ListContainersWithFilter(containers.HasGPUs) {
		if containers.IsDatadogAgentContainer(c.wmeta, container) {
			log.Debugf("Container %s is a Datadog container, skipping for device assignment", container.Name)
			continue
		}

		containerDevices, err := containers.MatchContainerDevices(container, allPhysicalDevices)
		if err != nil {
			c.telemetry.metrics.missingContainerGpuMapping.Inc(container.Name)
		}

		// despite an error, we still might have some devices assigned to the container
		// we also assume that each device can be assigned to only one container, and we store only the first one
		for _, device := range containerDevices {
			deviceID := device.GetDeviceInfo().UUID
			gpuToContainers[deviceID] = append(gpuToContainers[deviceID], container)
		}
	}

	return gpuToContainers
}

type deviceMetricsCollection struct {
	collectorMetrics map[nvidia.CollectorName][]nvidia.Metric // collector name -> metrics
	totalCount       int                                      // total number of metrics across all collectors
}

func (c *Check) emitMetrics(snd sender.Sender, gpuToContainersMap map[string][]*workloadmeta.Container, currentExecutionTime time.Time) error {
	err := c.ensureInitCollectors()
	if err != nil {
		return fmt.Errorf("failed to initialize NVML collectors: %w", err)
	}

	perDeviceMetrics := make(map[string]*deviceMetricsCollection)

	var multiErr error
	for _, collector := range c.collectors {
		log.Debugf("Collecting metrics from NVML collector: %s", collector.Name())
		startTime := time.Now()
		metrics, collectErr := collector.Collect()
		collectTime := time.Since(startTime)
		c.telemetry.collectorTelemetry.Time.Observe(float64(collectTime.Milliseconds()), string(collector.Name()))

		if collectErr != nil {
			c.telemetry.collectorTelemetry.CollectionErrors.Add(1, string(collector.Name()))
			multiErr = multierror.Append(multiErr, fmt.Errorf("collector %s failed. %w", collector.Name(), collectErr))
		}

		if len(metrics) > 0 {
			deviceUUID := collector.DeviceUUID()
			if perDeviceMetrics[deviceUUID] == nil {
				perDeviceMetrics[deviceUUID] = &deviceMetricsCollection{
					collectorMetrics: make(map[nvidia.CollectorName][]nvidia.Metric),
				}
			}
			perDeviceMetrics[deviceUUID].collectorMetrics[collector.Name()] = metrics
			perDeviceMetrics[deviceUUID].totalCount += len(metrics)
		}

		c.telemetry.metrics.metricsSent.Add(float64(len(metrics)), string(collector.Name()))
	}

	//iterate through devices to emit its metrics
	for deviceUUID, deviceData := range perDeviceMetrics {
		//filter out same metric with lower priority
		deduplicatedMetrics := nvidia.RemoveDuplicateMetrics(deviceData.collectorMetrics)
		c.telemetry.metrics.duplicateMetrics.Add(float64(deviceData.totalCount-len(deduplicatedMetrics)), deviceUUID)
		deviceContainers := gpuToContainersMap[deviceUUID]
		deviceTags := c.deviceTags[deviceUUID]

		// iterate through filtered metrics and emit them with the tags
		for _, metric := range deduplicatedMetrics {
			if err := c.emitSingleMetric(&metric, snd, currentExecutionTime, deviceContainers, deviceTags); err != nil {
				multiErr = multierror.Append(multiErr, fmt.Errorf("error emitting metric %s: %w", metric.Name, err))
			}
		}
	}

	return multiErr
}

func (c *Check) emitSingleMetric(metric *nvidia.Metric, snd sender.Sender, currentExecutionTime time.Time, deviceContainers []*workloadmeta.Container, deviceTags []string) error {
	var multiErr error

	metricWorkloads := metric.AssociatedWorkloads

	// Metrics with no associated workloads are assumed to apply to all workloads on the device.
	if len(metricWorkloads) == 0 {
		for _, deviceContainer := range deviceContainers {
			metricWorkloads = append(metricWorkloads, deviceContainer.EntityID)
		}
	}

	metricTags := []string{}
	for _, workloadID := range metricWorkloads {
		tags, err := c.workloadTagCache.GetOrCreateWorkloadTags(workloadID)
		if err != nil && !agenterrors.IsNotFound(err) { // Only report errors that are not "not found"
			multiErr = multierror.Append(multiErr, fmt.Errorf("error collecting workload tags for workload %s of type %s: %w", workloadID.ID, workloadID.Kind, err))
		}

		// always continue with whatever tags we can get even if there are errors
		metricTags = append(metricTags, tags...)
	}

	metricName := gpuMetricsNs + metric.Name
	allTags := append(append(deviceTags, metricTags...), metric.Tags...)

	// Use the current execution time as the timestamp for the metrics, that way we can ensure that the metrics are aligned with the check interval.
	// We need this to ensure weighted metrics are calibrated correctly.
	var err error
	metricTimestamp := float64(currentExecutionTime.UnixNano()) / float64(time.Second)
	switch metric.Type {
	case ddmetrics.CountType:
		err = snd.CountWithTimestamp(metricName, metric.Value, "", allTags, metricTimestamp)
	case ddmetrics.GaugeType:
		err = snd.GaugeWithTimestamp(metricName, metric.Value, "", allTags, metricTimestamp)
	default:
		err = fmt.Errorf("unsupported metric type %s", metric.Type)
	}

	if err != nil {
		multiErr = multierror.Append(multiErr, fmt.Errorf("error sending metric: %w", err))
	}

	return multiErr
}
