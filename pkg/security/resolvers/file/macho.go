// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"encoding/binary"
	"os"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func isMachO(header []byte, fileSize int64) bool {
	bytesToCheck := min(int(fileSize), len(header))
	if bytesToCheck < 4 {
		return false
	}
	machoMagic := binary.LittleEndian.Uint32(header)
	return machoMagic == 0xfeedface || machoMagic == 0xfeedfacf
}

func analyzeMachO(fileInfo os.FileInfo, header []byte, info *model.FileMetadata) error {
	abi, arch := getMachOArchAndABI(header, fileInfo.Size())
	info.ABI = int(abi)
	info.Architecture = int(arch)
	return nil
}

func getMachOArchAndABI(header []byte, fileSize int64) (model.ABI, model.Architecture) {
	bytesToCheck := min(int(fileSize), len(header))
	if bytesToCheck < 8 {
		return model.UnknownABI, model.UnknownArch
	}

	// Check magic number to determine ABI
	magic := binary.LittleEndian.Uint32(header)
	abi := model.UnknownABI
	if magic == 0xfeedfacf {
		abi = model.Bit64
	} else if magic == 0xfeedface {
		abi = model.Bit32
	}

	// Get CPU type from bytes 4-8
	cpuType := binary.LittleEndian.Uint32(header[4:8])
	arch := model.UnknownArch
	switch cpuType {
	case 0x01000007: // CPU_TYPE_X86_64
		arch = model.X8664
	case 0x0100000c: // CPU_TYPE_ARM64
		arch = model.ARM64
	case 0x00000007: // CPU_TYPE_I386
		arch = model.X86
	case 0x0000000c: // CPU_TYPE_ARM
		arch = model.ARM
	}
	return abi, arch
}
