// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	"github.com/DataDog/datadog-agent/pkg/gpu/amd"
	"github.com/DataDog/datadog-agent/pkg/gpu/containers"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// amdCollectorName is the collector name used in telemetry for AMD devices.
const amdCollectorName nvidia.CollectorName = "amd"

// refreshAMDDevices rediscovers AMD GPUs and returns the ones not excluded by
// configuration. Discovery errors are logged: devices that could be probed
// are still returned.
func (c *Check) refreshAMDDevices() []*amd.Device {
	if !c.amdEnabled {
		c.amdDevices = nil
		c.amdPresent = false
		c.amdDeviceTags = nil
		return nil
	}

	devices, err := amd.Discover(c.amdSysRoot)
	if err != nil && logLimitCheck.ShouldLog() {
		log.Warnf("error discovering AMD GPUs: %v", err)
	}
	if msg := amd.KFDAccessDeniedWarning(devices); msg != "" {
		// The workloadmeta amdgpu collector warns about this, rate-limited.
		log.Debug(msg)
	}

	c.amdPresent = len(devices) > 0
	c.amdDevices = devices[:0]
	c.amdDeviceTags = make(map[string][]string, len(devices))
	for _, dev := range devices {
		if c.isDeviceExcluded(dev.UUID) {
			continue
		}
		c.amdDevices = append(c.amdDevices, dev)
		c.amdDeviceTags[dev.UUID] = c.amdDeviceTagsFor(dev)
	}
	return c.amdDevices
}

// amdOnlyHost requires readable PCI inventory, including devices whose NVIDIA
// driver is missing. NVIDIA audio functions and network adapters are not GPUs.
func (c *Check) amdOnlyHost() bool {
	pciDir := filepath.Join(c.amdSysRoot, "bus", "pci", "devices")
	entries, err := os.ReadDir(pciDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(pciDir, entry.Name(), "vendor"))
		if err != nil {
			return false
		}
		vendor, err := strconv.ParseUint(strings.TrimSpace(string(content)), 0, 16)
		if err != nil {
			return false
		}
		if vendor != 0x10de {
			continue
		}
		class, err := os.ReadFile(filepath.Join(pciDir, entry.Name(), "class"))
		if err != nil {
			return false
		}
		value, err := strconv.ParseUint(strings.TrimSpace(string(class)), 0, 24)
		if err != nil || value>>16 == 0x03 || value>>16 == 0x12 {
			return false
		}
	}
	return true
}

// amdDeviceTagsFor returns the device tags of an AMD GPU. They come from the
// tagger, which derives them from the GPU entity reported by the amdgpu
// workloadmeta collector, exactly as for NVIDIA devices. Until that entity is
// in the store (first run, or collector disabled), the tags are built from the
// discovered device.
func (c *Check) amdDeviceTagsFor(dev *amd.Device) []string {
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
func amdDeviceTags(dev *amd.Device) []string {
	tags := []string{
		"gpu_vendor:" + amd.Vendor,
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
func amdTelemetryTags(dev *amd.Device) []string {
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

// collectAMDSamples reads the telemetry of every AMD device and the GPU memory
// of the processes using them.
func (c *Check) collectAMDSamples() []collectorSamplesCollection {
	if len(c.amdDevices) == 0 {
		return nil
	}
	// Charge the shared process scan to the first device, like its errors.
	start := time.Now()
	usage, _, usageErr := amd.ReadProcessMemory(c.amdSysRoot, c.amdDevices)
	processesByDevice := make(map[string][]amd.ProcessMemory, len(c.amdDevices))
	for _, u := range usage {
		processesByDevice[u.DeviceUUID] = append(processesByDevice[u.DeviceUUID], u)
	}

	results := make([]collectorSamplesCollection, len(c.amdDevices))
	for i, dev := range c.amdDevices {
		m, err := dev.ReadMetrics()
		if i == 0 && usageErr != nil {
			// Report the process read error once, on the first device.
			err = errors.Join(err, fmt.Errorf("read AMD GPU processes: %w", usageErr))
		}
		results[i] = collectorSamplesCollection{
			name:          amdCollectorName,
			deviceUUID:    dev.UUID,
			telemetryTags: amdTelemetryTags(dev),
			samples:       append(amdSamples(m), amdProcessSamples(m, processesByDevice[dev.UUID])...),
			err:           err,
			duration:      time.Since(start),
		}
		start = time.Now()
	}
	return results
}

// amdProcessSamples returns the per-process memory metrics of a device and,
// when processes use it, a memory.limit sample associated with all of them
// (with a higher priority than the device-level one, which deduplication
// replaces), so that the limit carries the same workload tags as the usage,
// as for NVIDIA devices.
func amdProcessSamples(m amd.Metrics, processes []amd.ProcessMemory) []nvidia.Sample {
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
		limit := nvidia.NewMetric("memory.limit", m.VRAMTotalBytes.Value, metrics.GaugeType, nvidia.Medium, nil, allWorkloads)
		// Workload membership is not static: emit at the observation time,
		// rather than dropping short-lived workloads or backfilling their tags.
		limit.StrictInterval = -1
		samples = append(samples, limit)
	}
	return samples
}

// amdSamples converts AMD telemetry to GPU check metrics, using the metric
// names and units of the GPU metrics spec. Values the device does not expose
// are not emitted.
func amdSamples(m amd.Metrics) []nvidia.Sample {
	samples := make([]nvidia.Sample, 0, 17)
	add := func(name string, r amd.Reading) {
		if r.Valid {
			samples = append(samples, &nvidia.Metric{Name: name, Value: r.Value, Type: metrics.GaugeType})
		}
	}

	add("device.total", amd.Reading{Value: 1, Valid: true})
	add("gr_engine_active", m.GPUBusyPercent)
	// No passive per-CU activity counter exists without ROCm profiling, so
	// sm_active uses the GFX busy percentage, as NVIDIA's eBPF fallback does.
	// On multi-XCC Instinct GPUs this is averaged over XCCs; it overstates
	// kernels that occupy only some of the compute units.
	add("sm_active", m.GPUBusyPercent)
	add("dram_active", m.MemoryBusyPercent)

	add("memory.limit", m.VRAMTotalBytes)
	if m.VRAMTotalBytes.Valid && m.VRAMUsedBytes.Valid && m.VRAMUsedBytes.Value <= m.VRAMTotalBytes.Value {
		add("memory.free", amd.Reading{Value: m.VRAMTotalBytes.Value - m.VRAMUsedBytes.Value, Valid: true})
		if m.VRAMTotalBytes.Value > 0 {
			add("memory.utilization", amd.Reading{Value: m.VRAMUsedBytes.Value / m.VRAMTotalBytes.Value, Valid: true})
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
func pcieBandwidth(gtPerSecond, width amd.Reading) amd.Reading {
	if !gtPerSecond.Valid || !width.Valid {
		return amd.Reading{}
	}
	bps, err := gpuutil.PCIeLinkBytesPerSecondFromTransferRate(gtPerSecond.Value, int(width.Value))
	if err != nil {
		return amd.Reading{}
	}
	return amd.Reading{Value: bps, Valid: true}
}

// addAMDGPUContainers adds to gpuToContainers (allocated if nil) the
// containers that Kubernetes allocated AMD GPUs to, keyed by AMD device UUID.
// The allocations come from the kubelet PodResources API, whose device IDs for
// AMD resources are resolved with amd.MatchDevicePluginID. Several containers
// can share a physical GPU when each is allocated one of its partitions.
func (c *Check) addAMDGPUContainers(gpuToContainers map[string][]*workloadmeta.Container) map[string][]*workloadmeta.Container {
	if len(c.amdDevices) == 0 {
		return gpuToContainers
	}
	if gpuToContainers == nil {
		gpuToContainers = make(map[string][]*workloadmeta.Container)
	}

	for _, container := range c.wmeta.ListContainersWithFilter(hasAMDGPUResource) {
		if containers.IsDatadogAgentContainer(c.wmeta, container) {
			continue
		}
		matched := make(map[string]struct{})
		for _, resource := range container.ResolvedAllocatedResources {
			if !strings.HasPrefix(resource.Name, amd.ResourcePrefix) {
				continue
			}
			dev := amd.MatchDevicePluginID(c.amdSysRoot, c.amdDevices, resource.ID)
			if dev == nil {
				c.telemetry.metrics.missingContainerGpuMapping.Inc(container.Name)
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
		if strings.HasPrefix(resource.Name, amd.ResourcePrefix) {
			return true
		}
	}
	return false
}
