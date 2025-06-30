// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// PatchOrReleaseCandidate represents either a patch version or release candidate
type PatchOrReleaseCandidate struct {
	IsPatch bool
	Version uint16
}

// GoVersion represents a parsed version string
type GoVersion struct {
	Major     uint16
	Minor     uint16
	PatchOrRC PatchOrReleaseCandidate
}

// ParseGoVersion extracts the Go version from an object file
func ParseGoVersion(mef *MMappingElfFile) (*GoVersion, error) {
	// Find the runtime.buildVersion symbol
	symbols, err := mef.Elf.Symbols()
	if err != nil {
		return nil, fmt.Errorf("failed to get symbols: %w", err)
	}

	var buildVersionSym *safeelf.Symbol
	for _, sym := range symbols {
		if sym.Name == "runtime.buildVersion" {
			buildVersionSym = &sym
			break
		}
	}

	if buildVersionSym == nil {
		return nil, fmt.Errorf("runtime.buildVersion not found")
	}

	// Find the section containing the symbol
	var section *safeelf.Section
	for _, s := range mef.Elf.Sections {
		if s.Addr <= buildVersionSym.Value && buildVersionSym.Value < s.Addr+s.Size {
			section = s
			break
		}
	}

	if section == nil {
		return nil, fmt.Errorf("section containing runtime.buildVersion not found")
	}

	// Read the string
	versionStr, err := readString(mef, section, buildVersionSym.Value, buildVersionSym.Size)
	if err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}

	return parseGoVersion(versionStr), nil
}

func readString(mef *MMappingElfFile, section *safeelf.Section, address, size uint64) (string, error) {
	ms, err := mef.MMap(section, 0, section.Size)
	if err != nil {
		return "", fmt.Errorf("failed to load section data: %w", err)
	}
	defer ms.Close()

	offset := address - section.Addr
	if offset+size > uint64(len(ms.Data)) {
		return "", fmt.Errorf("string data out of bounds")
	}

	// Handle string header based on size
	var dataAddr, dataSize uint64
	if size == 8 {
		// 32-bit pointers
		if offset+8 > uint64(len(ms.Data)) {
			return "", fmt.Errorf("not enough data for 32-bit string header")
		}
		dataAddr = uint64(binary.LittleEndian.Uint32(ms.Data[offset:]))
		dataSize = uint64(binary.LittleEndian.Uint32(ms.Data[offset+4:]))
	} else if size == 16 {
		// 64-bit pointers
		if offset+16 > uint64(len(ms.Data)) {
			return "", fmt.Errorf("not enough data for 64-bit string header")
		}
		dataAddr = binary.LittleEndian.Uint64(ms.Data[offset:])
		dataSize = binary.LittleEndian.Uint64(ms.Data[offset+8:])
	} else {
		return "", fmt.Errorf("invalid string header size: %d", size)
	}

	// Find the section containing the actual string data
	var dataSection *safeelf.Section
	for _, s := range mef.Elf.Sections {
		if s.Addr <= dataAddr && dataAddr < s.Addr+s.Size {
			dataSection = s
			break
		}
	}

	if dataSection == nil {
		return "", fmt.Errorf("failed to find data section")
	}

	mds, err := mef.MMap(dataSection, 0, dataSection.Size)
	if err != nil {
		return "", fmt.Errorf("failed to load data section: %w", err)
	}
	defer mds.Close()

	dataOffset := dataAddr - dataSection.Addr
	if dataOffset+dataSize > uint64(len(mds.Data)) {
		return "", fmt.Errorf("string data out of bounds in data section")
	}

	return string(mds.Data[dataOffset : dataOffset+dataSize]), nil
}

var goVersionRegex = regexp.MustCompile(`^go(\d+)\.(\d+)(\.(\d+)|rc(\d+))`)

func parseGoVersion(version string) *GoVersion {
	matches := goVersionRegex.FindStringSubmatch(version)
	if matches == nil {
		return nil
	}

	major, err := strconv.ParseUint(matches[1], 10, 16)
	if err != nil {
		return nil
	}

	minor, err := strconv.ParseUint(matches[2], 10, 16)
	if err != nil {
		return nil
	}

	var patchOrRC PatchOrReleaseCandidate
	if matches[4] != "" { // patch version
		patch, err := strconv.ParseUint(matches[4], 10, 16)
		if err != nil {
			return nil
		}
		patchOrRC = PatchOrReleaseCandidate{IsPatch: true, Version: uint16(patch)}
	} else if matches[5] != "" { // release candidate
		rc, err := strconv.ParseUint(matches[5], 10, 16)
		if err != nil {
			return nil
		}
		patchOrRC = PatchOrReleaseCandidate{IsPatch: false, Version: uint16(rc)}
	}

	return &GoVersion{
		Major:     uint16(major),
		Minor:     uint16(minor),
		PatchOrRC: patchOrRC,
	}
}
