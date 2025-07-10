// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func isELF(header []byte, fileSize int64) bool {
	bytesToCheck := min(int(fileSize), len(header))
	return bytesToCheck >= 4 && header[0] == 0x7f && header[1] == 'E' && header[2] == 'L' && header[3] == 'F'
}

func getELFInfoFromHeader(header []byte) (model.ABI, model.Architecture, error) {
	if len(header) < 20 {
		return model.UnknownABI, model.UnknownArch, fmt.Errorf("header too short")
	}

	// Get ABI from EI_CLASS (byte 4)
	var abi model.ABI
	switch header[4] {
	case 1: // ELFCLASS32
		abi = model.Bit32
	case 2: // ELFCLASS64
		abi = model.Bit64
	default:
		abi = model.UnknownABI
	}

	// Get endianness from EI_DATA (byte 5)
	var machine uint16
	switch header[5] {
	case 1: // ELFDATA2LSB (little-endian)
		machine = uint16(header[18]) | uint16(header[19])<<8
	case 2: // ELFDATA2MSB (big-endian)
		machine = uint16(header[18])<<8 | uint16(header[19])
	default:
		return abi, model.UnknownArch, fmt.Errorf("unknown endianness")
	}

	// Get architecture from e_machine (bytes 18-19)
	var arch model.Architecture
	switch machine {
	case 3: // EM_386
		arch = model.X86
	case 62: // EM_X86_64
		arch = model.X8664
	case 40: // EM_ARM
		arch = model.ARM
	case 183: // EM_AARCH64
		arch = model.ARM64
	default:
		arch = model.UnknownArch
	}

	return abi, arch, nil
}

func analyzeELF(file *os.File, data []byte, info *model.FileMetadata, checkLinkage bool) error {
	// Get ABI and architecture from header first
	abi, arch, err := getELFInfoFromHeader(data)
	if err != nil {
		return err
	}
	info.ABI = int(abi)
	info.Architecture = int(arch)

	// Parse full ELF file for linkage check if needed
	if checkLinkage {
		elfFile, err := safeelf.NewFile(file)
		if err != nil {
			return err
		}
		defer elfFile.Close()

		// Default to static
		info.Linkage = int(model.Static)

		// Check for dynamic linking indicators
		if section := elfFile.Section(".interp"); section != nil {
			info.Linkage = int(model.Dynamic)
			return nil
		}
	}
	return nil
}
