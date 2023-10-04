// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"debug/elf"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/network/go/goid"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/go-delve/delve/pkg/goversion"
)

type newProcessBinaryInspector struct {
	elf       elfMetadata
	symbols   map[string]elf.Symbol
	goVersion goversion.GoVersion
}

func InspectNewProcessBinary(elfFile *elf.File, functions map[string]FunctionConfiguration, structs map[FieldIdentifier]StructLookupFunction) (*Result, error) {
	if elfFile == nil {
		return nil, errors.New("got nil elf file")
	}

	// Determine the architecture of the binary
	arch, err := GetArchitecture(elfFile)
	if err != nil {
		return nil, err
	}

	goVersion, err := FindGoVersion(elfFile)
	if err != nil {
		return nil, err
	}

	abi, err := FindABI(goVersion, arch)
	if err != nil {
		return nil, err
	}

	symbolsSet := make(common.StringSet, len(functions))
	for symbol := range functions {
		symbolsSet[symbol] = struct{}{}
	}
	// Try to load in the ELF symbols.
	// This might fail if the binary was stripped.
	symbols, err := GetAllSymbolsByName(elfFile, symbolsSet)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving symbols: %+v", err)
	}

	inspector := newProcessBinaryInspector{
		elf: elfMetadata{
			file: elfFile,
			arch: arch,
		},
		symbols:   symbols,
		goVersion: goVersion,
	}

	goroutineIDMetadata, err := inspector.getGoroutineIDMetadata(abi)
	if err != nil {
		return nil, err
	}

	functionsData, err := inspector.findFunctions(functions)
	if err != nil {
		return nil, err
	}

	structOffsets := make(map[FieldIdentifier]uint64, len(structs))
	for structID, lookupFunc := range structs {
		structOffset, err := lookupFunc(goVersion, string(arch))
		if err != nil {
			return nil, err
		}
		structOffsets[structID] = structOffset
	}

	return &Result{
		Arch:                arch,
		ABI:                 abi,
		GoVersion:           goVersion,
		Functions:           functionsData,
		StructOffsets:       structOffsets,
		GoroutineIDMetadata: goroutineIDMetadata,
	}, nil

}

func (i *newProcessBinaryInspector) findFunctions(functions map[string]FunctionConfiguration) (map[string]FunctionMetadata, error) {
	functionMetadata := make(map[string]FunctionMetadata, len(functions))

	for funcName, funcConfig := range functions {
		offset, err := SymbolToOffset(i.elf.file, i.symbols[funcName])
		if err != nil {
			return nil, fmt.Errorf("could not find location for function %q: %w", funcName, err)
		}

		var returnLocations []uint64
		if funcConfig.IncludeReturnLocations {
			symbol, ok := i.symbols[funcName]
			if !ok {
				return nil, fmt.Errorf("could not find function %q in symbols", funcName)
			}

			locations, err := FindReturnLocations(i.elf.file, symbol, uint64(offset))
			if err != nil {
				return nil, fmt.Errorf("could not find return locations for function %q: %w", funcName, err)
			}

			returnLocations = locations
		}

		parameters, err := funcConfig.ParamLookupFunction(i.goVersion, string(i.elf.arch))
		if err != nil {
			return nil, fmt.Errorf("failed finding parameters of function %q: %w", funcName, err)
		}

		functionMetadata[funcName] = FunctionMetadata{
			EntryLocation:   uint64(offset),
			ReturnLocations: returnLocations,
			Parameters:      parameters,
		}
	}

	return functionMetadata, nil
}

// getRuntimeGAddrTLSOffset determines what the offset
// of the `runtime.g` value is in thread-local-storage.
//
// This implementation is based on github.com/go-delve/delve/pkg/proc.(*BinaryInfo).setGStructOffsetElf:
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/bininfo.go#L1413
// which is licensed under MIT.
func (i *newProcessBinaryInspector) getRuntimeGAddrTLSOffset() (uint64, error) {
	// This is a bit arcane. Essentially:
	// - If the program is pure Go, it can do whatever it wants, and puts the G
	//   pointer at %fs-8 on 64 bit.
	// - %Gs is the index of private storage in GDT on 32 bit, and puts the G
	//   pointer at -4(tls).
	// - Otherwise, Go asks the external linker to place the G pointer by
	//   emitting runtime.tlsg, a TLS symbol, which is relocated to the chosen
	//   offset in libc's TLS block.
	// - On ARM64 (but really, any architecture other than i386 and 86x64) the
	//   offset is calculated using runtime.tls_g and the formula is different.

	var tls *elf.Prog
	for _, prog := range i.elf.file.Progs {
		if prog.Type == elf.PT_TLS {
			tls = prog
			break
		}
	}

	switch i.elf.arch {
	case GoArchX86_64:
		tlsg, ok := i.symbols["runtime.tlsg"]
		if !ok || tls == nil {
			return ^uint64(i.elf.arch.PointerSize()) + 1, nil //-ptrSize
		}

		// According to https://reviews.llvm.org/D61824, linkers must pad the actual
		// size of the TLS segment to ensure that (tlsoffset%align) == (vaddr%align).
		// This formula, copied from the lld code, matches that.
		// https://github.com/llvm-mirror/lld/blob/9aef969544981d76bea8e4d1961d3a6980980ef9/ELF/InputSection.cpp#L643
		memsz := tls.Memsz + (-tls.Vaddr-tls.Memsz)&(tls.Align-1)

		// The TLS register points to the end of the TLS block, which is
		// tls.Memsz long. runtime.tlsg is an offset from the beginning of that block.
		return ^(memsz) + 1 + tlsg.Value, nil // -tls.Memsz + tlsg.Value

	case GoArchARM64:
		tlsg, ok := i.symbols["runtime"]
		if !ok || tls == nil {
			return 2 * uint64(i.elf.arch.PointerSize()), nil
		}

		return tlsg.Value + uint64(i.elf.arch.PointerSize()*2) + ((tls.Vaddr - uint64(i.elf.arch.PointerSize()*2)) & (tls.Align - 1)), nil

	default:
		return 0, errors.New("binary is for unsupported architecture")
	}
}

// getGoroutineIDMetadata collects enough metadata about the binary
// to be able to reliably determine the goroutine ID from the context of an eBPF uprobe.
// This is accomplished by finding the offset of the `goid` field in the `runtime.g` struct,
// which is the goroutine context struct.
//
// A pointer to this struct is always stored in thread-local-strorage (TLS),
// but it might also be in a dedicated register (which is faster to access),
// depending on the ABI and architecture:
// 1. If it has a dedicated register, this function gives the register number
// 2. Otherwise, this function finds the offset in TLS that the pointer exists at.
//
// See:
// - https://go.googlesource.com/proposal/+/master/design/40724-register-calling.md#go_s-current-stack_based-abi
// - https://go.googlesource.com/go/+/refs/heads/dev.regabi/src/cmd/compile/internal-abi.md#amd64-architecture
// - https://github.com/golang/go/blob/61011de1af0bc6ab286c4722632719d3da2cf746/src/runtime/runtime2.go#L403
// - https://github.com/golang/go/blob/61011de1af0bc6ab286c4722632719d3da2cf746/src/runtime/runtime2.go#L436
func (i *newProcessBinaryInspector) getGoroutineIDMetadata(abi GoABI) (GoroutineIDMetadata, error) {
	goroutineIDOffset, err := goid.GetGoroutineIDOffset(i.goVersion, string(i.elf.arch))
	if err != nil {
		return GoroutineIDMetadata{}, fmt.Errorf("could not find goroutine ID offset in goroutine context struct: %w", err)
	}

	// On x86_64 and the register ABI, the runtime.g pointer (current goroutine context struct) is stored in a register (r14):
	// https://go.googlesource.com/go/+/refs/heads/dev.regabi/src/cmd/compile/internal-abi.md#amd64-architecture
	// Additionally, on all architectures other than x86_64 and x86 (in all Go versions),
	// the runtime.g pointer is stored on a register.
	// On x86_64 pre-Go 1.17 and on x86 (in all Go versions),
	// the runtime.g pointer is stored in the thread's thread-local-storage.
	runtimeGInRegister := true
	if i.elf.arch == GoArchX86_64 {
		runtimeGInRegister = abi == GoABIRegister
	}

	var runtimeGRegister int
	var runtimeGTLSAddrOffset uint64
	if runtimeGInRegister {
		switch i.elf.arch {
		case GoArchX86_64:
			// https://go.googlesource.com/go/+/refs/heads/dev.regabi/src/cmd/compile/internal-abi.md#amd64-architecture
			runtimeGRegister = 14
		case GoArchARM64:
			// https://go.googlesource.com/go/+/refs/heads/master/src/cmd/compile/abi-internal.md#arm64-architecture
			runtimeGRegister = 28
		}
	} else {
		offset, err := i.getRuntimeGAddrTLSOffset()
		if err != nil {
			return GoroutineIDMetadata{}, fmt.Errorf("could not get offset of runtime.g offset in TLS: %w", err)
		}

		runtimeGTLSAddrOffset = offset
	}

	return GoroutineIDMetadata{
		GoroutineIDOffset:     goroutineIDOffset,
		RuntimeGInRegister:    runtimeGInRegister,
		RuntimeGRegister:      runtimeGRegister,
		RuntimeGTLSAddrOffset: runtimeGTLSAddrOffset,
	}, nil
}
