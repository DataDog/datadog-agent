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

	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/util/ktime"
)

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

func getSystemContext(nvmlLib nvml.Interface, procRoot string) (*systemContext, error) {
	ctx := &systemContext{
		maxGpuThreadsPerDevice: make(map[int]int),
		deviceSmVersions:       make(map[int]int),
		cudaSymbols:            make(map[string]*symbolsEntry),
		pidMaps:                make(map[int][]*procfs.ProcMap),
		nvmlLib:                nvmlLib,
		procRoot:               procRoot,
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
