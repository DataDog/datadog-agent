// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// isPE checks if the given header bytes represent a valid PE file by checking for the 'MZ' signature
func isPE(header []byte) bool {
	return len(header) >= 2 && header[0] == 'M' && header[1] == 'Z'
}

// determinePEArchitectureFromHeader analyzes the PE header to determine its ABI (32/64-bit) and architecture (x86/x86_64)
func determinePEArchitectureFromHeader(file *os.File, data []byte) (model.ABI, model.Architecture, error) {
	peOffset := binary.LittleEndian.Uint32(data[60:64])

	signature := make([]byte, 6)
	if peOffset+6 > uint32(len(data)) {
		if _, err := file.Seek(int64(peOffset), 0); err != nil {
			return model.UnknownABI, model.UnknownArch, err
		}
		if _, err := file.Read(signature); err != nil {
			return model.UnknownABI, model.UnknownArch, err
		}
	} else {
		signature = data[peOffset : peOffset+6]
	}

	if !bytes.Equal(signature[0:4], []byte{0x50, 0x45, 0x00, 0x00}) {
		return model.UnknownABI, model.UnknownArch, fmt.Errorf("invalid PE signature")
	}

	magic := binary.LittleEndian.Uint16(signature[4:])
	var abi model.ABI
	var arch model.Architecture
	switch magic {
	case pe.IMAGE_FILE_MACHINE_I386:
		arch = model.X86
		abi = model.Bit32
	case pe.IMAGE_FILE_MACHINE_AMD64:
		arch = model.X8664
		abi = model.Bit64
	case 0x01c4: // IMAGE_FILE_MACHINE_ARMNT
		arch = model.ARM
		abi = model.Bit32
	case pe.IMAGE_FILE_MACHINE_ARM64:
		arch = model.ARM64
		abi = model.Bit64
	default:
		arch = model.UnknownArch
		abi = model.UnknownABI
	}
	return abi, arch, nil
}

// getImportDirectoryOffset converts a Relative Virtual Address (RVA) to a file offset
func getImportDirectoryOffset(peFile *pe.File, rva uint32) uint32 {
	for _, section := range peFile.Sections {
		if rva >= section.VirtualAddress &&
			rva < section.VirtualAddress+section.Size {
			return section.Offset + (rva - section.VirtualAddress)
		}
	}
	return 0
}

// readDLLName reads and returns the name of a DLL from the specified offset in the file
func readDLLName(file *os.File, nameOffset uint32) (string, error) {
	if _, err := file.Seek(int64(nameOffset), 0); err != nil {
		return "", err
	}

	nameData := make([]byte, 256)
	n, err := file.Read(nameData)
	if err != nil || n == 0 {
		return "", err
	}

	end := 0
	for i := 0; i < n; i++ {
		if nameData[i] == 0 {
			end = i
			break
		}
	}
	if end == 0 {
		return "", fmt.Errorf("invalid DLL name")
	}

	return strings.ToLower(string(nameData[:end])), nil
}

// checkImportEntry verifies if an import entry is valid by comparing ILT and IAT entries
func checkImportEntry(iltData, iatData []byte, entrySize int, importDirRVA, importDirSize uint32) bool {
	for j := 0; j < len(iltData) && j < len(iatData); j += entrySize {
		if j+entrySize > len(iltData) || j+entrySize > len(iatData) {
			break
		}

		if entrySize == 8 {
			iltEntry := binary.LittleEndian.Uint64(iltData[j : j+8])
			iatEntry := binary.LittleEndian.Uint64(iatData[j : j+8])
			if iltEntry != 0 && iatEntry != 0 &&
				(iltEntry&0x8000000000000000) == 0 &&
				iltEntry != iatEntry &&
				iltEntry >= uint64(importDirRVA) &&
				iltEntry < uint64(importDirRVA+importDirSize) {
				return true
			}
		} else {
			iltEntry := binary.LittleEndian.Uint32(iltData[j : j+4])
			iatEntry := binary.LittleEndian.Uint32(iatData[j : j+4])
			if iltEntry != 0 && iatEntry != 0 &&
				(iltEntry&0x80000000) == 0 &&
				iltEntry != iatEntry &&
				iltEntry >= importDirRVA &&
				iltEntry < importDirRVA+importDirSize {
				return true
			}
		}
	}
	return false
}

// checkImports analyzes the import directory to determine if the file has imported libraries and system imports
func checkImports(peFile *pe.File, file *os.File, importDirSize, importDirRVA uint32, abi model.ABI) (bool, bool, error) {
	hasImportedLibraries := false
	hasSystemImports := false

	if importDirSize == 0 || importDirRVA == 0 {
		return false, false, nil
	}

	fileOffset := getImportDirectoryOffset(peFile, importDirRVA)
	if fileOffset == 0 {
		return false, false, nil
	}

	data := make([]byte, importDirSize)
	if _, err := file.Seek(int64(fileOffset), 0); err != nil {
		return false, false, err
	}
	if _, err := file.Read(data); err != nil {
		return false, false, err
	}

	for i := 0; i+20 <= len(data); i += 20 {
		iltRVA := binary.LittleEndian.Uint32(data[i : i+4])
		nameRVA := binary.LittleEndian.Uint32(data[i+12 : i+16])
		iatRVA := binary.LittleEndian.Uint32(data[i+16 : i+20])

		if iltRVA == 0 || nameRVA == 0 || iatRVA == 0 {
			continue
		}

		nameOffset := getImportDirectoryOffset(peFile, nameRVA)
		if nameOffset == 0 {
			continue
		}

		dllName, err := readDLLName(file, nameOffset)
		if err != nil {
			continue
		}

		iltOffset := getImportDirectoryOffset(peFile, iltRVA)
		iatOffset := getImportDirectoryOffset(peFile, iatRVA)
		if iltOffset == 0 || iatOffset == 0 {
			continue
		}

		iltData := make([]byte, 1024)
		if _, err := file.Seek(int64(iltOffset), 0); err != nil {
			continue
		}
		if _, err := file.Read(iltData); err != nil {
			continue
		}

		iatData := make([]byte, 1024)
		if _, err := file.Seek(int64(iatOffset), 0); err != nil {
			continue
		}
		if _, err := file.Read(iatData); err != nil {
			continue
		}

		entrySize := 8
		if abi == model.Bit32 {
			entrySize = 4
		}

		if checkImportEntry(iltData, iatData, entrySize, importDirRVA, importDirSize) {
			if strings.Contains(dllName, "kernel32") ||
				strings.Contains(dllName, "ntdll") ||
				strings.Contains(dllName, "msvcrt") {
				hasSystemImports = true
			} else {
				hasImportedLibraries = true
			}
		}
	}

	return hasImportedLibraries, hasSystemImports, nil
}

// getImportDirectoryInfo extracts the size and RVA of the import directory from the PE file
func getImportDirectoryInfo(peFile *pe.File) (uint32, uint32) {
	var importDirSize uint32
	var importDirRVA uint32
	switch oh := peFile.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		importDirSize = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT].Size
		importDirRVA = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT].VirtualAddress
	case *pe.OptionalHeader64:
		importDirSize = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT].Size
		importDirRVA = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT].VirtualAddress
	}
	return importDirSize, importDirRVA
}

// analyzePE performs a comprehensive analysis of a PE file, determining its architecture, ABI, and linkage type
func analyzePE(file *os.File, info *model.FileMetadata, data []byte, checkLinkage bool) error {
	// Try to determine architecture from PE header first
	abi, arch, err := determinePEArchitectureFromHeader(file, data)
	if err == nil {
		info.ABI = int(abi)
		info.Architecture = int(arch)
	}

	if checkLinkage {
		info.Linkage = int(model.Static)
		// Only try to parse PE file if it's not UPX packed
		if !info.IsUPXPacked {
			// Check if it's dynamically linked
			if checkLinkage {
				pefile, err := pe.NewFile(file)
				if err != nil {
					return err
				}
				defer pefile.Close()
				// Get import directory info
				importDirSize, importDirRVA := getImportDirectoryInfo(pefile)

				// Check if there's an import directory
				if importDirSize > 0 && importDirRVA > 0 {
					// Check for system imports and other library imports
					hasImportedLibraries, hasSystemImports, err := checkImports(pefile, file, importDirSize, importDirRVA, model.ABI(info.ABI))
					if err != nil {
						return err
					}

					// Mark as dynamic if we find any imports (system or non-system)
					if hasImportedLibraries || hasSystemImports {
						info.Linkage = int(model.Dynamic)
					}
				}
			}
		}
	}
	return nil
}
