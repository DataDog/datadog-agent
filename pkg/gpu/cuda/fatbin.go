// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This file contains a parser for the Cubin/Fatbin format.
// References:
// - https://github.com/VivekPanyam/cudaparsers
// - https://pdfs.semanticscholar.org/5096/25785304410039297b741ad2007e7ce0636b.pdf
// - https://github.com/cloudcores/CuAssembler/blob/master/CuAsm/CuNVInfo.py
// - https://datadoghq.atlassian.net/wiki/spaces/EBPFTEAM/pages/4084204125/Fatbin+Cubin+binary+format

// Generate String() methods for fatbinDataKind enums so they can be printed in logs/error messages
//go:generate go run golang.org/x/tools/cmd/stringer@latest -output fatbin_string.go -type=fatbinDataKind -linecomment

// Package cuda provides helpers for CUDA binary parsing
package cuda

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"unsafe"

	"github.com/pierrec/lz4/v4"
)

type fatbinDataKind uint16

const fatbinDataKindPtx fatbinDataKind = 1
const fatbinDataKindSm fatbinDataKind = 2

const fatbinMagic uint32 = 0xBA55ED50
const fatbinHeaderVersion uint16 = 1
const fatbinDataVersion uint16 = 0x0101
const fatbinDataMinKind = fatbinDataKindPtx
const fatbinDataMaxKind = fatbinDataKindSm

// Fatbin holds all CUDA binaries found in one fatbin package
type Fatbin struct {
	Kernels map[CubinKernelKey]*CubinKernel
}

// GetKernel returns the kernel with the given name and SM version from the fatbin
func (fb *Fatbin) GetKernel(name string, smVersion uint32) *CubinKernel {
	key := CubinKernelKey{Name: name, SmVersion: smVersion}
	if _, ok := fb.Kernels[key]; !ok {
		return nil
	}
	return fb.Kernels[key]
}

type fatbinHeader struct {
	Magic      uint32
	Version    uint16
	HeaderSize uint16
	FatSize    uint64 // not including the header
}

func (fbh *fatbinHeader) validate() error {
	if fbh.Magic != fatbinMagic {
		return fmt.Errorf("invalid fatbin header, magic number %x does not match expected %x", fbh.Magic, fatbinMagic)
	}
	if fbh.Version != fatbinHeaderVersion {
		return fmt.Errorf("invalid fatbin header, version %d does not match expected %d", fbh.Version, fatbinHeaderVersion)
	}

	return nil
}

type fatbinData struct {
	Kind                    uint16
	Version                 uint16
	HeaderSize              uint32
	PaddedPayloadSize       uint32
	Unknown0                uint32 // check if it's written into separately
	PayloadSize             uint32
	Unknown1                uint32
	Unknown2                uint32
	SmVersion               uint32
	BitWidth                uint32
	Unknown3                uint32
	Unknown4                uint64
	Unknown5                uint64
	UncompressedPayloadSize uint64
}

func (fbd *fatbinData) dataKind() fatbinDataKind {
	return fatbinDataKind(fbd.Kind)
}

func (fbd *fatbinData) validate() error {
	dataKind := fbd.dataKind()
	if dataKind < fatbinDataMinKind || dataKind > fatbinDataMaxKind {
		return fmt.Errorf("kind %d is not in the expected range [%d, %d]", dataKind, fatbinDataMinKind, fatbinDataMaxKind)
	}
	if fbd.Version != fatbinDataVersion {
		return fmt.Errorf("version %d does not match expected %d", fbd.Version, fatbinDataVersion)
	}

	return nil
}

// ParseFatbinFromELFFilePath opens the given path and parses the resulting ELF for CUDA kernels
func ParseFatbinFromELFFilePath(path string) (*Fatbin, error) {
	elfFile, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open ELF file %s: %w", path, err)
	}
	defer elfFile.Close()

	return ParseFatbinFromELFFile(elfFile)
}

func getBufferOffset(buf io.Seeker) int64 {
	offset, _ := buf.Seek(0, io.SeekCurrent)
	return offset
}

// ParseFatbinFromELFFile parses the fatbin sections of the given ELF file and returns the information found in it
func ParseFatbinFromELFFile(elfFile *elf.File) (*Fatbin, error) {
	fatbin := &Fatbin{
		Kernels: make(map[CubinKernelKey]*CubinKernel),
	}

	for _, sect := range elfFile.Sections {
		// CUDA embeds the fatbin data in sections named .nv_fatbin or __nv_relfatbin
		if sect.Name != ".nv_fatbin" && sect.Name != "__nv_relfatbin" {
			continue
		}

		// Use the Open method to avoid reading everything into memory
		buffer := sect.Open()

		// The fatbin format will have a header, then a sequence of data headers + payloads.
		// After the data corresponding to the first header, we might have more headers so
		// we need to loop until we reach the end of the section data.
		// Illustration of the format
		//
		// fatbinHeader           (16 bytes)
		//   fatbinDataHeader     (64 bytes minimum)
		//   fatbinDataPayload    (variable size)
		//   fatbinDataHeader     (64 bytes minimum)
		//   fatbinDataPayload    (variable size)
		// fatbinHeader           (16 bytes)
		//   fatbinDataHeader     (64 bytes minimum)
		//   fatbinDataPayload    (variable size)
		for {
			var fbHeader fatbinHeader
			err := binary.Read(buffer, binary.LittleEndian, &fbHeader)
			if err != nil {
				if err == io.EOF {
					break
				}

				return nil, fmt.Errorf("failed to parse fatbin header: %w", err)
			}

			// Check the header is valid
			if err := fbHeader.validate(); err != nil {
				return nil, fmt.Errorf("invalid fatbin header: %w", err)
			}

			// We need to read only up to the size given to us by the header, not to the end of the section.
			readStart := getBufferOffset(buffer)
			for currOffset := getBufferOffset(buffer); uint64(currOffset-readStart) < fbHeader.FatSize; currOffset = getBufferOffset(buffer) {
				if err := parseFatbinData(buffer, fatbin); err != nil {
					if err == io.EOF {
						break
					}

					return nil, fmt.Errorf("failed to parse fatbin data: %w", err)
				}
			}
		}
	}

	return fatbin, nil
}

func parseFatbinData(buffer io.ReadSeeker, fatbin *Fatbin) error {
	// Each data section starts with a data header, read it
	var fbData fatbinData
	err := binary.Read(buffer, binary.LittleEndian, &fbData)
	if err != nil {
		if err == io.EOF {
			return err
		}

		return fmt.Errorf("failed to parse fatbin data: %w", err)
	}

	// Check that we have a valid header
	if err := fbData.validate(); err != nil {
		return fmt.Errorf("invalid fatbin data: %w", err)
	}

	// The header size is the size of the struct, but the actual header in file might be larger
	// If that's the case, we need to skip the rest of the header
	fatbinDataSize := uint32(unsafe.Sizeof(fbData))
	if fbData.HeaderSize > fatbinDataSize {
		_, err = buffer.Seek(int64(fbData.HeaderSize-fatbinDataSize), io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to skip rest of fatbin data header: %w", err)
		}
	}

	if fbData.dataKind() != fatbinDataKindSm {
		// We only support SM data for now, skip this one
		_, err := buffer.Seek(int64(fbData.PaddedPayloadSize), io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to skip PTX fatbin data: %w", err)
		}

		return nil // Skip this data section
	}

	// Now read the payload. Fatbin format could have both compressed and uncompressed payloads. The way this is
	// determined is by the UncompressedPayloadSize field. If it's non-zero, it indicates the size the payload will
	// have once uncompressed. If it's zero, the payload is not compressed.
	var payload []byte
	if fbData.UncompressedPayloadSize != 0 {
		compressedPayload := make([]byte, fbData.PaddedPayloadSize)
		_, err := io.ReadFull(buffer, compressedPayload)
		if err != nil {
			return fmt.Errorf("failed to read fatbin compressed payload: %w", err)
		}

		// Keep only the actual payload, ignore the padding for the uncompression
		compressedPayload = compressedPayload[:fbData.PayloadSize]

		payload = make([]byte, fbData.UncompressedPayloadSize)
		_, err = lz4.UncompressBlock(compressedPayload, payload)
		if err != nil {
			return fmt.Errorf("failed to decompress fatbin payload: %w", err)
		}
	} else {
		payload = make([]byte, fbData.PaddedPayloadSize)
		_, err := io.ReadFull(buffer, payload)
		if err != nil {
			return fmt.Errorf("failed to read fatbin payload: %w", err)
		}
	}

	// The payload is a cubin ELF, parse it with the cubin parser
	parser := newCubinParser()
	err = parser.parseCubinElf(payload)
	if err != nil {
		return fmt.Errorf("failed to parse cubin ELF: %w", err)
	}

	// Retrieve all the kernels found in the cubin parser and add them to the fatbin, including
	// the SM version they were compiled for which is only available in the fatbin data
	for _, kernel := range parser.kernels {
		key := CubinKernelKey{Name: kernel.Name, SmVersion: fbData.SmVersion}
		fatbin.Kernels[key] = kernel
	}

	return nil
}
