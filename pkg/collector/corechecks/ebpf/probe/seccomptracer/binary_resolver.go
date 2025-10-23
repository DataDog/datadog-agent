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
func resolveAddress(info *binaryInfo, address uint64) string {
	basename := filepath.Base(info.pathname)

	// Try DWARF first if available
	if info.dwarfData != nil {
		if symbol := resolveDWARF(info, address); symbol != "" {
			return symbol
		}
	}

	// Fall back to symbol table
	if len(info.symbols) > 0 {
		if symbol := resolveSymbol(info, address); symbol != "" {
			return symbol
		}
	}

	// Final fallback: raw address
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

	// Format the result with inline information if available
	basename := filepath.Base(lineEntry.File.Name)

	if inlineInfo != "" {
		// Inlined function: show both inlined and parent function
		return fmt.Sprintf("%s@%s (%s:%d)", funcName, inlineInfo, basename, lineEntry.Line)
	}

	// Regular function with line info
	return fmt.Sprintf("%s (%s:%d)", funcName, basename, lineEntry.Line)
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

	// Verify the address falls within this symbol's range
	// We don't have size info, so we just check it's not unreasonably far
	offset := address - symbol.Value
	if offset > 0x100000 { // 1MB seems like a reasonable max function size
		return fmt.Sprintf("%s+0x%x", basename, address)
	}

	// Clean up the symbol name (demangle C++ names minimally)
	symbolName := symbol.Name
	if symbolName == "" {
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
