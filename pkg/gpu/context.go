// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gpu

import (
	"debug/elf"
	"fmt"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	sectime "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// systemContext holds certain attributes about the system that are used by the GPU probe.
type systemContext struct {
	// deviceSmVersions maps each device index to its SM (Compute architecture) version
	deviceSmVersions map[int]int

	// fileData maps each file path to its Fatbin file data
	fileData map[string]*fileData

	// pidMaps maps each process ID to its memory maps
	pidMaps map[int]*kernel.ProcMapEntries

	// timeResolver is used to convert from kernel time to system time
	timeResolver *sectime.Resolver

	// processCache is used to resolve process information
	processCache *tracer.ProcessCache
}

// fileData holds the symbol table and Fatbin data for a given file.
type fileData struct {
	symbolTable  map[uint64]string
	fatbin       *cuda.Fatbin
	lastAccessed time.Time
}

func (fd *fileData) updateAccessTime() {
	fd.lastAccessed = time.Now()
}

type systemContextOpts string

const (
	// systemContextOptDisableGpuQuery disables querying GPU devices, useful for tests where no GPU is available
	systemContextOptDisableGpuQuery systemContextOpts = "disableGpuQuery"
)

func getSystemContext(opts ...systemContextOpts) (*systemContext, error) {
	var err error

	ctx := &systemContext{
		fileData: make(map[string]*fileData),
		pidMaps:  make(map[int]*kernel.ProcMapEntries),
	}

	if !slices.Contains(opts, systemContextOptDisableGpuQuery) {
		if err = ctx.queryDevices(); err != nil {
			return nil, fmt.Errorf("error querying devices: %w", err)
		}
	}

	ctx.timeResolver, err = sectime.NewResolver()
	if err != nil {
		return nil, fmt.Errorf("cannot create time resolver: %s", err)
	}

	ctx.processCache, err = tracer.NewProcessCache(32768)
	if err != nil {
		return nil, fmt.Errorf("cannot create process cache: %s", err)
	}
	events.RegisterHandler(ctx.processCache)

	return ctx, nil
}

func (ctx *systemContext) queryDevices() error {
	devices, err := cuda.GetGPUDevices()
	if err != nil {
		return fmt.Errorf("error getting GPU devices: %w", err)
	}

	ctx.deviceSmVersions = make(map[int]int)
	for i, device := range devices {
		major, minor, ret := device.GetCudaComputeCapability()
		if err = cuda.WrapNvmlError(ret); err != nil {
			return fmt.Errorf("error getting SM version: %w", err)
		}
		ctx.deviceSmVersions[i] = major*10 + minor
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

	maps, err := kernel.ReadProcessMemMaps(pid, "/proc")
	if err != nil {
		return nil, fmt.Errorf("error reading process memory maps: %w", err)
	}

	ctx.pidMaps[pid] = &maps
	return &maps, nil
}

func (ctx *systemContext) cleanupDataForProcess(pid int) {
	delete(ctx.pidMaps, pid)
}

func (ctx *systemContext) cleanupOldEntries() {
	maxFatbinAge := 5 * time.Minute
	fatbinExpirationTime := time.Now().Add(-maxFatbinAge)

	for path, fd := range ctx.fileData {
		if fd.lastAccessed.Before(fatbinExpirationTime) {
			delete(ctx.fileData, path)
		}
	}
}
