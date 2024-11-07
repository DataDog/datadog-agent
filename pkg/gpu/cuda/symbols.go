// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package cuda

import (
	"debug/elf"
	"fmt"
)

// Symbols holds all necessary data from a CUDA executable for
// getting necessary CUDA kernel data. That is, the symbol table
// which maps addresses to symbol names and the fatbin data with all
// the CUDA kernels available in the binary and their metadata.
type Symbols struct {
	SymbolTable map[uint64]string
	Fatbin      *Fatbin
}

// GetSymbols reads an ELF file from the given path and return the parsed CUDA data
func GetSymbols(path string) (*Symbols, error) {
	elfFile, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening ELF file %s: %w", path, err)
	}
	defer elfFile.Close()

	fatbin, err := ParseFatbinFromELFFile(elfFile)
	if err != nil {
		return nil, fmt.Errorf("error parsing fatbin on %s: %w", path, err)
	}

	data := &Symbols{
		SymbolTable: make(map[uint64]string),
		Fatbin:      fatbin,
	}

	syms, err := elfFile.Symbols()
	if err != nil {
		return nil, fmt.Errorf("error reading symbols from ELF file %s: %w", path, err)
	}

	for _, sym := range syms {
		data.SymbolTable[sym.Value] = sym.Name
	}

	return data, nil
}
