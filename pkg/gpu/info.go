// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"debug/elf"
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

type gpuSystemInfo struct {
	deviceSmVersions map[int]int
	fileData         map[string]*fileData
	pidMaps          map[int]*kernel.ProcMapEntries
}

type fileData struct {
	symbolTable map[uint64]string
	fatbin      *Fatbin
}

func getGpuSystemInfo() (*gpuSystemInfo, error) {
	info := &gpuSystemInfo{
		fileData: make(map[string]*fileData),
		pidMaps:  make(map[int]*kernel.ProcMapEntries),
	}
	if err := info.queryDevices(); err != nil {
		return nil, fmt.Errorf("error querying devices: %w", err)
	}

	return info, nil
}

func (info *gpuSystemInfo) queryDevices() error {
	devices, err := GetGPUDevices()
	if err != nil {
		return fmt.Errorf("error getting GPU devices: %w", err)
	}

	info.deviceSmVersions = make(map[int]int)
	for i, device := range devices {
		major, minor, ret := device.GetCudaComputeCapability()
		if err = WrapNvmlError(ret); err != nil {
			return fmt.Errorf("error getting SM version: %w", err)
		}
		info.deviceSmVersions[i] = major*10 + minor
	}

	return nil
}

func (info *gpuSystemInfo) getFileData(path string) (*fileData, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("error getting absolute path: %w", err)
	}

	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("error reading link %s: %w", path, err)
	}

	if fd, ok := info.fileData[path]; ok {
		return fd, nil
	}

	elfFile, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening ELF file %s: %w", path, err)
	}

	fatbin, err := ParseFatbinFromELFFile(elfFile)
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

	info.fileData[path] = fd
	return info.fileData[path], nil
}

func (info *gpuSystemInfo) getProcessMemoryMaps(pid int) (*kernel.ProcMapEntries, error) {
	if maps, ok := info.pidMaps[pid]; ok {
		return maps, nil
	}

	maps, err := kernel.ReadProcessMemMaps(pid, "/proc")
	if err != nil {
		return nil, fmt.Errorf("error reading process memory maps: %w", err)
	}

	info.pidMaps[pid] = &maps
	return &maps, nil
}
