// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package seccomptracer provides symbolication for stack traces
package seccomptracer

import (
	"debug/dwarf"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// resolveAddress resolves an address to symbolic information using DWARF, symbol table, or raw address fallback
func resolveAddress(info *binaryInfo, address uint64, mode SymbolicationMode) string {
	basename := filepath.Base(info.pathname)

	// Convert file offset to virtual address for ET_EXEC binaries
	// For ET_EXEC files, symbols have virtual addresses (e.g., 0x400748)
	// but we receive file offsets (e.g., 0x754), so we need to add the base
	virtualAddr := address + info.baseAddr

	// Try DWARF first if available
	if mode&SymbolicationModeDWARF != 0 && info.dwarfData != nil {
		if symbol := resolveDWARF(info, virtualAddr); symbol != "" {
			return symbol
		}
	}

	// Fall back to symbol table
	if mode&SymbolicationModeSymTable != 0 && len(info.symbols) > 0 {
		if symbol := resolveSymbol(info, virtualAddr); symbol != "" {
			return symbol
		}
	}

	// Final fallback: raw address (use file offset)
	return fmt.Sprintf("%s+0x%x", basename, address)
}

// resolveDWARF attempts to resolve an address using DWARF debug information
// Returns empty string if resolution fails
func resolveDWARF(info *binaryInfo, address uint64) string {
	// Get line information for this address
	lineEntry, err := findLineEntry(info.dwarfData, address)
	if err != nil {
		log.Tracef("Failed to find line entry for address 0x%x: %v", address, err)
		return ""
	}

	if lineEntry == nil {
		return ""
	}

	// Find the function containing this address
	funcName, inlineInfo, err := findFunction(info.dwarfData, address)
	if err != nil {
		log.Tracef("Failed to find function for address 0x%x: %v", address, err)
		return ""
	}

	if funcName == "" {
		// Have line info but no function name
		if lineEntry.File != nil {
			basename := filepath.Base(lineEntry.File.Name)
			return fmt.Sprintf("(unknown) (%s:%d)", basename, lineEntry.Line)
		}
		return ""
	}

	// Format the result
	// Get the binary basename for consistent formatting
	binaryBasename := filepath.Base(info.pathname)

	if inlineInfo != "" {
		// Inlined function: show both inlined and parent function
		// Format: binary!funcName()@inlineInfo
		return fmt.Sprintf("%s!%s()@%s", binaryBasename, funcName, inlineInfo)
	}

	// Regular function with DWARF info
	// Format: binary!funcName()
	return fmt.Sprintf("%s!%s()", binaryBasename, funcName)
}

// findLineEntry finds the line table entry for the given address
func findLineEntry(dwarfData *dwarf.Data, address uint64) (*dwarf.LineEntry, error) {
	reader := dwarfData.Reader()
	if reader == nil {
		return nil, fmt.Errorf("failed to get DWARF reader")
	}

	// Iterate through compilation units
	for {
		entry, err := reader.Next()
		if err != nil {
			return nil, fmt.Errorf("error reading DWARF entries: %w", err)
		}
		if entry == nil {
			break
		}

		// Look for compilation units
		if entry.Tag != dwarf.TagCompileUnit {
			continue
		}

		// Get line reader for this compilation unit
		lr, err := dwarfData.LineReader(entry)
		if err != nil {
			continue
		}

		// Search for the address in this compilation unit's line table
		var bestEntry *dwarf.LineEntry
		for {
			var le dwarf.LineEntry
			err := lr.Next(&le)
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}

			// Look for the closest line entry <= address
			if le.Address <= address {
				if bestEntry == nil || le.Address > bestEntry.Address {
					leCopy := le
					bestEntry = &leCopy
				}
			} else if bestEntry != nil {
				// We've passed the address, found our best match
				break
			}
		}

		if bestEntry != nil && bestEntry.File != nil {
			return bestEntry, nil
		}
	}

	return nil, nil
}

// findFunction finds the function name and inline information for the given address
func findFunction(dwarfData *dwarf.Data, address uint64) (funcName string, inlineInfo string, err error) {
	reader := dwarfData.Reader()
	if reader == nil {
		return "", "", fmt.Errorf("failed to get DWARF reader")
	}

	var currentFunc string
	var inlinedFunc string

	for {
		entry, err := reader.Next()
		if err != nil {
			return "", "", fmt.Errorf("error reading DWARF entries: %w", err)
		}
		if entry == nil {
			break
		}

		// Check for subprogram (function) entries
		if entry.Tag == dwarf.TagSubprogram || entry.Tag == dwarf.TagInlinedSubroutine {
			// Get address range
			lowPC, ok := entry.Val(dwarf.AttrLowpc).(uint64)
			if !ok {
				continue
			}

			// Check high PC (can be either absolute address or offset from low PC)
			highPCAttr := entry.AttrField(dwarf.AttrHighpc)
			if highPCAttr == nil {
				continue
			}

			var highPC uint64
			switch v := highPCAttr.Val.(type) {
			case uint64:
				highPC = v
			case int64:
				// High PC is an offset from low PC
				highPC = lowPC + uint64(v)
			default:
				continue
			}

			// Check if address is in range
			if address >= lowPC && address < highPC {
				name, _ := entry.Val(dwarf.AttrName).(string)
				if name == "" {
					continue
				}

				if entry.Tag == dwarf.TagInlinedSubroutine {
					inlinedFunc = name
					// Keep looking for the parent function
				} else {
					currentFunc = name
					if inlinedFunc != "" {
						// Found both inlined and parent function
						return inlinedFunc, currentFunc, nil
					}
					// Found the containing function
					return currentFunc, "", nil
				}
			}
		}
	}

	if currentFunc != "" {
		return currentFunc, "", nil
	}
	if inlinedFunc != "" {
		return inlinedFunc, "", nil
	}

	return "", "", nil
}

// resolveSymbol attempts to resolve an address using the symbol table
// Returns empty string if resolution fails
func resolveSymbol(info *binaryInfo, address uint64) string {
	if len(info.symbols) == 0 {
		return ""
	}

	basename := filepath.Base(info.pathname)

	// Binary search to find the symbol containing this address
	idx := sort.Search(len(info.symbols), func(i int) bool {
		return info.symbols[i].Value > address
	})

	// idx now points to the first symbol with Value > address
	// The symbol we want is at idx-1 (if it exists)
	if idx == 0 {
		// Address is before the first symbol
		return fmt.Sprintf("%s+0x%x", basename, address)
	}

	symbol := info.symbols[idx-1]

	offset := address - symbol.Value
	symbolName := symbol.Name
	if symbolName == "" {
		return fmt.Sprintf("%s+0x%x", basename, address)
	}

	// If offset is too large (>1MB), likely incorrect - fallback to binary+offset
	const maxReasonableOffset = 0x100000 // 1MB
	if offset > maxReasonableOffset {
		return fmt.Sprintf("%s+0x%x", basename, address)
	}

	// Remove version suffixes like @@GLIBC_2.2.5
	if idx := strings.Index(symbolName, "@@"); idx >= 0 {
		symbolName = symbolName[:idx]
	}
	if idx := strings.Index(symbolName, "@"); idx >= 0 {
		symbolName = symbolName[:idx]
	}

	if offset > 0 {
		return fmt.Sprintf("%s!%s+0x%x", basename, symbolName, offset)
	}
	return fmt.Sprintf("%s!%s", basename, symbolName)
}
