// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Generate String() methods for nvInfoAttr,nvInfoFormat enums so they can be printed in logs/error messages
//go:generate go run golang.org/x/tools/cmd/stringer@latest -output cubin_string.go -type=nvInfoAttr,nvInfoFormat -linecomment

package cuda

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"regexp"
)

const maxCudaKernelNameLength = 256

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
	nviAttrCudaAPIVersion
	nviAttrNumMbarriers
	nviAttrMbarrierInstrOffsets
	nviAttrCoroutineResumeIDOffsets
	nviAttrSamRegionStackSize
	nviAttrPerRegTargetPerfStats

	// New between cuda 11.6 and 11.8
	nviAttrCtaPerCluster
	nviAttrExplicitCluster
	nviAttrMaxClusterRank
	nviAttrInstrRegMap
)

// enabledNvInfoAttrs is a map of the attributes we care about in the .nv.info section
// Parsing all of them is not necessary and some of have quite a lot of data
var enabledNvInfoAttrs = map[nvInfoAttr]struct{}{}

// nvInfoItem is the in-file representation of an item  header in the .nv.info section. The value follows
// according to the format
type nvInfoItem struct {
	Format nvInfoFormat
	Attr   nvInfoAttr
}

type sectionParserFunc func(*elfSection, []byte) error

type sectionParser struct {
	prefix []byte
	parser sectionParserFunc
}

// cubinParser is a helper struct to parse the cubin ELF sections
type cubinParser struct {
	// kernels is the internal index for the parsed kernels for this cubin file.
	// The key is the kernel name as a byte array to avoid string allocations on lookup,
	// as in some situations we are parsing a lot of kernels.
	kernels        map[[maxCudaKernelNameLength]byte]*CubinKernel
	sectionParsers []sectionParser
}

func newCubinParser() *cubinParser {
	cp := &cubinParser{
		kernels: make(map[[maxCudaKernelNameLength]byte]*CubinKernel),
	}

	cp.sectionParsers = []sectionParser{
		{prefix: []byte(".nv.info"), parser: cp.parseNvInfoSection},
		{prefix: []byte(".text"), parser: cp.parseTextSection},
		{prefix: []byte(".nv.shared"), parser: cp.parseSharedMemSection},
		{prefix: []byte(".nv.constant"), parser: cp.parseConstantMemSection},
	}

	return cp
}

func (cp *cubinParser) getOrCreateKernel(name []byte) *CubinKernel {
	var nameStr [maxCudaKernelNameLength]byte
	copy(nameStr[:], name)

	if _, ok := cp.kernels[nameStr]; !ok {
		cp.kernels[nameStr] = &CubinKernel{
			Name: string(name),
		}
	}
	return cp.kernels[nameStr]
}

const elfVersionOffset = 20

func (cp *cubinParser) parseCubinElf(data []byte) error {
	// Hacks to be able to parse the ELF: the ELF version is not supported by the Go ELF parser, so we need to
	// trick it into thinking it's the old version. Check for boundaries first
	if len(data) <= elfVersionOffset {
		return errors.New("invalid cubin data, too short")
	}
	data[elfVersionOffset] = 1

	lazyReader := newLazySectionReader(bytes.NewReader(data))

	// Iterate through all the sections, parse all the ones we know how to parse
	for sect := range lazyReader.Iterate() {
		for _, parser := range cp.sectionParsers {
			if !bytes.HasPrefix(sect.nameBytes, parser.prefix) {
				continue
			}

			var kernelName []byte
			if len(sect.nameBytes) > len(parser.prefix) && sect.nameBytes[len(parser.prefix)] == '.' {
				kernelName = sect.nameBytes[len(parser.prefix)+1:]
			}

			err := parser.parser(sect, kernelName)
			if err != nil {
				return fmt.Errorf("failed to parse section %s: %w", sect.Name(), err)
			}
		}
	}

	if err := lazyReader.Err(); err != nil {
		return fmt.Errorf("failed to read ELF sections: %w", err)
	}

	return nil
}

// nvInfoParsedItem is the parsed representation of an item in the .nv.info section, including the value
type nvInfoParsedItem struct {
	nvInfoItem
	value []byte
}

func (cp *cubinParser) parseNvInfoSection(sect *elfSection, kernelName []byte) error {
	if len(enabledNvInfoAttrs) == 0 || len(kernelName) == 0 {
		// if there are no enabled attributes, we don't need to parse the section
		// same if there's no kernel name
		return nil
	}

	items := make(map[nvInfoAttr]nvInfoParsedItem)
	buffer := sect.Reader()

	for {
		var item nvInfoItem
		if err := binary.Read(buffer, binary.LittleEndian, &item); err != nil {
			if err == io.EOF {
				break
			}

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
		case nviFmtNone:
			valueSize = 2 // Doesn't really make sense as that data isn't used, but we need to skip it
		default:
			return fmt.Errorf("unsupported nvInfoFormat %d", item.Format)
		}

		_, enabled := enabledNvInfoAttrs[item.Attr]
		if !enabled {
			// Skip the value if we don't care about this attribute
			if _, err := buffer.Seek(int64(valueSize), io.SeekCurrent); err != nil {
				return fmt.Errorf("failed to skip value of size %d: %w", valueSize, err)
			}
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

	cp.getOrCreateKernel(kernelName).attributes = items

	return nil
}

func (cp *cubinParser) parseTextSection(sect *elfSection, kernelName []byte) error {
	if len(kernelName) == 0 {
		return nil
	}

	kernel := cp.getOrCreateKernel(kernelName)
	kernel.SymtabIndex = int(sect.Info & 0xff)
	kernel.KernelSize = sect.Size

	return nil
}

func (cp *cubinParser) parseSharedMemSection(sect *elfSection, kernelName []byte) error {
	if len(kernelName) == 0 {
		return nil
	}

	kernel := cp.getOrCreateKernel(kernelName)
	kernel.SharedMem = sect.Size

	return nil
}

var constantSectNameRegex = regexp.MustCompile(`\.nv\.constant\d\.(.*)`)

func (cp *cubinParser) parseConstantMemSection(sect *elfSection, _ []byte) error {
	// Constant memory sections are named .nv.constantX.Y where X is the constant memory index and Y is the name
	// so we have to do some custom parsing
	match := constantSectNameRegex.FindSubmatch(sect.nameBytes)
	if match == nil {
		// Not a constant memory section. We might be missing the kernel name, for example, which happens
		// on some binaries. In that case just ignore the section
		return nil
	}

	kernelName := match[1]
	kernel := cp.getOrCreateKernel(kernelName)
	kernel.ConstantMem += sect.Size

	return nil
}
