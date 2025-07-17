// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// systemContext holds certain attributes about the system that are used by the GPU probe.
type systemContext struct {
	// timeResolver allows to resolve kernel-time timestamps
	timeResolver *ktime.Resolver

	// procRoot is the root directory for process information
	procRoot string

	// selectedDeviceByPIDAndTID maps each process ID to the map of thread IDs to selected device index.
	// The reason to have a nested map is to allow easy cleanup of data when a process exits.
	// The thread ID is important as the device selection in CUDA is per-thread.
	// Note that this is the device index as seen by the process itself, which might
	// be modified by the CUDA_VISIBLE_DEVICES environment variable later
	selectedDeviceByPIDAndTID map[int]map[int]int32

	// cudaVisibleDevicesPerProcess maps each process ID to the latest visible
	// devices environment variable that was set by the process. This is used to
	// keep track of updates during process runtime, which aren't visible in
	// /proc/pid/environ.
	cudaVisibleDevicesPerProcess map[int]string

	// deviceCache is a cache of GPU devices on the system
	deviceCache ddnvml.DeviceCache

	// visibleDevicesCache is a cache of visible devices for each process, to avoid
	// looking into the environment variables every time
	visibleDevicesCache map[int][]ddnvml.Device

	// workloadmeta is the workloadmeta component that we use to get necessary container metadata
	workloadmeta workloadmeta.Component

	// cudaKernelCache caches kernel data and handles background loading
	cudaKernelCache *cuda.KernelCache
}

type systemContextOptions struct {
	procRoot             string
	wmeta                workloadmeta.Component
	tm                   telemetry.Component
	fatbinParsingEnabled bool
	config               *config.Config
}

type systemContextOption func(*systemContextOptions)

func withProcRoot(procRoot string) systemContextOption {
	return func(opts *systemContextOptions) {
		opts.procRoot = procRoot
	}
}

func withWorkloadMeta(wmeta workloadmeta.Component) systemContextOption {
	return func(opts *systemContextOptions) {
		opts.wmeta = wmeta
	}
}

func withTelemetry(tm telemetry.Component) systemContextOption {
	return func(opts *systemContextOptions) {
		opts.tm = tm
	}
}

func withFatbinParsingEnabled(enabled bool) systemContextOption {
	return func(opts *systemContextOptions) {
		opts.fatbinParsingEnabled = enabled
	}
}

func withConfig(config *config.Config) systemContextOption {
	return func(opts *systemContextOptions) {
		opts.config = config
	}
}

func newSystemContextOptions(optList ...systemContextOption) *systemContextOptions {
	opts := &systemContextOptions{
		fatbinParsingEnabled: false,
		config:               config.New(),
	}
	for _, opt := range optList {
		opt(opts)
	}
	return opts
}

func getSystemContext(optList ...systemContextOption) (*systemContext, error) {
	opts := newSystemContextOptions(optList...)

	ctx := &systemContext{
		procRoot:                     opts.procRoot,
		selectedDeviceByPIDAndTID:    make(map[int]map[int]int32),
		visibleDevicesCache:          make(map[int][]ddnvml.Device),
		cudaVisibleDevicesPerProcess: make(map[int]string),
		workloadmeta:                 opts.wmeta,
	}

	var err error
	ctx.deviceCache, err = ddnvml.NewDeviceCache()
	if err != nil {
		return nil, fmt.Errorf("error creating device cache: %w", err)
	}

	ctx.timeResolver, err = ktime.NewResolver()
	if err != nil {
		return nil, fmt.Errorf("error creating time resolver: %w", err)
	}

	if opts.fatbinParsingEnabled {
		ctx.cudaKernelCache, err = cuda.NewKernelCache(opts.procRoot, ctx.deviceCache.SMVersionSet(), opts.tm, opts.config.KernelCacheQueueSize)
		if err != nil {
			return nil, fmt.Errorf("error creating kernel cache: %w", err)
		}
	}

	return ctx, nil
}

// removeProcess removes any data associated with a process from the system context.
func (ctx *systemContext) removeProcess(pid int) {
	delete(ctx.selectedDeviceByPIDAndTID, pid)
	delete(ctx.visibleDevicesCache, pid)
	delete(ctx.cudaVisibleDevicesPerProcess, pid)

	if ctx.cudaKernelCache != nil {
		ctx.cudaKernelCache.CleanProcessData(pid)
	}
}

// cleanOld removes any old entries that have not been accessed in a while, to avoid
// retaining unused data forever
func (ctx *systemContext) cleanOld() {
	if ctx.cudaKernelCache != nil {
		ctx.cudaKernelCache.CleanOld()
	}
}

// filterDevicesForContainer filters the available GPU devices for the given
// container. If the ID is not empty, we check the assignment of GPU resources
// to the container and return only the devices that are available to the
// container.
func (ctx *systemContext) filterDevicesForContainer(devices []ddnvml.Device, containerID string) ([]ddnvml.Device, error) {
	if containerID == "" {
		// If the process is not running in a container, we assume all devices are available.
		return devices, nil
	}

	container, err := ctx.workloadmeta.GetContainer(containerID)
	if err != nil {
		// If we don't find the container, we assume all devices are available.
		// This can happen sometimes, e.g. if we don't have the container in the
		// store yet. Do not block metrics on that.
		if dderrors.IsNotFound(err) {
			return devices, nil
		}

		// Some error occurred while retrieving the container, this could be a
		// general error with the store, report it.
		return nil, fmt.Errorf("cannot retrieve data for container %s: %s", containerID, err)
	}

	filteredDevices, err := matchContainerDevices(container, devices)

	// Found matching devices, return them
	if len(filteredDevices) > 0 {
		if err != nil {
			if logLimitProbe.ShouldLog() {
				log.Warnf("error matching some container devices: %s. Will continue with the available devices", err)
			}
		}
		return filteredDevices, nil
	}

	// We didn't match any devices to the container, but there were not errors
	// while matching, which means that there were no GPU devices assigned to
	// the container to try and match. This could be a problem in the PodResources
	// API, or it could be due to the container environment being different
	// (e.g., Docker instead of k8s). In any case, we return the available
	// devices as a fallback.
	if err == nil {
		// An special case is when we only have one GPU in the system. In that
		// case, we don't need the API as there's only one device available, so
		// return it directly as a fallback
		if len(devices) == 1 {
			return devices, nil
		}

		// If we have more than one GPU, we need to return an error as we can't
		// determine which device to use.
		return nil, fmt.Errorf("container %s has no GPUs assigned to it, check whether we have access to the PodResources kubelet API", containerID)
	}

	// If the container has GPUs assigned to it but we couldn't match it to our
	// devices, return the error for this case and show the allocated resources
	// for debugging purposes.
	return nil, fmt.Errorf("no GPU devices found for container %s that matched its allocated resources %+v: %w", containerID, container.ResolvedAllocatedResources, err)
}

// getCurrentActiveGpuDevice returns the active GPU device for a given process and thread, based on the
// last selection (via cudaSetDevice) this thread made and the visible devices for the process.
// This function caches the visible devices for the process in the visibleDevicesCache map, so it only
// does the expensive operations of looking into the process state and filtering devices one time for each process
// containerIDFunc is a function that returns the container ID for the given process. As retrieving the container ID
// might be expensive, we pass a function that can be called to retrieve it only when needed
func (ctx *systemContext) getCurrentActiveGpuDevice(pid int, tid int, containerIDFunc func() string) (ddnvml.Device, error) {
	visibleDevices, ok := ctx.visibleDevicesCache[pid]
	if !ok {
		containerID := containerIDFunc()

		// Order is important! We need to filter the devices for the container
		// first. In a container setting, the environment variable acts as a
		// filter on the devices that are available to the process, not on the
		// devices available on the host system.
		var err error // avoid shadowing visibleDevices, declare error before so we can use = instead of :=
		visibleDevices, err = ctx.filterDevicesForContainer(ctx.deviceCache.AllPhysicalDevices(), containerID)
		if err != nil {
			return nil, fmt.Errorf("error filtering devices for container %s: %w", containerID, err)
		}

		envVar, ok := ctx.cudaVisibleDevicesPerProcess[pid]
		if !ok {
			envVar, err = kernel.GetProcessEnvVariable(pid, ctx.procRoot, cuda.CudaVisibleDevicesEnvVar)
			if err != nil {
				return nil, fmt.Errorf("error getting env var %s for process %d: %w", cuda.CudaVisibleDevicesEnvVar, pid, err)
			}
		}

		visibleDevices, err = cuda.ParseVisibleDevices(visibleDevices, envVar)
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
		return nil, fmt.Errorf("device index %d is out of range for visible devices %+v", selectedDeviceIndex, visibleDevices)
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

func (ctx *systemContext) setUpdatedVisibleDevicesEnvVar(pid int, envVar string) {
	ctx.cudaVisibleDevicesPerProcess[pid] = envVar

	// Invalidate the visible devices cache to force a re-scan of the devices
	delete(ctx.visibleDevicesCache, pid)
}
