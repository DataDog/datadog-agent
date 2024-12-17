// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"fmt"
	"slices"
	"time"

	"github.com/prometheus/procfs"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const nvidiaResourceName = "nvidia.com/gpu"

// systemContext holds certain attributes about the system that are used by the GPU probe.
type systemContext struct {
	// maxGpuThreadsPerDevice maps each device index to the maximum number of threads it can run in parallel
	maxGpuThreadsPerDevice map[int]int

	// timeResolver allows to resolve kernel-time timestamps
	timeResolver *ktime.Resolver

	// nvmlLib is the NVML library used to query GPU devices
	nvmlLib nvml.Interface

	// deviceSmVersions maps each device index to its SM (Compute architecture) version
	deviceSmVersions map[int]int

	// cudaSymbols maps each executable file path to its Fatbin file data
	cudaSymbols map[string]*symbolsEntry

	// pidMaps maps each process ID to its memory maps
	pidMaps map[int][]*procfs.ProcMap

	// procRoot is the root directory for process information
	procRoot string

	// procfsObj is the procfs filesystem object to retrieve process maps
	procfsObj procfs.FS

	// selectedDeviceByPIDAndTID maps each process ID to the map of thread IDs to selected device index.
	// The reason to have a nested map is to allow easy cleanup of data when a process exits.
	// The thread ID is important as the device selection in CUDA is per-thread.
	// Note that this is the device index as seen by the process itself, which might
	// be modified by the CUDA_VISIBLE_DEVICES environment variable later
	selectedDeviceByPIDAndTID map[int]map[int]int32

	// gpuDevices is the list of GPU devices on the system. Needs to be present to
	// be able to compute the visible devices for a process
	gpuDevices []nvml.Device

	// visibleDevicesCache is a cache of visible devices for each process, to avoid
	// looking into the environment variables every time
	visibleDevicesCache map[int][]nvml.Device

	// workloadmeta is the workloadmeta component that we use to get necessary container metadata
	workloadmeta workloadmeta.Component
}

// symbolsEntry embeds cuda.Symbols adding a field for keeping track of the last
// time the entry was accessed, for cleanup purposes.
type symbolsEntry struct {
	*cuda.Symbols
	lastUsedTime time.Time
}

func (e *symbolsEntry) updateLastUsedTime() {
	e.lastUsedTime = time.Now()
}

func getSystemContext(nvmlLib nvml.Interface, procRoot string, wmeta workloadmeta.Component) (*systemContext, error) {
	ctx := &systemContext{
		maxGpuThreadsPerDevice:    make(map[int]int),
		deviceSmVersions:          make(map[int]int),
		cudaSymbols:               make(map[string]*symbolsEntry),
		pidMaps:                   make(map[int][]*procfs.ProcMap),
		nvmlLib:                   nvmlLib,
		procRoot:                  procRoot,
		selectedDeviceByPIDAndTID: make(map[int]map[int]int32),
		visibleDevicesCache:       make(map[int][]nvml.Device),
		workloadmeta:              wmeta,
	}

	if err := ctx.fillDeviceInfo(); err != nil {
		return nil, fmt.Errorf("error querying devices: %w", err)
	}

	var err error
	ctx.timeResolver, err = ktime.NewResolver()
	if err != nil {
		return nil, fmt.Errorf("error creating time resolver: %w", err)
	}

	ctx.procfsObj, err = procfs.NewFS(procRoot)
	if err != nil {
		return nil, fmt.Errorf("error creating procfs filesystem: %w", err)
	}

	return ctx, nil
}

func getDeviceSmVersion(device nvml.Device) (int, error) {
	major, minor, ret := device.GetCudaComputeCapability()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("error getting SM version: %s", nvml.ErrorString(ret))
	}

	return major*10 + minor, nil
}

func (ctx *systemContext) fillDeviceInfo() error {
	count, ret := ctx.nvmlLib.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get device count: %s", nvml.ErrorString(ret))
	}
	for i := 0; i < count; i++ {
		dev, ret := ctx.nvmlLib.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device handle for index %d: %s", i, nvml.ErrorString(ret))
		}
		smVersion, err := getDeviceSmVersion(dev)
		if err != nil {
			return err
		}
		ctx.deviceSmVersions[i] = smVersion

		maxThreads, ret := dev.GetNumGpuCores()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting max threads for device %s: %s", dev, nvml.ErrorString(ret))
		}

		ctx.maxGpuThreadsPerDevice[i] = maxThreads

		ctx.gpuDevices = append(ctx.gpuDevices, dev)
	}
	return nil
}

func (ctx *systemContext) getCudaSymbols(path string) (*symbolsEntry, error) {
	if data, ok := ctx.cudaSymbols[path]; ok {
		data.updateLastUsedTime()
		return data, nil
	}

	data, err := cuda.GetSymbols(path)
	if err != nil {
		return nil, fmt.Errorf("error getting file data: %w", err)
	}

	wrapper := &symbolsEntry{Symbols: data}
	wrapper.updateLastUsedTime()
	ctx.cudaSymbols[path] = wrapper

	return wrapper, nil
}

func (ctx *systemContext) getProcessMemoryMaps(pid int) ([]*procfs.ProcMap, error) {
	if maps, ok := ctx.pidMaps[pid]; ok {
		return maps, nil
	}

	proc, err := ctx.procfsObj.Proc(pid)
	if err != nil {
		return nil, fmt.Errorf("error opening process %d: %w", pid, err)
	}

	maps, err := proc.ProcMaps()
	if err != nil {
		return nil, fmt.Errorf("error reading process %d memory maps: %w", pid, err)
	}

	// Remove any maps that don't have a pathname, we only want to keep the ones that are backed by a file
	// to read from there the CUDA symbols.
	maps = slices.DeleteFunc(maps, func(m *procfs.ProcMap) bool {
		return m.Pathname == ""
	})
	slices.SortStableFunc(maps, func(a, b *procfs.ProcMap) int {
		return int(a.StartAddr) - int(b.StartAddr)
	})
	if err != nil {
		return nil, fmt.Errorf("error reading process memory maps: %w", err)
	}

	ctx.pidMaps[pid] = maps
	return maps, nil
}

// removeProcess removes any data associated with a process from the system context.
func (ctx *systemContext) removeProcess(pid int) {
	delete(ctx.pidMaps, pid)
	delete(ctx.selectedDeviceByPIDAndTID, pid)
	delete(ctx.visibleDevicesCache, pid)
}

// cleanupOldEntries removes any old entries that have not been accessed in a while, to avoid
// retaining unused data forever
func (ctx *systemContext) cleanupOldEntries() {
	maxFatbinAge := 5 * time.Minute
	fatbinExpirationTime := time.Now().Add(-maxFatbinAge)

	for path, data := range ctx.cudaSymbols {
		if data.lastUsedTime.Before(fatbinExpirationTime) {
			delete(ctx.cudaSymbols, path)
		}
	}
}

// filterDevicesForContainer filters the available GPU devices for the given
// container. If the ID is not empty, we check the assignment of GPU resources
// to the container and return only the devices that are available to the
// container.
func (ctx *systemContext) filterDevicesForContainer(devices []nvml.Device, containerID string) ([]nvml.Device, error) {
	if containerID == "" {
		// If the process is not running in a container, we assume all devices are available.
		return devices, nil
	}

	container, err := ctx.workloadmeta.GetContainer(containerID)
	if err != nil {
		// If we don't find the container, we assume all devices are available.
		// This can happen sometimes, e.g. if we don't have the container in the
		// store yet. Do not block metrics on that.
		if errors.IsNotFound(err) {
			return devices, nil
		}

		// Some error occurred while retrieving the container, this could be a
		// general error with the store, report it.
		return nil, fmt.Errorf("cannot retrieve data for container %s: %s", containerID, err)
	}

	var filteredDevices []nvml.Device
	for _, resource := range container.AllocatedResources {
		// Only consider NVIDIA GPUs
		if resource.Name != nvidiaResourceName {
			continue
		}

		for _, device := range devices {
			uuid, ret := device.GetUUID()
			if ret != nvml.SUCCESS {
				log.Warnf("Error getting GPU UUID for device %s: %s", device, nvml.ErrorString(ret))
				continue
			}

			if resource.ID == uuid {
				filteredDevices = append(filteredDevices, device)
				break
			}
		}
	}

	// We didn't find any devices assigned to the container, report it as an error.
	if len(filteredDevices) == 0 {
		return nil, fmt.Errorf("no GPU devices found for container %s that matched its allocated resources %+v", containerID, container.AllocatedResources)
	}

	return filteredDevices, nil
}

// getCurrentActiveGpuDevice returns the active GPU device for a given process and thread, based on the
// last selection (via cudaSetDevice) this thread made and the visible devices for the process.
func (ctx *systemContext) getCurrentActiveGpuDevice(pid int, tid int, containerID string) (nvml.Device, error) {
	visibleDevices, ok := ctx.visibleDevicesCache[pid]
	if !ok {
		// Order is important! We need to filter the devices for the container
		// first. In a container setting, the environment variable acts as a
		// filter on the devices that are available to the process, not on the
		// devices available on the host system.
		var err error // avoid shadowing visibleDevices, declare error before so we can use = instead of :=
		visibleDevices, err = ctx.filterDevicesForContainer(ctx.gpuDevices, containerID)
		if err != nil {
			return nil, fmt.Errorf("error filtering devices for container %s: %w", containerID, err)
		}

		visibleDevices, err = cuda.GetVisibleDevicesForProcess(visibleDevices, pid, ctx.procRoot)
		if err != nil {
			return nil, fmt.Errorf("error getting visible devices for process %d: %w", pid, err)
		}

		ctx.visibleDevicesCache[pid] = visibleDevices
	}

	if len(visibleDevices) == 0 {
		return nil, fmt.Errorf("no GPU devices for process %d", pid)
	}

	selectedDeviceIndex := int32(0)
	pidMap, ok := ctx.selectedDeviceByPIDAndTID[pid]
	if ok {
		selectedDeviceIndex = pidMap[tid] // Defaults to 0, which is the same as CUDA
	}

	if selectedDeviceIndex < 0 || selectedDeviceIndex >= int32(len(visibleDevices)) {
		return nil, fmt.Errorf("device index %d is out of range", selectedDeviceIndex)
	}

	return visibleDevices[selectedDeviceIndex], nil
}

// setDeviceSelection sets the selected device index for a given process and thread.
func (ctx *systemContext) setDeviceSelection(pid int, tid int, deviceIndex int32) {
	if _, ok := ctx.selectedDeviceByPIDAndTID[pid]; !ok {
		ctx.selectedDeviceByPIDAndTID[pid] = make(map[int]int32)
	}

	ctx.selectedDeviceByPIDAndTID[pid][tid] = deviceIndex
}

// getDeviceByUUID returns the device with the given UUID.
func (ctx *systemContext) getDeviceByUUID(uuid string) (nvml.Device, error) {
	for _, dev := range ctx.gpuDevices {
		devUUID, ret := dev.GetUUID()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("error getting device UUID: %s", nvml.ErrorString(ret))
		}
		if devUUID == uuid {
			return dev, nil
		}
	}
	return nil, fmt.Errorf("device with UUID %s not found", uuid)
}
