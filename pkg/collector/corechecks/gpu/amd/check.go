// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package amd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	amdgpu "github.com/DataDog/datadog-agent/pkg/gpu/amd"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/containers"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// logLimitCheck limits repeated warnings, as in the NVIDIA GPU check.
var logLimitCheck = log.NewLogLimit(20, 10*time.Minute)

// amdCollectorName is the collector name used in telemetry for AMD devices.
const amdCollectorName nvidia.CollectorName = "amd"

// defaultAMDReadTimeout bounds how long a check run waits for the telemetry of
// one AMD device. Reading all of its attributes takes about a millisecond on
// real hardware, but a sysfs read blocked in the driver (a hung GPU or SMU)
// cannot be cancelled.
const defaultAMDReadTimeout = 5 * time.Second

// Check collects the metrics of AMD GPUs driven by the amdgpu kernel
// driver, from sysfs. It is separate from the NVIDIA GPU check, and shares its
// sample emission pipeline and metric names.
type Check struct {
	core.CheckBase
	tagger              tagger.Component                 // tagger adds device and workload tags to metrics
	wmeta               workloadmeta.Component           // wmeta lists the containers allocated AMD GPUs
	telemetryComponent  telemetry.Component              // telemetryComponent creates the telemetry of the workload tag cache
	telemetry           *checkTelemetry                  // telemetry holds the internal telemetry of the check
	containerProvider   proccontainers.ContainerProvider // containerProvider maps PIDs to containers when workloadmeta does not have the process
	workloadTagCache    *gpu.WorkloadTagCache            // workloadTagCache caches workload tags
	rateCalculator      *nvidia.RateCalculator           // rateCalculator calculates the rate of metrics
	strictIntervals     *nvidia.StrictIntervalProcessor  // strictIntervals timestamps metrics that must be emitted on a fixed cadence
	excludedDeviceUUIDs map[string]struct{}              // excludedDeviceUUIDs contains normalized device UUIDs whose metrics are not collected
	sysRoot             string                           // sysRoot is the sysfs root used to discover AMD GPUs
	devices             []*amdgpu.Device                 // devices are the AMD GPUs found on the last run, minus excluded ones
	deviceTags          map[string][]string              // deviceTags maps device UUIDs to their device tags
	readTimeout         time.Duration                    // readTimeout bounds the wait for the telemetry of one device
	pendingReads        map[string]<-chan struct{}       // pendingReads holds telemetry reads, by device UUID, that have not returned yet
}

type checkTelemetry struct {
	emitter                    gpu.EmitterTelemetry
	missingContainerGpuMapping telemetry.Counter
	deviceCount                telemetry.Gauge
}

// amdCollectorTelemetryTagNames are the tag names of the collector
// telemetry, in the order of the values returned by amdTelemetryTags.
var amdCollectorTelemetryTagNames = []string{
	"collector",
	"gpu_device",
	"gpu_virtualization_mode",
	"gpu_architecture",
	"gpu_slicing_mode",
	"gpu_nvlink_capable",
	"gpu_nvlink_version",
	"gpu_driver_version",
}

func newCheckTelemetry(tm telemetry.Component) *checkTelemetry {
	return &checkTelemetry{
		emitter: gpu.EmitterTelemetry{
			CollectionRuns:   tm.NewCounter(CheckName, "collection_runs", amdCollectorTelemetryTagNames, "Number of AMD GPU telemetry collections"),
			CollectionErrors: tm.NewCounter(CheckName, "collection_errors", amdCollectorTelemetryTagNames, "Number of errors collecting AMD GPU telemetry"),
			CollectionTime:   tm.NewHistogram(CheckName, "collection_time_ms", amdCollectorTelemetryTagNames, "Time taken to collect AMD GPU telemetry, in milliseconds", []float64{10, 100, 500, 1000, 5000}),
			MetricsSent:      tm.NewCounter(CheckName, "metrics_sent", []string{"collector"}, "Number of AMD GPU metrics sent"),
			DuplicateMetrics: tm.NewCounter(CheckName, "duplicate_metrics", []string{"device"}, "Number of duplicate AMD GPU metrics removed by priority de-duplication"),
		},
		missingContainerGpuMapping: tm.NewCounter(CheckName, "missing_container_gpu_mapping", []string{"container_name"}, "Number of containers with no matching AMD GPU device"),
		deviceCount:                tm.NewGauge(CheckName, "device_total", nil, "Number of AMD GPU devices"),
	}
}

// Factory creates a new AMD GPU check factory
func Factory(tagger tagger.Component, telemetry telemetry.Component, wmeta workloadmeta.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger, telemetry, wmeta)
	})
}

func newCheck(tagger tagger.Component, tm telemetry.Component, wmeta workloadmeta.Component) check.Check {
	return &Check{
		CheckBase:           core.NewCheckBase(CheckName),
		tagger:              tagger,
		wmeta:               wmeta,
		telemetryComponent:  tm,
		telemetry:           newCheckTelemetry(tm),
		rateCalculator:      nvidia.NewRateCalculator(),
		strictIntervals:     nvidia.NewStrictIntervalProcessor(0),
		excludedDeviceUUIDs: make(map[string]struct{}),
		deviceTags:          make(map[string][]string),
	}
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if !pkgconfigsetup.Datadog().GetBool("gpu.enabled") || !pkgconfigsetup.Datadog().GetBool("gpu.amd.enabled") {
		return fmt.Errorf("%w: AMD GPU check requires gpu.enabled and gpu.amd.enabled", check.ErrSkipCheckInstance)
	}

	if err := c.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}

	c.excludedDeviceUUIDs = make(map[string]struct{})
	for _, deviceUUID := range pkgconfigsetup.Datadog().GetStringSlice("gpu.excluded_devices") {
		c.excludedDeviceUUIDs[strings.ToLower(deviceUUID)] = struct{}{}
	}
	c.strictIntervals = nvidia.NewStrictIntervalProcessor(gpuconfig.New().StaticMetricsReportingInterval)
	if c.sysRoot == "" {
		// Tests set the root before Configure; otherwise honor HOST_SYS and /host/sys in containers.
		c.sysRoot = kernel.SysFSRoot()
	}
	if c.readTimeout == 0 {
		c.readTimeout = defaultAMDReadTimeout
	}

	if c.containerProvider == nil {
		// As in the NVIDIA GPU check: not an error, to keep `agent check` working.
		containerProvider, err := proccontainers.GetSharedContainerProvider()
		if err != nil {
			log.Errorf("failed to get shared container provider: %v", err)
		}
		c.containerProvider = containerProvider
	}

	workloadTagCache, err := gpu.NewWorkloadTagCacheWithSubsystem(CheckName, c.tagger, c.wmeta, c.containerProvider, c.telemetryComponent, pkgconfigsetup.Datadog().GetInt("gpu.workload_tag_cache_size"))
	if err != nil {
		return fmt.Errorf("error creating workload tag cache: %w", err)
	}
	c.workloadTagCache = workloadTagCache
	return nil
}

// Interval returns the scheduling interval of the check, which follows
// gpu.collection_interval_override like the NVIDIA GPU check.
func (c *Check) Interval() time.Duration {
	if iv := pkgconfigsetup.Datadog().GetInt("gpu.collection_interval_override"); iv > 0 {
		return time.Duration(iv) * time.Second
	}
	return c.CheckBase.Interval()
}

// Run executes the check
func (c *Check) Run() error {
	snd, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}
	// Commit the metrics even in case of an error
	defer snd.Commit()

	devices := c.refreshDevices()
	c.telemetry.deviceCount.Set(float64(len(devices)))

	// Make sure workload tag resolution attempts retrieving the most up to date values.
	// Stale cache entries (from previous runs) might still be used as a fallback.
	c.workloadTagCache.MarkStale()

	if err := c.emitMetrics(snd, time.Now()); err != nil && logLimitCheck.ShouldLog() {
		log.Warnf("error while sending AMD GPU metrics: %s", err)
	}
	return nil
}

// emitMetrics collects and sends the metrics of the devices found by the last
// refreshDevices.
func (c *Check) emitMetrics(snd sender.Sender, currentExecutionTime time.Time) error {
	emitter := gpu.SampleEmitter{
		WorkloadTagCache: c.workloadTagCache,
		RateCalculator:   c.rateCalculator,
		StrictIntervals:  c.strictIntervals,
		Telemetry:        c.telemetry.emitter,
	}
	deviceTags := func(deviceUUID string) []string { return c.deviceTags[deviceUUID] }
	return emitter.Emit(snd, c.collectSamples(), c.gpuToContainers(), deviceTags, currentExecutionTime)
}

func (c *Check) isDeviceExcluded(deviceUUID string) bool {
	_, excluded := c.excludedDeviceUUIDs[strings.ToLower(deviceUUID)]
	return excluded
}

// refreshDevices rediscovers AMD GPUs and returns the ones not excluded by
// configuration. Discovery errors are logged: devices that could be probed
// are still returned.
func (c *Check) refreshDevices() []*amdgpu.Device {
	devices, err := amdgpu.Discover(c.sysRoot)
	if err != nil && logLimitCheck.ShouldLog() {
		log.Warnf("error discovering AMD GPUs: %v", err)
	}
	if msg := amdgpu.KFDAccessDeniedWarning(devices); msg != "" {
		// The workloadmeta amdgpu collector warns about this, rate-limited.
		log.Debug(msg)
	}

	c.devices = devices[:0]
	c.deviceTags = make(map[string][]string, len(devices))
	for _, dev := range devices {
		if c.isDeviceExcluded(dev.UUID) {
			continue
		}
		c.devices = append(c.devices, dev)
		c.deviceTags[dev.UUID] = c.deviceTagsFor(dev)
	}
	return c.devices
}

// deviceTagsFor returns the device tags of an AMD GPU. They come from the
// tagger, which derives them from the GPU entity reported by the amdgpu
// workloadmeta collector, exactly as for NVIDIA devices. Until that entity is
// in the store (first run, or collector disabled), the tags are built from the
// discovered device.
func (c *Check) deviceTagsFor(dev *amdgpu.Device) []string {
	if c.tagger != nil {
		tags, err := c.tagger.Tag(taggertypes.NewEntityID(taggertypes.GPU, dev.UUID), taggertypes.ChecksConfigCardinality)
		if err != nil && logLimitCheck.ShouldLog() {
			log.Warnf("error getting tags for AMD GPU %s: %v", dev.UUID, err)
		}
		if len(tags) > 0 {
			return tags
		}
	}
	return amdDeviceTags(dev)
}

// amdDeviceTags builds the device tags of an AMD GPU, with the same keys and
// values the tagger produces from its workloadmeta entity (ExtractGPUTags).
func amdDeviceTags(dev *amdgpu.Device) []string {
	tags := []string{
		"gpu_vendor:" + amdgpu.Vendor,
		"gpu_uuid:" + strings.ToLower(dev.UUID),
		"gpu_device:" + gpuutil.NormalizeGPUDeviceName(dev.Name),
		"gpu_pci_bus_id:" + strings.ToLower(dev.PCIBusID),
		"gpu_slicing_mode:none",
		"gpu_mig_profile:none",
		"gpu_nvlink_capable:false",
		"gpu_parent_uuid:" + strings.ToLower(dev.UUID),
	}
	if dev.DriverVersion != "" {
		tags = append(tags, "gpu_driver_version:"+dev.DriverVersion)
	}
	if dev.Architecture != "" {
		tags = append(tags, "gpu_architecture:"+strings.ToLower(dev.Architecture))
	}
	if gpuType := gpuutil.ExtractGPUType(dev.Name); gpuType != "" {
		tags = append(tags, "gpu_type:"+strings.ToLower(gpuType))
	}
	return tags
}

// amdTelemetryTags returns the collector telemetry tag values of an AMD GPU,
// in the order of the NVIDIA collector telemetry tag names.
func amdTelemetryTags(dev *amdgpu.Device) []string {
	return []string{
		string(amdCollectorName),
		gpuutil.NormalizeGPUDeviceName(dev.Name),
		"unknown",                 // gpu_virtualization_mode
		dev.Architecture,          // gpu_architecture
		"none",                    // gpu_slicing_mode
		strconv.FormatBool(false), // gpu_nvlink_capable
		"",                        // gpu_nvlink_version
		dev.DriverVersion,         // gpu_driver_version
	}
}

// collectSamples reads the telemetry of every AMD device and the GPU memory
// of the processes using them.
func (c *Check) collectSamples() []gpu.CollectorSamples {
	if len(c.devices) == 0 {
		return nil
	}
	// Charge the shared process scan to the first device, like its errors.
	start := time.Now()
	usage, _, usageErr := amdgpu.ReadProcessMemory(c.sysRoot, c.devices)
	processScan := time.Since(start)
	processesByDevice := make(map[string][]amdgpu.ProcessMemory, len(c.devices))
	for _, u := range usage {
		processesByDevice[u.DeviceUUID] = append(processesByDevice[u.DeviceUUID], u)
	}

	readings := c.readMetrics()
	results := make([]gpu.CollectorSamples, len(c.devices))
	for i, dev := range c.devices {
		reading := readings[i]
		if !reading.metrics.VRAMTotalBytes.Valid && dev.MemoryTotal > 0 {
			// The VRAM size is static: when the read did not return it (a
			// blocked device), keep the limit that process memory is compared
			// with, from discovery.
			reading.metrics.VRAMTotalBytes = amdgpu.Reading{Value: float64(dev.MemoryTotal), Valid: true}
		}
		if i == 0 {
			reading.duration += processScan
			if usageErr != nil {
				// Report the process read error once, on the first device.
				reading.err = errors.Join(reading.err, fmt.Errorf("read AMD GPU processes: %w", usageErr))
			}
		}
		results[i] = gpu.CollectorSamples{
			Name:          amdCollectorName,
			DeviceUUID:    dev.UUID,
			TelemetryTags: amdTelemetryTags(dev),
			Samples:       append(amdSamples(reading.metrics), amdProcessSamples(reading.metrics, processesByDevice[dev.UUID])...),
			Err:           reading.err,
			Duration:      reading.duration,
		}
	}
	return results
}

// amdReading is the outcome of reading the telemetry of one AMD device.
type amdReading struct {
	metrics  amdgpu.Metrics
	err      error
	duration time.Duration
}

// readMetrics reads the telemetry of every AMD device concurrently and
// waits at most readTimeout. A sysfs read blocked in the driver (a hung GPU
// or SMU) cannot be cancelled, so a device whose read has not returned is
// reported with an error and skipped by later runs until that read returns:
// one stuck device neither holds the check runner nor piles up blocked
// goroutines. Its process memory, read from the KFD, is still reported.
func (c *Check) readMetrics() []amdReading {
	if c.pendingReads == nil {
		c.pendingReads = make(map[string]<-chan struct{})
	}
	type indexedReading struct {
		index   int
		reading amdReading
	}
	readings := make([]amdReading, len(c.devices))
	waiting := make(map[int]bool, len(c.devices))
	results := make(chan indexedReading, len(c.devices)) // buffered: late reads never block
	for i, dev := range c.devices {
		if pending, ok := c.pendingReads[dev.UUID]; ok {
			select {
			case <-pending:
				delete(c.pendingReads, dev.UUID)
			default:
				readings[i].err = fmt.Errorf("telemetry read of AMD GPU %s is still blocked from a previous run", dev.UUID)
				continue
			}
		}
		done := make(chan struct{})
		c.pendingReads[dev.UUID] = done
		waiting[i] = true
		go func() {
			defer close(done)
			start := time.Now()
			m, err := dev.ReadMetrics()
			results <- indexedReading{index: i, reading: amdReading{metrics: m, err: err, duration: time.Since(start)}}
		}()
	}

	record := func(r indexedReading) {
		readings[r.index] = r.reading
		delete(waiting, r.index)
		delete(c.pendingReads, c.devices[r.index].UUID)
	}
	timeout := time.NewTimer(c.readTimeout)
	defer timeout.Stop()
	for len(waiting) > 0 {
		select {
		case r := <-results:
			record(r)
		case <-timeout.C:
			// Keep the reads that returned as the deadline passed, rather than
			// reporting them as blocked.
			for drained := false; !drained; {
				select {
				case r := <-results:
					record(r)
				default:
					drained = true
				}
			}
			for i := range waiting {
				readings[i] = amdReading{
					err:      fmt.Errorf("telemetry read of AMD GPU %s did not return within %s", c.devices[i].UUID, c.readTimeout),
					duration: c.readTimeout,
				}
			}
			return readings
		}
	}
	return readings
}

// amdProcessSamples returns the per-process memory metrics of a device and,
// when processes use it, a memory.limit sample associated with all of them
// (with a higher priority than the device-level one, which deduplication
// replaces), so that the limit carries the same workload tags as the usage,
// as for NVIDIA devices.
func amdProcessSamples(m amdgpu.Metrics, processes []amdgpu.ProcessMemory) []nvidia.Sample {
	if len(processes) == 0 {
		return nil
	}
	samples := make([]nvidia.Sample, 0, len(processes)+1)
	allWorkloads := make([]workloadmeta.EntityID, 0, len(processes))
	for _, p := range processes {
		workload := workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: strconv.Itoa(p.PID)}
		allWorkloads = append(allWorkloads, workload)
		samples = append(samples, nvidia.NewMetric("process.memory.usage", float64(p.VRAMBytes), metrics.GaugeType, nvidia.Medium, nil, []workloadmeta.EntityID{workload}))
	}
	if m.VRAMTotalBytes.Valid {
		samples = append(samples, nvidia.NewMetric("memory.limit", m.VRAMTotalBytes.Value, metrics.GaugeType, nvidia.Medium, nil, allWorkloads))
	}
	return samples
}

// amdSamples converts AMD telemetry to GPU check metrics, using the metric
// names and units of the GPU metrics spec. Values the device does not expose
// are not emitted.
func amdSamples(m amdgpu.Metrics) []nvidia.Sample {
	samples := make([]nvidia.Sample, 0, 17)
	add := func(name string, r amdgpu.Reading) {
		if r.Valid {
			samples = append(samples, &nvidia.Metric{Name: name, Value: r.Value, Type: metrics.GaugeType})
		}
	}

	add("device.total", amdgpu.Reading{Value: 1, Valid: true})
	add("gr_engine_active", m.GPUBusyPercent)
	// No passive per-CU activity counter exists without ROCm profiling, so
	// sm_active uses the GFX busy percentage, as NVIDIA's eBPF fallback does.
	// On multi-XCC Instinct GPUs this is averaged over XCCs; it overstates
	// kernels that occupy only some of the compute units.
	add("sm_active", m.GPUBusyPercent)
	add("dram_active", m.MemoryBusyPercent)

	add("memory.limit", m.VRAMTotalBytes)
	if m.VRAMTotalBytes.Valid && m.VRAMUsedBytes.Valid && m.VRAMUsedBytes.Value <= m.VRAMTotalBytes.Value {
		add("memory.free", amdgpu.Reading{Value: m.VRAMTotalBytes.Value - m.VRAMUsedBytes.Value, Valid: true})
		if m.VRAMTotalBytes.Value > 0 {
			add("memory.utilization", amdgpu.Reading{Value: m.VRAMUsedBytes.Value / m.VRAMTotalBytes.Value, Valid: true})
		}
	}

	// gpu.temperature is the die temperature: the edge sensor when present,
	// otherwise the junction (hotspot) sensor, which some ASICs report alone.
	if m.EdgeTemperatureC.Valid {
		add("temperature", m.EdgeTemperatureC)
	} else {
		add("temperature", m.JunctionTemperatureC)
	}
	add("memory.temperature", m.MemoryTemperatureC)

	add("power.usage", m.PowerMilliwatts)
	add("power.management_limit", m.PowerCapMilliwatts)

	add("clock.speed.graphics", m.GraphicsClockMHz)
	add("clock.speed.memory", m.MemoryClockMHz)

	add("pci.link.width.current", m.PCIeLinkWidth)
	add("pci.link.width.max", m.PCIeMaxLinkWidth)
	add("pci.link.speed.current", pcieBandwidth(m.PCIeLinkSpeedGTs, m.PCIeLinkWidth))
	add("pci.link.speed.max", pcieBandwidth(m.PCIeMaxLinkSpeedGTs, m.PCIeMaxLinkWidth))

	return samples
}

// pcieBandwidth converts a per-lane transfer rate and a lane count to usable
// bytes per second, using the same PCIe generation table as NVIDIA devices.
func pcieBandwidth(gtPerSecond, width amdgpu.Reading) amdgpu.Reading {
	if !gtPerSecond.Valid || !width.Valid {
		return amdgpu.Reading{}
	}
	bps, err := gpuutil.PCIeLinkBytesPerSecondFromTransferRate(gtPerSecond.Value, int(width.Value))
	if err != nil {
		return amdgpu.Reading{}
	}
	return amdgpu.Reading{Value: bps, Valid: true}
}

// gpuToContainers returns the containers that Kubernetes allocated AMD GPUs
// to, keyed by AMD device UUID.
// The allocations come from the kubelet PodResources API, whose device IDs for
// AMD resources are resolved with amdgpu.MatchDevicePluginID. Several containers
// can share a physical GPU when each is allocated one of its partitions.
func (c *Check) gpuToContainers() map[string][]*workloadmeta.Container {
	if len(c.devices) == 0 {
		return nil
	}
	gpuToContainers := make(map[string][]*workloadmeta.Container)

	for _, container := range c.wmeta.ListContainersWithFilter(hasAMDGPUResource) {
		if containers.IsDatadogAgentContainer(c.wmeta, container) {
			continue
		}
		matched := make(map[string]struct{})
		for _, resource := range container.ResolvedAllocatedResources {
			if !strings.HasPrefix(resource.Name, amdgpu.ResourcePrefix) {
				continue
			}
			dev := amdgpu.MatchDevicePluginID(c.sysRoot, c.devices, resource.ID)
			if dev == nil {
				c.telemetry.missingContainerGpuMapping.Inc(container.Name)
				continue
			}
			if _, dup := matched[dev.UUID]; dup {
				continue // two partitions of the same GPU
			}
			matched[dev.UUID] = struct{}{}
			gpuToContainers[dev.UUID] = append(gpuToContainers[dev.UUID], container)
		}
	}
	return gpuToContainers
}

// hasAMDGPUResource reports whether Kubernetes allocated an AMD device plugin
// resource to the container.
func hasAMDGPUResource(container *workloadmeta.Container) bool {
	for _, resource := range container.ResolvedAllocatedResources {
		if strings.HasPrefix(resource.Name, amdgpu.ResourcePrefix) {
			return true
		}
	}
	return false
}
