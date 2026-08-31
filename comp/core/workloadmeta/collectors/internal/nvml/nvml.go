// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.uber.org/fx"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "nvml"
	componentName = "workloadmeta-nvml"
	nvidiaVendor  = "nvidia"
)

var logLimiter = log.NewLogLimit(20, 10*time.Minute)

type collector struct {
	id                                 string
	catalog                            workloadmeta.AgentType
	store                              workloadmeta.Component
	seenUUIDs                          map[string]struct{}
	seenPIDsToGPUs                     map[int][]string    // PID -> GPU UUIDs
	seenContainerGPUs                  map[string]struct{} // container IDs published with a GPU mapping
	reportedDriverNotLoaded            bool
	integrateWithWorkloadmetaProcesses bool
	gpuMonitoringEnabled               bool
	lastCollectionTimestamp            time.Time
}

func (c *collector) getGPUDeviceInfo(device ddnvml.Device) (*workloadmeta.GPU, error) {
	// build the GPU device info using the pre-computed values
	// from the device cache
	devInfo := device.GetDeviceInfo()
	nvlinkVersion := devInfo.NVLinkVersion
	if devInfo.NVLinkLinkCount == 0 {
		nvlinkVersion = "not_nvlink_capable"
	} else if nvlinkVersion == "" {
		nvlinkVersion = "unknown"
	}
	gpuDeviceInfo := workloadmeta.GPU{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindGPU,
			ID:   devInfo.UUID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: devInfo.Name,
		},
		Vendor:  nvidiaVendor,
		Device:  devInfo.Name,
		GPUType: gpuutil.ExtractGPUType(devInfo.Name),
		Index:   devInfo.Index,
		ComputeCapability: workloadmeta.GPUComputeCapability{
			Major: int(devInfo.SMVersion / 10),
			Minor: int(devInfo.SMVersion % 10),
		},
		TotalCores:    devInfo.CoreCount,
		TotalMemory:   devInfo.Memory,
		Architecture:  gpuutil.ArchToString(devInfo.Architecture),
		NVLinkVersion: nvlinkVersion,
	}

	switch d := device.(type) {
	case *ddnvml.PhysicalDevice:
		gpuDeviceInfo.DeviceType = workloadmeta.GPUDeviceTypePhysical
		for _, child := range d.MIGChildren {
			gpuDeviceInfo.ChildrenGPUUUIDs = append(gpuDeviceInfo.ChildrenGPUUUIDs, child.GetDeviceInfo().UUID)
		}
	case *ddnvml.MIGDevice:
		gpuDeviceInfo.DeviceType = workloadmeta.GPUDeviceTypeMIG
		if d.Parent != nil {
			gpuDeviceInfo.ParentGPUUUID = d.Parent.UUID
		}
	default:
		gpuDeviceInfo.DeviceType = workloadmeta.GPUDeviceTypeUnknown
	}

	c.fillNVMLAttributes(&gpuDeviceInfo, device)
	c.fillProcesses(&gpuDeviceInfo, device)

	return &gpuDeviceInfo, nil
}

// fillNVMLAttributes fills the attributes of the GPU device by querying NVML API
func (c *collector) fillNVMLAttributes(gpuDeviceInfo *workloadmeta.GPU, device ddnvml.Device) {
	migDevice, isMig := device.(*ddnvml.MIGDevice)
	physicalDevice := device
	if isMig {
		physicalDevice = migDevice.Parent
	}

	virtMode, err := physicalDevice.GetVirtualizationMode()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("cannot get virtualization mode: %v for %d", err, gpuDeviceInfo.Index)
		}
	} else {
		gpuDeviceInfo.VirtualizationMode = gpuutil.VirtualizationModeToString(virtMode)
	}

	memBusWidth, err := device.GetMemoryBusWidth()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
		}
	} else {
		gpuDeviceInfo.MemoryBusWidth = memBusWidth
	}

	pciInfo, err := physicalDevice.GetPciInfo()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
		}
	} else {
		gpuDeviceInfo.PCIBusID = pciBusIDFromNVMLInfo(pciInfo)
	}

	fabricInfo, err := physicalDevice.GetGpuFabricInfo()
	if err == nil {
		if clusterUUID, cliqueID, ok := fabricInfoToTags(fabricInfo); ok {
			gpuDeviceInfo.FabricClusterUUID = clusterUUID
			gpuDeviceInfo.FabricCliqueID = cliqueID
		}
	}

	// Do not generate errors for vGPU devices, we already know that they don't support max clock info
	if virtMode != nvml.GPU_VIRTUALIZATION_MODE_VGPU {
		maxSMClock, err := physicalDevice.GetMaxClockInfo(nvml.CLOCK_SM)
		if err != nil {
			if logLimiter.ShouldLog() {
				log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
			}
		} else {
			gpuDeviceInfo.MaxClockRates[workloadmeta.GPUSM] = maxSMClock
		}

		maxMemoryClock, err := physicalDevice.GetMaxClockInfo(nvml.CLOCK_MEM)
		if err != nil {
			if logLimiter.ShouldLog() {
				log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
			}
		} else {
			gpuDeviceInfo.MaxClockRates[workloadmeta.GPUMemory] = maxMemoryClock
		}
	} else {
		if _, ok := c.seenUUIDs[gpuDeviceInfo.EntityID.ID]; !ok && logLimiter.ShouldLog() {
			// only report the warning once for each device
			log.Infof("vGPU device %s does not support queries for max clock info", gpuDeviceInfo.EntityID.ID)
		}
	}
}

func pciBusIDFromNVMLInfo(pciInfo nvml.PciInfo) string {
	// NVML exposes domain, bus, and device as numeric fields, but not the PCI
	// function. For NVIDIA GPUs, the GPU function is the .0 function; companion
	// functions, when present, represent auxiliary devices such as audio.
	return strings.ToLower(fmt.Sprintf("%04x:%02x:%02x.0", pciInfo.Domain, pciInfo.Bus, pciInfo.Device))
}

func fabricClusterUUIDFromNVMLInfo(clusterUUID [16]uint8) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", clusterUUID[0:4], clusterUUID[4:6], clusterUUID[6:8], clusterUUID[8:10], clusterUUID[10:16])
}

func fabricInfoToTags(fabricInfo nvml.GpuFabricInfo_v2) (string, uint32, bool) {
	if fabricInfo.State != nvml.GPU_FABRIC_STATE_COMPLETED ||
		nvml.Return(fabricInfo.Status) != nvml.SUCCESS ||
		fabricInfo.ClusterUuid == [16]uint8{} {
		return "", 0, false
	}

	return fabricClusterUUIDFromNVMLInfo(fabricInfo.ClusterUuid), fabricInfo.CliqueId, true
}

func (c *collector) fillProcesses(gpuDeviceInfo *workloadmeta.GPU, device ddnvml.Device) {
	seenPIDs := make(map[int]struct{})
	procs, err := device.GetComputeRunningProcesses()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v for %d", err, gpuDeviceInfo.Index)
		}
	}

	for _, proc := range procs {
		seenPIDs[int(proc.Pid)] = struct{}{}
	}

	// GetProcessUtilization can show more processes than GetComputeRunningProcesses, but it might not be supported by all devices.
	utilizationProcs, err := device.GetProcessUtilization(uint64(c.lastCollectionTimestamp.UnixMicro()))
	if err != nil {
		var nvmlErr *ddnvml.NvmlAPIError
		if errors.As(err, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_NOT_FOUND) {
			utilizationProcs = nil // error not found occurs normally when no process is using the GPU, clear the array to avoid processing any data
		} else {
			// only logs
			if logLimiter.ShouldLog() {
				log.Debugf("%v for %d", err, gpuDeviceInfo.Index)
			}
		}
	}

	for _, proc := range utilizationProcs {
		seenPIDs[int(proc.Pid)] = struct{}{}
	}

	gpuDeviceInfo.ActivePIDs = make([]int, 0, len(seenPIDs))
	for pid := range seenPIDs {
		gpuDeviceInfo.ActivePIDs = append(gpuDeviceInfo.ActivePIDs, pid)
	}
	slices.Sort(gpuDeviceInfo.ActivePIDs) // Sort to ensure the gpu device info doesn't change due to PID ordering changes
}

// newCollector creates a new collector with the default values, useful for testing.
func newCollector(store workloadmeta.Component, config config.Component) *collector {
	collector := &collector{
		id:                      collectorID,
		catalog:                 workloadmeta.NodeAgent,
		seenUUIDs:               map[string]struct{}{},
		seenPIDsToGPUs:          make(map[int][]string),
		seenContainerGPUs:       make(map[string]struct{}),
		store:                   store,
		lastCollectionTimestamp: time.Now(),
		gpuMonitoringEnabled:    true,
	}

	if config != nil {
		collector.integrateWithWorkloadmetaProcesses = config.GetBool("gpu.integrate_with_workloadmeta_processes")
		collector.gpuMonitoringEnabled = config.GetBool("gpu.enabled")
	}

	return collector
}

// NewCollector returns a kubelet CollectorProvider that instantiates its collector
func NewCollector(config config.Component) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: newCollector(nil, config),
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

// Start initializes the NVML library and sets the store
func (c *collector) Start(_ context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.NVML) {
		return dderrors.NewDisabled(componentName, "Agent does not have NVML library available")
	}

	if !c.gpuMonitoringEnabled {
		return dderrors.NewDisabled(componentName, "GPU monitoring is disabled")
	}

	c.store = store

	return nil
}

// Pull collects the GPUs available on the node and notifies the store
func (c *collector) Pull(ctx context.Context) error {
	lib, err := ddnvml.GetSafeNvmlLib()
	if err != nil {
		// Do not consider an unloaded driver as an error more than once.
		// Some installations will have the NVIDIA libraries but not the driver. Report the error
		// only once to avoid log spam, treat it the same as if there was no library available or
		// there were no GPUs.
		if ddnvml.IsDriverNotLoaded(err) && !c.reportedDriverNotLoaded {
			c.reportedDriverNotLoaded = true
			return nil
		}

		return fmt.Errorf("failed to get NVML library : %w", err)
	}

	deviceCache := ddnvml.NewDeviceCache(ddnvml.WithDeviceCacheLib(lib))
	if err := deviceCache.Refresh(); err != nil {
		return fmt.Errorf("failed to initialize device cache: %w", err)
	}

	// driver version is equal to all devices of the same vendor
	// currently we handle only nvidia.
	// in the future this function should be refactored to support more vendors
	driverVersion, err := lib.SystemGetDriverVersion()
	// we try to get the driver version as best effort, just log warning if it fails
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("%v", err)
		}
	}

	// attempt getting list of unhealthy devices (if available)
	unhealthyDevices, err := c.getUnhealthyDevices(ctx)
	if err != nil && logLimiter.ShouldLog() {
		log.Warnf("failed getting unhealthy devices: %v", err)
	}

	// note: the device list can change over time so we need to set/unset for reconciliation
	allDevices, err := deviceCache.All()
	if err != nil {
		// Should not happen as we check the last init error for the library
		return fmt.Errorf("failed to get all devices: %w", err)
	}

	// add/update current devices
	currentUUIDs := map[string]struct{}{}
	pidToGPUs := make(map[int][]string) // PID -> GPU UUIDs
	timestamp := time.Now()
	var events []workloadmeta.CollectorEvent
	for _, dev := range allDevices {
		gpu, err := c.getGPUDeviceInfo(dev)
		if err != nil {
			return err
		}

		gpu.DriverVersion = driverVersion

		_, unhealthy := unhealthyDevices[gpu.ID]
		gpu.Healthy = !unhealthy

		uuid := dev.GetDeviceInfo().UUID
		currentUUIDs[uuid] = struct{}{}
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeSet,
			Entity: gpu,
		})

		if c.integrateWithWorkloadmetaProcesses {
			for _, pid := range gpu.ActivePIDs {
				pidToGPUs[pid] = append(pidToGPUs[pid], uuid)
			}
		}
	}

	// remove previous devices that are no more available
	for uuid := range c.seenUUIDs {
		if _, ok := currentUUIDs[uuid]; ok {
			continue
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.GPU{
				EntityID: workloadmeta.EntityID{
					ID:   uuid,
					Kind: workloadmeta.KindGPU,
				},
			},
		})
	}

	c.seenUUIDs = currentUUIDs

	if c.integrateWithWorkloadmetaProcesses {
		events = append(events, c.createProcessEvents(pidToGPUs)...)
	}

	events = append(events, c.createContainerGPUEvents(deviceCache)...)

	c.store.Notify(events)
	c.lastCollectionTimestamp = timestamp

	return nil
}

func (c *collector) createProcessEvents(pidToGPUs map[int][]string) []workloadmeta.CollectorEvent {
	events := make([]workloadmeta.CollectorEvent, 0, len(pidToGPUs))

	// Create events for active processes
	for pid, uuids := range pidToGPUs {
		var gpuEntityIDs []workloadmeta.EntityID
		for _, uuid := range uuids {
			gpuEntityIDs = append(gpuEntityIDs, workloadmeta.EntityID{
				Kind: workloadmeta.KindGPU,
				ID:   uuid,
			})
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindProcess,
					ID:   strconv.Itoa(int(pid)),
				},
				Pid:  int32(pid),
				GPUs: gpuEntityIDs,
			},
		})
	}

	// Remove inactive processes. Because we use SourceNVML for the Process entities, workloadmeta
	// will not remove the process if it has been added by another source.
	for pid := range c.seenPIDsToGPUs {
		if _, stillActive := pidToGPUs[pid]; stillActive {
			continue
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Process{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindProcess,
					ID:   strconv.Itoa(int(pid)),
				},
			},
		})
	}

	c.seenPIDsToGPUs = pidToGPUs

	return events
}

// cdiSpecDirs and migMinorsPath are variables so tests can point them at
// fixtures; the node-local chain reads real files and is otherwise untestable.
//
// CDI specs are written by the DRA driver into the host's /var/run/cdi, which
// is a real directory on disk rather than a kernel filesystem -- so a
// containerized Agent only sees it through a mount. The standard deployment
// mounts the host's /var/run at /host/var/run (Dockerfiles/manifests/agent.yaml),
// so that prefix is tried first and works with no extra wiring; the bare path
// covers a host-installed Agent and deployments that bind /var/run/cdi
// directly. migMinorsPath needs no prefix: /proc/driver is a procfs entry
// owned by the nvidia driver and is visible in any procfs instance, including
// the container's own.
var (
	cdiSpecDirs   = []string{"/host/var/run/cdi", "/var/run/cdi"}
	migMinorsPath = "/proc/driver/nvidia-caps/mig-minors"
)

// createContainerGPUEvents publishes the Container<->GPU edge for DRA-allocated
// devices, resolving each container's CDI device nodes to NVML UUIDs. This is
// the node-local route (RFC §4.4a): it repairs the container->device edge that
// the regex guess in pkg/gpu/containers cannot make on a DRA node, where the
// allocated device name is pool-scoped ("gpu-0-mig-1g18gb-19-0") rather than an
// NVML index or UUID.
func (c *collector) createContainerGPUEvents(deviceCache ddnvml.DeviceCache) []workloadmeta.CollectorEvent {
	var events []workloadmeta.CollectorEvent
	// Containers that still claim a DRA device, whether or not this pull could
	// resolve it. Retraction keys off this rather than off resolution success:
	// a transient read failure would otherwise unset the mapping and set it
	// again on the next pull, flapping the pod tags on the device's metrics.
	claiming := make(map[string]struct{})

	for _, container := range c.store.ListContainers() {
		// One event per container, not per device: workloadmeta replaces the
		// per-source entity on each Set, so emitting one event per device would
		// leave only the last device's UUID on a container holding several.
		var uuids []string
		var hasCDIDevices bool
		for _, res := range container.ResolvedAllocatedResources {
			// Only DRA resources carry CDI device names.
			for _, cdiName := range res.CdiDevices {
				hasCDIDevices = true
				for _, uuid := range c.resolveCDIToGPUs(deviceCache, cdiName) {
					if !slices.Contains(uuids, uuid) {
						uuids = append(uuids, uuid)
					}
				}
			}
		}
		if hasCDIDevices {
			claiming[container.ID] = struct{}{}
		}
		if len(uuids) == 0 {
			continue
		}
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeSet,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   container.ID,
				},
				GPUDeviceIDs: uuids,
			},
		})
	}

	// Retract the mapping for containers that are gone, or that no longer hold
	// a DRA device. Because the entity is published under SourceNVML,
	// workloadmeta keeps it alive after every other source has unset it, so
	// without this the entity survives the container: it never leaves the
	// store, the tagger never expires its tags, and
	// ListContainersWithFilter(HasGPUs) keeps returning it -- so when the
	// device is reallocated its metrics carry the dead pod's tags too.
	for id := range c.seenContainerGPUs {
		if _, stillClaiming := claiming[id]; stillClaiming {
			continue
		}
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceNVML,
			Type:   workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   id,
				},
			},
		})
	}
	c.seenContainerGPUs = claiming

	return events
}

// resolveCDIToGPUs resolves a fully-qualified CDI device name (e.g.
// "k8s.gpu.nvidia.com/claim=<uid>-gpu-0-mig-...") to the UUIDs of the NVML
// devices it pins, via the node-local chain: CDI spec device nodes -> either a
// physical device (/dev/nvidiaN, whole-card claim) or mig-minors -> (gpu, gi,
// ci) -> NVML MIG device.
func (c *collector) resolveCDIToGPUs(deviceCache ddnvml.DeviceCache, cdiName string) []string {
	// The CDI name is "<vendor>/claim=<uid>-<device>"; the spec file is keyed
	// by the UID and the entry inside it by the whole "<uid>-<device>" string.
	deviceKey := cdiDeviceKey(cdiName)
	claimUID := cdiClaimUID(deviceKey)
	if claimUID == "" {
		if logLimiter.ShouldLog() {
			log.Debugf("DRA: cannot extract claim UID from CDI device name %q", cdiName)
		}
		return nil
	}

	nodes, err := cdiDeviceNodes(claimUID, deviceKey)
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Debugf("DRA: cannot read CDI spec for claim %s: %s", claimUID, err)
		}
		return nil
	}
	if len(nodes) == 0 {
		if logLimiter.ShouldLog() {
			log.Debugf("DRA: no nvidia device nodes in CDI spec for claim %s", claimUID)
		}
		return nil
	}

	// A MIG entry pins the parent card's device node as well as the capability
	// devices, because the container needs access to the card its instance
	// lives on. That parent is access, not an allocation: resolving it as a
	// whole card would tie the container to every other workload on the card,
	// and would do so silently -- with a plausible UUID rather than an error --
	// whenever the MIG hop below fails.
	migEntry := false
	for _, node := range nodes {
		if isMIGCapabilityDevice(node.path) {
			migEntry = true
			break
		}
	}

	var uuids []string
	var capMinors []int
	for _, node := range nodes {
		// A whole-card claim pins /dev/nvidiaN, which is the NVML index
		// directly -- no capability device and no mig-minors hop involved.
		if minor, ok := physicalDeviceMinor(node.path); ok {
			if migEntry {
				continue // the parent of this entry's MIG instance
			}
			if uuid, ok := resolvePhysicalUUID(deviceCache, minor); ok {
				uuids = append(uuids, uuid)
			} else if logLimiter.ShouldLog() {
				log.Debugf("DRA: NVML has no physical device with minor number %d (claim %s)", minor, claimUID)
			}
			continue
		}
		if !isMIGCapabilityDevice(node.path) {
			continue
		}
		if node.minor < 0 {
			// The spec pins a capability device but the scan did not find its
			// minor. Logged rather than skipped silently: without the minor
			// this container cannot be attributed at all, and the cause is a
			// spec layout the scanner does not understand.
			log.Warnf("DRA: no minor for capability device %s in CDI spec for claim %s; the MIG container will not be attributed", node.path, claimUID)
			continue
		}
		capMinors = append(capMinors, node.minor)
	}

	// MIG: map the capability minors to (gpu, gi, ci) tuples, then to UUIDs.
	for _, inst := range migMinorsToInstances(capMinors) {
		uuid, ok := resolveMIGUUID(deviceCache, inst.gpu, inst.gi, inst.ci)
		if !ok {
			if logLimiter.ShouldLog() {
				log.Debugf("DRA: NVML has no MIG device for gpu%d/gi%d/ci%d (claim %s)", inst.gpu, inst.gi, inst.ci, claimUID)
			}
			continue
		}
		uuids = append(uuids, uuid)
	}

	if len(uuids) == 0 {
		return nil
	}
	log.Debugf("DRA: resolved %s -> %v", cdiName, uuids)
	return uuids
}

// isMIGCapabilityDevice reports whether a device-node path is a MIG capability
// device. The check is anchored on the directory rather than on the substring
// "nvidia-cap": /dev/nvidia-caps-imex-channels/channelN also contains it, and
// IMEX channel minors are numbered in a different space, so treating one as a
// capability minor can resolve to a MIG instance the container does not own.
func isMIGCapabilityDevice(path string) bool {
	return strings.HasPrefix(path, "/dev/nvidia-caps/nvidia-cap")
}

// cdiDeviceKey returns the part of a CDI device name after "claim=", which is
// both the "<uid>-<device>" key the spec file uses for its device entries and
// the string cdiClaimUID reads the UID out of.
func cdiDeviceKey(cdiName string) string {
	const marker = "claim="
	i := strings.Index(cdiName, marker)
	if i < 0 {
		return ""
	}
	return cdiName[i+len(marker):]
}

// cdiClaimUID extracts the claim UID from a CDI device key of the form
// "<uid>-gpu-0-mig-...".
//
// The UID is a Kubernetes UUID and therefore contains hyphens
// ("c8593c85-440d-4156-b199-aea592ff83df"), so it cannot be split off at the
// first hyphen -- doing so yields only the first 8 characters and the spec file
// lookup then misses. A UUID is always 36 characters, and the driver appends
// "-<device-name>" after it.
func cdiClaimUID(deviceKey string) string {
	const uuidLen = 36
	if len(deviceKey) < uuidLen {
		return ""
	}
	uid := deviceKey[:uuidLen]
	// The UID is interpolated into a file path below. It comes from kubelet, so
	// this is a sanity check rather than a trust boundary, but a path separator
	// here would escape the spec directory.
	if strings.ContainsAny(uid, `/\`) {
		return ""
	}
	return uid
}

// cdiDeviceNode is one device node pinned by a CDI spec.
type cdiDeviceNode struct {
	path  string
	minor int
}

// cdiSpec is the part of a CDI specification this code reads. CDI is a
// standardised format, so the field names are a contract rather than an
// observation; everything not needed here is left out deliberately.
type cdiSpec struct {
	Devices []struct {
		Name           string `yaml:"name"`
		ContainerEdits struct {
			DeviceNodes []struct {
				Path  string `yaml:"path"`
				Minor *int   `yaml:"minor"`
			} `yaml:"deviceNodes"`
		} `yaml:"containerEdits"`
	} `yaml:"devices"`
}

// cdiDeviceNodes reads the CDI spec for a claim and returns the nvidia device
// nodes pinned by one device entry. Spec path:
// <cdiSpecDir>/k8s.gpu.nvidia.com-claim_<uid>.yaml.
//
// A claim holding several devices lists them all in one file, so the entry is
// selected by name: reading every node in the file would attribute all of the
// claim's devices to any container holding one of them.
//
// The spec is unmarshalled, with a line scan kept as a fallback. The scan is
// what was verified against real hardware, so it stays reachable if a driver
// ever emits a document this struct does not fit; but it can only guess at
// which entry a node belongs to, and it depends on the order and spacing of
// keys, so it is not the primary path.
func cdiDeviceNodes(claimUID, deviceKey string) ([]cdiDeviceNode, error) {
	name := fmt.Sprintf("k8s.gpu.nvidia.com-claim_%s.yaml", claimUID)
	var data []byte
	var err error
	for _, dir := range cdiSpecDirs {
		data, err = os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	if nodes, ok := parseCDISpec(data, deviceKey); ok {
		return nodes, nil
	}
	if logLimiter.ShouldLog() {
		log.Debugf("DRA: could not read device %q out of the CDI spec for claim %s; falling back to scanning the file", deviceKey, claimUID)
	}
	return scanCDISpec(data, deviceKey), nil
}

// parseCDISpec returns the nvidia device nodes of one device entry. It reports
// false when the document does not parse, does not contain the named entry, or
// contains it with no nvidia device node -- in every one of those cases the
// caller is better served by the scan than by an empty result, because an empty
// result silently drops the container's attribution.
func parseCDISpec(data []byte, deviceKey string) ([]cdiDeviceNode, bool) {
	var spec cdiSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, false
	}

	for _, device := range spec.Devices {
		if device.Name != deviceKey {
			continue
		}
		var nodes []cdiDeviceNode
		for _, dn := range device.ContainerEdits.DeviceNodes {
			if !strings.Contains(dn.Path, "/dev/nvidia") {
				continue
			}
			node := cdiDeviceNode{path: dn.Path, minor: -1}
			if dn.Minor != nil {
				node.minor = *dn.Minor
			}
			nodes = append(nodes, node)
		}
		return nodes, len(nodes) > 0
	}
	return nil, false
}

// scanCDISpec is the fallback described on cdiDeviceNodes: it collects device
// nodes by reading lines, attributing each to the most recent sequence entry
// named "name".
//
// When no entry matches deviceKey it returns every node in the file only if
// the file describes a single device, where "all of them" and "this one" are
// the same set. On a multi-device claim it returns nothing instead: handing
// back every device would tag this container with GPUs belonging to its
// siblings, and a wrong pod tag is worse than a missing one -- it is invisible
// downstream, where a missing one shows up as untagged.
func scanCDISpec(data []byte, deviceKey string) []cdiDeviceNode {
	lines := strings.Split(string(data), "\n")
	byDevice := map[string][]cdiDeviceNode{}
	var all []cdiDeviceNode
	current := ""

	for i, line := range lines {
		key, value, isItem, ok := yamlKeyValue(line)
		if !ok {
			continue
		}
		// Device entries are sequence items ("- name: <uid>-gpu-0"); requiring
		// the item marker keeps a "name" key nested elsewhere in the document
		// from re-scoping the nodes that follow it.
		if isItem && key == "name" {
			current = value
			continue
		}
		if key != "path" || !strings.Contains(value, "/dev/nvidia") {
			continue
		}

		node := cdiDeviceNode{path: value, minor: -1}
		// The minor belongs to this entry, so scan until the next entry starts
		// rather than for a fixed number of lines: a device node carries an
		// optional "type" field, and a fixed window silently loses the minor
		// whenever the driver emits one more key than the window allows.
		for j := i + 1; j < len(lines); j++ {
			k, v, nextIsItem, ok := yamlKeyValue(lines[j])
			if !ok {
				continue
			}
			if nextIsItem || k == "path" {
				break // next entry; this one has no minor
			}
			if k == "minor" {
				if n, err := strconv.Atoi(v); err == nil {
					node.minor = n
				}
				break
			}
		}

		all = append(all, node)
		if current != "" {
			byDevice[current] = append(byDevice[current], node)
		}
	}

	if nodes, found := byDevice[deviceKey]; found {
		return nodes
	}
	if len(byDevice) <= 1 {
		return all
	}
	log.Warnf("DRA: CDI spec describes %d devices but none matched %q; not attributing, because returning all of them would tag this container with another container's GPUs", len(byDevice), deviceKey)
	return nil
}

// yamlKeyValue splits one line of a CDI spec into its key and value, stripping
// the list-item marker that prefixes the first key of a sequence entry
// ("- path: /dev/nvidia0") and reporting whether it was present. Matching the
// key exactly, rather than looking for "path:" anywhere in the line, is what
// keeps "hostPath:" -- which every device node also carries -- from being read
// as a second device.
func yamlKeyValue(line string) (key, value string, isItem, ok bool) {
	trimmed := strings.TrimSpace(line)
	if after, found := strings.CutPrefix(trimmed, "- "); found {
		isItem = true
		trimmed = strings.TrimSpace(after)
	}
	key, value, found := strings.Cut(trimmed, ":")
	if !found {
		return "", "", false, false
	}
	return key, strings.Trim(strings.TrimSpace(value), `"'`), isItem, true
}

// physicalDeviceMinor returns the minor number for a /dev/nvidiaN device node
// path -- N is the minor, not NVML's enumeration index. Capability devices
// (/dev/nvidia-caps/nvidia-capN) and control nodes (/dev/nvidiactl,
// /dev/nvidia-uvm) are not physical devices.
func physicalDeviceMinor(path string) (int, bool) {
	const prefix = "/dev/nvidia"
	if !strings.HasPrefix(path, prefix) {
		return 0, false
	}
	index, err := strconv.Atoi(path[len(prefix):])
	if err != nil {
		return 0, false
	}
	return index, true
}

// migInstance identifies one MIG compute instance.
type migInstance struct {
	gpu, gi, ci int
}

// migMinorsToInstances maps capability device minors to MIG instances using
// mig-minors, whose lines look like "gpu0/gi11/ci0/access 103".
//
// A MIG instance is pinned by two capability devices -- the GPU instance's
// ("gpu0/gi11/access") and the compute instance's ("gpu0/gi11/ci0/access") --
// and only the latter carries a full tuple, so lines without a "ci" component
// are skipped. Returning every instance rather than one keeps a claim holding
// several MIG devices from collapsing onto whichever the scan happened to see
// last.
func migMinorsToInstances(minors []int) []migInstance {
	if len(minors) == 0 {
		return nil
	}
	data, err := os.ReadFile(migMinorsPath)
	if err != nil {
		return nil
	}

	var instances []migInstance
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		minor, err := strconv.Atoi(fields[1])
		if err != nil || !slices.Contains(minors, minor) {
			continue
		}

		// Only the compute-instance entry carries the full tuple; the GPU
		// instance's own entry ("gpu0/gi11/access") has no ci and is skipped.
		gpu, gi, ci := -1, -1, -1
		for _, part := range strings.Split(fields[0], "/") {
			switch {
			case strings.HasPrefix(part, "gpu"):
				if v, err := strconv.Atoi(strings.TrimPrefix(part, "gpu")); err == nil {
					gpu = v
				}
			case strings.HasPrefix(part, "gi"):
				if v, err := strconv.Atoi(strings.TrimPrefix(part, "gi")); err == nil {
					gi = v
				}
			case strings.HasPrefix(part, "ci"):
				if v, err := strconv.Atoi(strings.TrimPrefix(part, "ci")); err == nil {
					ci = v
				}
			}
		}
		if gpu < 0 || gi < 0 || ci < 0 {
			continue
		}
		inst := migInstance{gpu: gpu, gi: gi, ci: ci}
		if !slices.Contains(instances, inst) {
			instances = append(instances, inst)
		}
	}
	return instances
}

// resolvePhysicalUUID returns the UUID of the physical device owning a
// /dev/nvidiaN minor number.
//
// Matching is on MinorNumber, which is what the device-node path encodes.
// Index is a separate NVML concept: the two agree on ordinary configurations
// but nothing guarantees it, and matching on the wrong one resolves to another
// card silently, with a plausible UUID. A device whose driver did not expose a
// minor (-1, the API is non-critical) is skipped rather than compared against
// its index, so an unavailable API yields no attribution instead of a
// confidently wrong one.
func resolvePhysicalUUID(deviceCache ddnvml.DeviceCache, minor int) (string, bool) {
	all, err := deviceCache.All()
	if err != nil {
		return "", false
	}
	for _, dev := range all {
		physical, ok := dev.(*ddnvml.PhysicalDevice)
		if ok && physical.MinorNumber >= 0 && physical.MinorNumber == minor {
			return physical.GetDeviceInfo().UUID, true
		}
	}
	return "", false
}

// resolveMIGUUID resolves (gpu, gi, ci) to a MIG device UUID via NVML.
func resolveMIGUUID(deviceCache ddnvml.DeviceCache, gpu, gi, ci int) (string, bool) {
	all, err := deviceCache.All()
	if err != nil {
		return "", false
	}
	for _, dev := range all {
		physical, ok := dev.(*ddnvml.PhysicalDevice)
		if !ok || physical.Index != gpu {
			continue
		}
		// A GPU instance can hold several compute instances (e.g. 3g.71gb split
		// into 3x 1c.3g); matching on the GPU instance alone would attribute
		// every one of those containers to whichever CI NVML enumerates first.
		var giMatches []*ddnvml.MIGDevice
		for _, mig := range physical.MIGChildren {
			if mig.MIGInstanceID == gi {
				giMatches = append(giMatches, mig)
			}
		}
		for _, mig := range giMatches {
			if mig.ComputeInstanceID == ci {
				return mig.GetDeviceInfo().UUID, true
			}
		}
		// ComputeInstanceID is -1 when the non-critical API was unavailable, so
		// no CI comparison is possible. Falling back to the GI is correct only
		// while it identifies one device -- the 1-CI-per-GI case, which is every
		// 1g/2g profile. With several, the GI is ambiguous and any pick is a
		// guess, so return nothing rather than tag containers with each other's
		// instances.
		if len(giMatches) == 1 && giMatches[0].ComputeInstanceID < 0 {
			return giMatches[0].GetDeviceInfo().UUID, true
		}
		if len(giMatches) > 1 && logLimiter.ShouldLog() {
			log.Debugf("DRA: gpu%d/gi%d holds %d compute instances and NVML did not expose their IDs; not attributing ci%d", gpu, gi, len(giMatches), ci)
		}
	}
	return "", false
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}
