// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run golang.org/x/tools/cmd/stringer@latest -output fatbin_string.go -type=nvInfoAttr,nvInfoFormat,fatbinDataKind -linecomment

// package cuda provides helpers for CUDA binary parsing
package cuda

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unsafe"

	"github.com/pierrec/lz4/v4"
)

type fatbinDataKind uint16

const fatbinDataKindPtx fatbinDataKind = 1
const fatbinDataKindSm fatbinDataKind = 2

const fatbinMagic = 0xBA55ED50
const fatbinHeaderVersion = 1
const fatbinDataVersion = 0x0101
const fatbinDataMinKind = fatbinDataKindPtx
const fatbinDataMaxKind = fatbinDataKindSm

// Fatbin holds all CUDA binaries found in one fatbin package
type Fatbin struct {
	Kernels map[CubinKernelKey]*CubinKernel
}

// CubinKernelKey is the key to identify a kernel in a fatbin
type CubinKernelKey struct {
	Name      string
	SmVersion uint32
}

// CubinKernel holds the information of a CUDA kernel
type CubinKernel struct {
	Name        string                          // Name of the kernel
	attributes  map[nvInfoAttr]nvInfoParsedItem // Attributes of the kernel
	SymtabIndex int                             // Index of this kernel in the ELF symbol table
	KernelSize  uint64                          // Size of the kernel in bytes
	SharedMem   uint64                          // Size of the shared memory used by the kernel
	ConstantMem uint64                          // Size of the constant memory used by the kernel
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

// ParseFatbinFromELFFilePath opens the given path and parses the resulting ELF for CUDA kernels
func ParseFatbinFromELFFilePath(path string) (*Fatbin, error) {
	elfFile, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open ELF file %s: %w", path, err)
	}
	defer elfFile.Close()

	return ParseFatbinFromELFFile(elfFile)
}

// ParseFatbinFromPath parses the fatbin sections of the given ELF file and returns the information found in it
func ParseFatbinFromELFFile(elfFile *elf.File) (*Fatbin, error) {
	fatbin := &Fatbin{
		Kernels: make(map[CubinKernelKey]*CubinKernel),
	}

	for _, sect := range elfFile.Sections {
		// CUDA embeds the fatbin data in sections named .nv_fatbin or __nv_relfatbin
		if sect.Name != ".nv_fatbin" && sect.Name != "__nv_relfatbin" {
			continue
		}

		data, err := sect.Data()
		if err != nil {
			return nil, fmt.Errorf("failed to read section %s: %w", sect.Name, err)
		}

		// The fatbin format will have a header, then a sequence of data headers + payloads.
		// After the data corresponding to the first header, we might have more headers so
		// we need to loop until we reach the end of the section data
		buffer := bytes.NewReader(data)
		for buffer.Len() > 0 {
			var fatbinHeader fatbinHeader
			err = binary.Read(buffer, binary.LittleEndian, &fatbinHeader)
			if err != nil {
				return nil, fmt.Errorf("failed to parse fatbin header: %w", err)
			}

			// Check the header is valid
			if fatbinHeader.Magic != fatbinMagic {
				return nil, fmt.Errorf("invalid fatbin header, magic number %x does not match expected %x", fatbinHeader.Magic, fatbinMagic)
			}
			if fatbinHeader.Version != fatbinHeaderVersion {
				return nil, fmt.Errorf("invalid fatbin header, version %d does not match expected %d", fatbinHeader.Version, fatbinHeaderVersion)
			}

			// We need to read only up to the size given to us by the header, not to the end of the section.
			readStart, _ := buffer.Seek(0, io.SeekCurrent)
			for currOffset, _ := buffer.Seek(0, io.SeekCurrent); buffer.Len() > 0 && currOffset-readStart < int64(fatbinHeader.FatSize); currOffset, _ = buffer.Seek(0, io.SeekCurrent) {
				// Each data section starts with a data header, read it
				var fatbinData fatbinData
				err = binary.Read(buffer, binary.LittleEndian, &fatbinData)
				if err != nil {
					return nil, fmt.Errorf("failed to parse fatbin data: %w", err)
				}

				// Check that we have a valid header
				dataKind := fatbinDataKind(fatbinData.Kind)
				if dataKind < fatbinDataMinKind || dataKind > fatbinDataMaxKind {
					return nil, fmt.Errorf("invalid fatbin data, kind %d is not in the expected range [%d, %d]", dataKind, fatbinDataMinKind, fatbinDataMaxKind)
				}
				if fatbinData.Version != fatbinDataVersion {
					return nil, fmt.Errorf("invalid fatbin data, version %d does not match expected %d", fatbinData.Version, fatbinDataVersion)
				}

				// The header size is the size of the struct, but the actual header in file might be larger
				// If that's the case, we need to skip the rest of the header
				fatbinDataSize := uint32(unsafe.Sizeof(fatbinData))
				if fatbinData.HeaderSize > fatbinDataSize {
					_, err = buffer.Seek(int64(fatbinData.HeaderSize-fatbinDataSize), io.SeekCurrent)
					if err != nil {
						return nil, fmt.Errorf("failed to skip rest of fatbin data header: %w", err)
					}
				}

				if dataKind != fatbinDataKindSm {
					// We only support SM data for now, skip this one
					_, err := buffer.Seek(int64(fatbinData.PaddedPayloadSize), io.SeekCurrent)
					if err != nil {
						return nil, fmt.Errorf("failed to skip PTX fatbin data: %w", err)
					}
					continue
				}

				// Now read the payload. Fatbin format could have both compressed and uncompressed payloads
				var payload []byte
				if fatbinData.UncompressedPayloadSize != 0 {
					compressedPayload := make([]byte, fatbinData.PaddedPayloadSize)
					_, err := io.ReadFull(buffer, compressedPayload)
					if err != nil {
						return nil, fmt.Errorf("failed to read fatbin compressed payload: %w", err)
					}

					// Keep only the actual payload, ignore the padding for the uncompression
					compressedPayload = compressedPayload[:fatbinData.PayloadSize]

					payload = make([]byte, fatbinData.UncompressedPayloadSize)
					_, err = lz4.UncompressBlock(compressedPayload, payload)
					if err != nil {
						return nil, fmt.Errorf("failed to decompress fatbin payload: %w", err)
					}
				} else {
					payload = make([]byte, fatbinData.PaddedPayloadSize)
					_, err := io.ReadFull(buffer, payload)
					if err != nil {
						return nil, fmt.Errorf("failed to read fatbin payload: %w", err)
					}
				}

				// The payload is a cubin ELF, parse it with the cubin parser
				parser := newCubinParser()
				err = parser.parseCubinElf(payload)
				if err != nil {
					return nil, fmt.Errorf("failed to parse cubin ELF: %w", err)
				}

				// Retrieve all the kernels found in the cubin parser and add them to the fatbin, including
				// the SM version they were compiled for which is only available in the fatbin data
				for _, kernel := range parser.kernels {
					key := CubinKernelKey{Name: kernel.Name, SmVersion: fatbinData.SmVersion}
					fatbin.Kernels[key] = kernel
				}
			}
		}
	}

	return fatbin, nil
}

// nvInfoFormat defines the format (size) of a value of a .nv.info item
type nvInfoFormat uint8

const (
	nviFmtNone nvInfoFormat = 0x01
	nviFmtBval nvInfoFormat = 0x02
	nviFmtHval nvInfoFormat = 0x03
	nviFmtSval nvInfoFormat = 0x04
)

// nvInfoAttr defines the attribute of a .nv.info item
type nvInfoAttr uint8

const (
	nviAttrError nvInfoAttr = iota
	nviAttrPad
	nviAttrImageSlot
	nviAttrJumptableRelocs
	nviAttrCtaidzUsed
	nviAttrMaxThreads
	nviAttrImageOffset
	nviAttrImageSize
	nviAttrTextureNormalized
	nviAttrSamplerInit
	nviAttrParamCbank
	nviAttrSmemParamOffsets
	nviAttrCbankParamOffsets
	nviAttrSyncStack
	nviAttrTexidSampidMap
	nviAttrExterns
	nviAttrReqntid
	nviAttrFrameSize
	nviAttrMinStackSize
	nviAttrSamplerForceUnnormalized
	nviAttrBindlessImageOffsets
	nviAttrBindlessTextureBank
	nviAttrBindlessSurfaceBank
	nviAttrKparamInfo
	nviAttrSmemParamSize
	nviAttrCbankParamSize
	nviAttrQueryNumattrib
	nviAttrMaxregCount
	nviAttrExitInstrOffsets
	nviAttrS2rctaidInstrOffsets
	nviAttrCrsStackSize
	nviAttrNeedCnpWrapper
	nviAttrNeedCnpPatch
	nviAttrExplicitCaching
	nviAttrIstypepUsed
	nviAttrMaxStackSize
	nviAttrSuqUsed
	nviAttrLdCachemodInstrOffsets
	nviAttrLoadCacheRequest
	nviAttrAtomSysInstrOffsets
	nviAttrCoopGroupInstrOffsets
	nviAttrCoopGroupMaxRegids
	nviAttrSw1850030War
	nviAttrWmmaUsed
	nviAttrHasPreV10Object
	nviAttrAtomf16EmulInstrOffsets
	nviAttrAtom16EmulInstrRegMap
	nviAttrRegcount
	nviAttrSw2393858War
	nviAttrIntWarpWideInstrOffsets
	nviAttrSharedScratch
	nviAttrStatistics

	// New between cuda 10.2 and 11.6
	nviAttrIndirectBranchTargets
	nviAttrSw2861232War
	nviAttrSwWar
	nviAttrCudaApiVersion
	nviAttrNumMbarriers
	nviAttrMbarrierInstrOffsets
	nviAttrCoroutineResumeIdOffsets
	nviAttrSamRegionStackSize
	nviAttrPerRegTargetPerfStats

	// New between cuda 11.6 and 11.8
	nviAttrCtaPerCluster
	nviAttrExplicitCluster
	nviAttrMaxClusterRank
	nviAttrInstrRegMap
)

// nvInfoItem is the in-file representation of an item  header in the .nv.info section. The value follows
// according to the format
type nvInfoItem struct {
	Format nvInfoFormat
	Attr   nvInfoAttr
}

// cubinParser is a helper struct to parse the cubin ELF sections
type cubinParser struct {
	kernels            map[string]*CubinKernel
	sectPrefixToParser map[string]func(*elf.Section, *bytes.Reader, string) error
}

func newCubinParser() cubinParser {
	cp := cubinParser{
		kernels:            make(map[string]*CubinKernel),
		sectPrefixToParser: make(map[string]func(*elf.Section, *bytes.Reader, string) error),
	}

	cp.sectPrefixToParser[".nv.info"] = cp.parseNvInfoSection
	cp.sectPrefixToParser[".text"] = cp.parseTextSection
	cp.sectPrefixToParser[".nv.shared"] = cp.parseSharedMemSection
	cp.sectPrefixToParser[".nv.constant"] = cp.parseConstantMemSection

	return cp
}

func (cp *cubinParser) getKernel(name string) *CubinKernel {
	if _, ok := cp.kernels[name]; !ok {
		cp.kernels[name] = &CubinKernel{
			Name: name,
		}
	}
	return cp.kernels[name]
}

func (cp *cubinParser) parseCubinElf(data []byte) error {
	// Hacks to be able to parse the ELF: the ELF version is not supported by the Go ELF parser, so we need to
	// trick it into thinking it's the old version. Check for boundaries first
	if len(data) < 21 {
		return fmt.Errorf("invalid cubin data, too short")
	}
	data[20] = 1

	cubinElf, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to parse cubin ELF: %w", err)
	}
	defer cubinElf.Close()

	// Iterate through all the sections, parse all the ones we know how to parse
	for _, sect := range cubinElf.Sections {
		for prefix, parser := range cp.sectPrefixToParser {
			prefixWithDot := prefix + "."

			if !strings.HasPrefix(sect.Name, prefix) {
				continue
			}

			var buffer *bytes.Reader
			if sect.Type != elf.SHT_NOBITS {
				// do not provide a buffer for NOBITS sections
				sectData, err := sect.Data()
				if err != nil {
					return fmt.Errorf("failed to read section %s: %w", sect.Name, err)
				}
				buffer = bytes.NewReader(sectData)
			}

			var kernelName string
			if strings.HasPrefix(sect.Name, prefixWithDot) {
				kernelName = strings.TrimPrefix(sect.Name, prefixWithDot)
			}

			err = parser(sect, buffer, kernelName)
			if err != nil {
				return fmt.Errorf("failed to parse section %s: %w", sect.Name, err)
			}
		}
	}

	return nil
}

// nvInfoParsedItem is the parsed representation of an item in the .nv.info section, including the value
type nvInfoParsedItem struct {
	nvInfoItem
	value []byte
}

func (cp *cubinParser) parseNvInfoSection(_ *elf.Section, buffer *bytes.Reader, kernelName string) error {
	items := make(map[nvInfoAttr]nvInfoParsedItem)

	for buffer.Len() > 0 {
		var item nvInfoItem
		if err := binary.Read(buffer, binary.LittleEndian, &item); err != nil {
			return fmt.Errorf("failed to read item: %w", err)
		}

		var valueSize int

		switch item.Format {
		case nviFmtBval:
			valueSize = 1
		case nviFmtHval:
			valueSize = 2
		case nviFmtSval:
			var nval uint16
			if err := binary.Read(buffer, binary.LittleEndian, &nval); err != nil {
				return fmt.Errorf("failed to parse value size: %w", err)
			}
			valueSize = int(nval)
		}

		parsedItem := nvInfoParsedItem{
			nvInfoItem: item,
			value:      make([]byte, valueSize),
		}
		if _, err := io.ReadFull(buffer, parsedItem.value); err != nil {
			return fmt.Errorf("failed to read value: %w", err)
		}
		items[item.Attr] = parsedItem
	}

	if kernelName != "" {
		cp.getKernel(kernelName).attributes = items
	}

	return nil
}

func (cp *cubinParser) parseTextSection(sect *elf.Section, _ *bytes.Reader, kernelName string) error {
	if kernelName == "" {
		return nil
	}

	kernel := cp.getKernel(kernelName)
	kernel.SymtabIndex = int(sect.Info & 0xff)
	kernel.KernelSize = sect.Size

	return nil
}

func (cp *cubinParser) parseSharedMemSection(sect *elf.Section, _ *bytes.Reader, kernelName string) error {
	if kernelName == "" {
		return nil
	}

	kernel := cp.getKernel(kernelName)
	kernel.SharedMem = sect.Size

	return nil
}

var constantSectNameRegex = regexp.MustCompile(`\.nv\.constant\d\.(.*)`)

func (cp *cubinParser) parseConstantMemSection(sect *elf.Section, _ *bytes.Reader, kernelName string) error {
	// Constant memory sections are named .nv.constantX.Y where X is the constant memory index and Y is the name
	// so we have to do some custom parsing
	match := constantSectNameRegex.FindStringSubmatch(sect.Name)
	if match == nil {
		// Not a constant memory section. We might be missing the kernel name, for example, which happens
		// on some binaries. In that case just ignore the section
		return nil
	}

	kernelName = match[1]
	kernel := cp.getKernel(kernelName)
	kernel.ConstantMem += sect.Size

	return nil
}
