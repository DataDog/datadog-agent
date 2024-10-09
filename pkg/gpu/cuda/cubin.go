// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Generate String() methods for nvInfoAttr,nvInfoFormat enums so they can be printed in logs/error messages
//go:generate go run golang.org/x/tools/cmd/stringer@latest -output cubin_string.go -type=nvInfoAttr,nvInfoFormat -linecomment

package cuda

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"strings"
)

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

type sectionParserFunc func(*elf.Section, io.ReadSeeker, string) error

// cubinParser is a helper struct to parse the cubin ELF sections
type cubinParser struct {
	kernels            map[string]*CubinKernel
	sectPrefixToParser map[string]sectionParserFunc
}

func newCubinParser() *cubinParser {
	cp := &cubinParser{
		kernels:            make(map[string]*CubinKernel),
		sectPrefixToParser: make(map[string]sectionParserFunc),
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

const elfVersionOffset = 20

func (cp *cubinParser) parseCubinElf(data []byte) error {
	// Hacks to be able to parse the ELF: the ELF version is not supported by the Go ELF parser, so we need to
	// trick it into thinking it's the old version. Check for boundaries first
	if len(data) <= elfVersionOffset {
		return fmt.Errorf("invalid cubin data, too short")
	}
	data[elfVersionOffset] = 1

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

			var kernelName string
			if strings.HasPrefix(sect.Name, prefixWithDot) {
				kernelName = strings.TrimPrefix(sect.Name, prefixWithDot)
			}

			err = parser(sect, sect.Open(), kernelName)
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

func (cp *cubinParser) parseNvInfoSection(_ *elf.Section, buffer io.ReadSeeker, kernelName string) error {
	items := make(map[nvInfoAttr]nvInfoParsedItem)

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
		default:
			return fmt.Errorf("unsupported format %d", item.Format)
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

func (cp *cubinParser) parseTextSection(sect *elf.Section, _ io.ReadSeeker, kernelName string) error {
	if kernelName == "" {
		return nil
	}

	kernel := cp.getKernel(kernelName)
	kernel.SymtabIndex = int(sect.Info & 0xff)
	kernel.KernelSize = sect.Size

	return nil
}

func (cp *cubinParser) parseSharedMemSection(sect *elf.Section, _ io.ReadSeeker, kernelName string) error {
	if kernelName == "" {
		return nil
	}

	kernel := cp.getKernel(kernelName)
	kernel.SharedMem = sect.Size

	return nil
}

var constantSectNameRegex = regexp.MustCompile(`\.nv\.constant\d\.(.*)`)

func (cp *cubinParser) parseConstantMemSection(sect *elf.Section, _ io.ReadSeeker, _ string) error {
	// Constant memory sections are named .nv.constantX.Y where X is the constant memory index and Y is the name
	// so we have to do some custom parsing
	match := constantSectNameRegex.FindStringSubmatch(sect.Name)
	if match == nil {
		// Not a constant memory section. We might be missing the kernel name, for example, which happens
		// on some binaries. In that case just ignore the section
		return nil
	}

	kernelName := match[1]
	kernel := cp.getKernel(kernelName)
	kernel.ConstantMem += sect.Size

	return nil
}
