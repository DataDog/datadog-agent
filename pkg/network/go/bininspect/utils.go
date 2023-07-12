// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"debug/dwarf"
	"debug/elf"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/network/go/asmscan"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/go-delve/delve/pkg/goversion"
)

// GetArchitecture returns the `runtime.GOARCH`-compatible names of the architecture.
// Only returns a value for supported architectures.
func GetArchitecture(elfFile *elf.File) (GoArch, error) {
	switch elfFile.FileHeader.Machine {
	case elf.EM_X86_64:
		return GoArchX86_64, nil
	case elf.EM_AARCH64:
		return GoArchARM64, nil
	}

	return "", ErrUnsupportedArch
}

// HasDwarfInfo attempts to parse the DWARF data and look for any records.
// If it cannot be parsed or if there are no DWARF info records,
// then it assumes that the binary has been stripped.
func HasDwarfInfo(elfFile *elf.File) (*dwarf.Data, bool) {
	dwarfData, err := elfFile.DWARF()
	if err != nil {
		return nil, false
	}

	infoReader := dwarfData.Reader()
	if firstEntry, err := infoReader.Next(); err == nil && firstEntry != nil {
		return dwarfData, true
	}

	return nil, false
}

// FindGoVersion attempts to determine the Go version
// from the embedded string inserted in the binary by the linker.
// The implementation is available in src/cmd/go/internal/version/version.go:
// https://cs.opensource.google/go/go/+/refs/tags/go1.17.2:src/cmd/go/internal/version/version.go
// The main logic was pulled out to a sub-package, `binversion`
func FindGoVersion(elfFile *elf.File) (goversion.GoVersion, error) {
	version, _, err := binversion.ReadElfBuildInfo(elfFile)
	if err != nil {
		return goversion.GoVersion{}, fmt.Errorf("could not get Go toolchain version from ELF binary file: %w", err)
	}

	parsed, ok := goversion.Parse(version)
	if !ok {
		return goversion.GoVersion{}, fmt.Errorf("failed to parse Go toolchain version %q", version)
	}
	return parsed, nil
}

// FindABI returns the ABI for a given go version and architecture.
// We statically assume the ABI based on the Go version and architecture
func FindABI(version goversion.GoVersion, arch GoArch) (GoABI, error) {
	switch arch {
	case GoArchX86_64:
		if version.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 17}) {
			return GoABIRegister, nil
		}
		return GoABIStack, nil

	case GoArchARM64:
		if version.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 18}) {
			return GoABIRegister, nil
		}
		return GoABIStack, nil
	}

	return "", ErrUnsupportedArch
}

// FindReturnLocations returns the offsets of all the returns of the given func (sym) with the given offset.
// Note that this may not behave well with panics or defer statements.
// See the following links for more context:
// - https://github.com/go-delve/delve/pull/2704/files#diff-fb7b7a020e32bf8bf477c052ac2d2857e7e587478be6039aebc7135c658417b2R769
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/breakpoints.go#L86-L95
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/breakpoints.go#L374
func FindReturnLocations(elfFile *elf.File, sym elf.Symbol, functionOffset uint64) ([]uint64, error) {
	arch, err := GetArchitecture(elfFile)
	if err != nil {
		return nil, err
	}

	textSection := elfFile.Section(".text")
	if textSection == nil {
		return nil, fmt.Errorf("no %q section found in binary file", ".text")
	}

	switch arch {
	case GoArchX86_64:
		return asmscan.ScanFunction(textSection, sym, functionOffset, asmscan.FindX86_64ReturnInstructions)
	case GoArchARM64:
		return asmscan.ScanFunction(textSection, sym, functionOffset, asmscan.FindARM64ReturnInstructions)
	default:
		return nil, ErrUnsupportedArch
	}
}

// SymbolToOffset returns the offset of the given symbol name in the given elf file.
func SymbolToOffset(f *elf.File, symbol elf.Symbol) (uint32, error) {
	if f == nil {
		return 0, errors.New("got nil elf file")
	}

	var sectionsToSearchForSymbol []*elf.Section

	for i := range f.Sections {
		if f.Sections[i].Flags == elf.SHF_ALLOC+elf.SHF_EXECINSTR {
			sectionsToSearchForSymbol = append(sectionsToSearchForSymbol, f.Sections[i])
		}
	}

	if len(sectionsToSearchForSymbol) == 0 {
		return 0, fmt.Errorf("symbol %q not found in file - no sections to search", symbol)
	}

	var executableSection *elf.Section

	// Find what section the symbol is in by checking the executable section's
	// addr space.
	for m := range sectionsToSearchForSymbol {
		sectionStart := sectionsToSearchForSymbol[m].Addr
		sectionEnd := sectionStart + sectionsToSearchForSymbol[m].Size
		if symbol.Value >= sectionStart && symbol.Value < sectionEnd {
			executableSection = sectionsToSearchForSymbol[m]
			break
		}
	}

	if executableSection == nil {
		return 0, errors.New("could not find symbol in executable sections of binary")
	}

	return uint32(symbol.Value - executableSection.Addr + executableSection.Offset), nil

}
