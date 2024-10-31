// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"debug/elf"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"

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

	// fileData maps each executable file path to its Fatbin file data
	fileData map[string]*fileData

	// pidMaps maps each process ID to its memory maps
	pidMaps map[int]*kernel.ProcMapEntries

	// procRoot is the root directory for process information
	procRoot string
}

// fileData holds the symbol table and Fatbin data for a given file.
type fileData struct {
	symbolTable  map[uint64]string
	fatbin       *cuda.Fatbin
	lastAccessed time.Time // keep the last time this file was accessed to cleanup old entries
}

func (fd *fileData) updateAccessTime() {
	fd.lastAccessed = time.Now()
}

func getSystemContext(nvmlLib nvml.Interface, procRoot string) (*systemContext, error) {
	ctx := &systemContext{
		maxGpuThreadsPerDevice: make(map[int]int),
		deviceSmVersions:       make(map[int]int),
		fileData:               make(map[string]*fileData),
		pidMaps:                make(map[int]*kernel.ProcMapEntries),
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

func (ctx *systemContext) getFileData(path string) (*fileData, error) {
	if fd, ok := ctx.fileData[path]; ok {
		fd.updateAccessTime()
		return fd, nil
	}

	elfFile, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening ELF file %s: %w", path, err)
	}
	defer elfFile.Close()

	fatbin, err := cuda.ParseFatbinFromELFFile(elfFile)
	if err != nil {
		return nil, fmt.Errorf("error parsing fatbin on %s: %w", path, err)
	}

	fd := &fileData{
		symbolTable: make(map[uint64]string),
		fatbin:      fatbin,
	}

	syms, err := elfFile.Symbols()
	if err != nil {
		return nil, fmt.Errorf("error reading symbols from ELF file %s: %w", path, err)
	}

	for _, sym := range syms {
		fd.symbolTable[sym.Value] = sym.Name
	}

	fd.updateAccessTime()
	ctx.fileData[path] = fd
	return ctx.fileData[path], nil
}

func (ctx *systemContext) getProcessMemoryMaps(pid int) (*kernel.ProcMapEntries, error) {
	if maps, ok := ctx.pidMaps[pid]; ok {
		return maps, nil
	}

	maps, err := kernel.ReadProcessMemMaps(pid, ctx.procRoot)
	if err != nil {
		return nil, fmt.Errorf("error reading process memory maps: %w", err)
	}

	ctx.pidMaps[pid] = &maps
	return &maps, nil
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

	for path, fd := range ctx.fileData {
		if fd.lastAccessed.Before(fatbinExpirationTime) {
			delete(ctx.fileData, path)
		}
	}
}
