// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package uprobes

import (
	"debug/elf"
	"fmt"
	"runtime"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/network/go/bininspect"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/common"
)

// BinaryInspector implementors are responsible for extracting the metadata required to attach from a binary.
type BinaryInspector interface {
	// Inspect returns the metadata required to attach to a binary. The first
	// return is a map of symbol names to their corresponding metadata, the
	// second return is a boolean indicating whether this binary is compatible
	// and can be attached or not. It is encouraged to return early if the
	// binary is not compatible, to avoid unnecessary work. In the future, the
	// first and second return values should be merged into a single struct, but
	// for now this allows us to keep the API compatible with the existing
	// implementation.
	Inspect(fpath utils.FilePath, requests []SymbolRequest) (map[string]bininspect.FunctionMetadata, bool, error)

	// Cleanup is called when a certain file path is not needed anymore, the implementation can clean up
	// any resources associated with the file path.
	Cleanup(fpath utils.FilePath)
}

// SymbolRequest represents a request for symbols and associated data from a binary
type SymbolRequest struct {
	// Name of the symbol to request
	Name string
	// BestEffort indicates that the symbol is not mandatory, and the inspector should not return an error if it is not found
	BestEffort bool
	// IncludeReturnLocations indicates that the inspector should also include the return locations of the function, for manual
	// attachment into those return points instead of using uretprobes.
	IncludeReturnLocations bool
}

// NativeBinaryInspector is a BinaryInspector that uses the ELF format to extract the metadata directly from native functions.
type NativeBinaryInspector struct {
}

// Ensure NativeBinaryInspector implements BinaryInspector
var _ BinaryInspector = &NativeBinaryInspector{}

// Inspect extracts the metadata required to attach to a binary from the ELF file at the given path.
func (p *NativeBinaryInspector) Inspect(fpath utils.FilePath, requests []SymbolRequest) (map[string]bininspect.FunctionMetadata, bool, error) {
	path := fpath.HostPath
	elfFile, err := elf.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer elfFile.Close()

	// This only allows amd64 and arm64 and not the 32-bit variants, but that
	// is fine since we don't monitor 32-bit applications at all in the shared
	// library watcher since compat syscalls aren't supported by the syscall
	// trace points. We do actually monitor 32-bit applications for istio and
	// nodejs monitoring, but our uprobe hooks only properly support 64-bit
	// applications, so there's no harm in rejecting 32-bit applications here.
	arch, err := bininspect.GetArchitecture(elfFile)
	if err != nil {
		return nil, false, fmt.Errorf("cannot get architecture of %s: %w", path, err)
	}

	// Ignore foreign architectures.  This can happen when running stuff under
	// qemu-user, for example, and installing a uprobe will lead to segfaults
	// since the foreign instructions will be patched with the native break
	// instruction.
	if string(arch) != runtime.GOARCH {
		return nil, false, nil
	}

	mandatorySymbols := make(common.StringSet, len(requests))
	bestEffortSymbols := make(common.StringSet, len(requests))

	for _, req := range requests {
		if req.BestEffort {
			bestEffortSymbols.Add(req.Name)
		} else {
			mandatorySymbols.Add(req.Name)
		}

		if req.IncludeReturnLocations {
			return nil, false, errors.New("return locations are not supported by the native binary inspector")
		}
	}

	symbolMap, err := bininspect.GetAllSymbolsByName(elfFile, mandatorySymbols)
	if err != nil {
		return nil, false, err
	}
	/* Best effort to resolve symbols, so we don't care about the error */
	symbolMapBestEffort, _ := bininspect.GetAllSymbolsByName(elfFile, bestEffortSymbols)

	funcMap := make(map[string]bininspect.FunctionMetadata, len(symbolMap)+len(symbolMapBestEffort))
	for _, symMap := range []map[string]elf.Symbol{symbolMap, symbolMapBestEffort} {
		for symbol, sym := range symMap {
			m, err := p.symbolToFuncMetadata(elfFile, sym)
			if err != nil {
				return nil, false, fmt.Errorf("failed to convert symbol %v to function metadata: %w", sym, err)
			}
			funcMap[symbol] = *m
		}
	}

	return funcMap, true, nil
}

func (*NativeBinaryInspector) symbolToFuncMetadata(elfFile *elf.File, sym elf.Symbol) (*bininspect.FunctionMetadata, error) {
	manager.SanitizeUprobeAddresses(elfFile, []elf.Symbol{sym})
	offset, err := bininspect.SymbolToOffset(elfFile, sym)
	if err != nil {
		return nil, err
	}

	return &bininspect.FunctionMetadata{EntryLocation: uint64(offset)}, nil
}

// Cleanup is a no-op for the native inspector
func (*NativeBinaryInspector) Cleanup(_ utils.FilePath) {
	// Nothing to do here for the native inspector
}
