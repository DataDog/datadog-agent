// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// makeSymbolsLsCommand returns the "usm symbols ls" cobra command.
func makeSymbolsLsCommand(_ *command.GlobalParams) *cobra.Command {
	var dynamic bool

	cmd := &cobra.Command{
		Use:   "ls <binary-file>",
		Short: "List symbols from binary files",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runSymbolsLs(args[0], dynamic)
		},
	}

	cmd.Flags().BoolVar(&dynamic, "dynamic", false,
		"Display dynamic symbols instead of static symbols")

	return cmd
}

type indexedSymbol struct {
	symbol safeelf.Symbol
	index  int
}

// runSymbolsLs is the main implementation of the symbols ls command.
func runSymbolsLs(filePath string, dynamic bool) error {

	// Open the ELF file
	elfFile, err := safeelf.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open ELF file %s: %w", filePath, err)
	}
	defer elfFile.Close()

	// Get symbols based on mode
	var symbols []safeelf.Symbol
	if dynamic {
		symbols, err = elfFile.DynamicSymbols()
		if err != nil {
			if errors.Is(err, safeelf.ErrNoSymbols) {
				fmt.Fprintf(os.Stderr, "%s: no dynamic symbols\n", filePath)
				return nil
			}
			return fmt.Errorf("failed to read dynamic symbols: %w", err)
		}
	} else {
		symbols, err = elfFile.Symbols()
		if err != nil {
			if errors.Is(err, safeelf.ErrNoSymbols) {
				fmt.Fprintf(os.Stderr, "%s: no symbols\n", filePath)
				return nil
			}
			return fmt.Errorf("failed to read symbols: %w", err)
		}
	}

	// Get version information if displaying dynamic symbols (before sorting)
	var versionMap map[int]string
	if dynamic {
		versionMap = getSymbolVersions(elfFile)
	}

	// Create indexed symbols to preserve original index after sorting
	indexedSymbols := make([]indexedSymbol, len(symbols))
	for i, sym := range symbols {
		indexedSymbols[i] = indexedSymbol{symbol: sym, index: i}
	}

	// Sort symbols by value (address)
	sort.Slice(indexedSymbols, func(i, j int) bool {
		return indexedSymbols[i].symbol.Value < indexedSymbols[j].symbol.Value
	})

	// Display symbols in nm format
	for _, idxSym := range indexedSymbols {
		sym := idxSym.symbol

		// Filter out FILE symbols (nm behavior - use -a to show them)
		if safeelf.ST_TYPE(sym.Info) == safeelf.STT_FILE {
			continue
		}

		symbolType := getSymbolType(&sym, elfFile)

		// Get symbol name with version if available
		// Version info is shown for undefined (imported) symbols
		symbolName := sym.Name
		if dynamic && versionMap != nil {
			// Check if symbol is undefined (section index 0) or check symbolType == 'U'
			isUndefined := (sym.Section == safeelf.SHN_UNDEF) || (symbolType == 'U')
			if isUndefined {
				// Symbol is undefined (imported), check for version
				if version, ok := versionMap[idxSym.index]; ok && version != "" {
					symbolName = symbolName + version
				}
			}
		}

		// Format: address type name
		// Only show blank address for undefined symbols (U or w)
		if sym.Value == 0 && (symbolType == 'U' || symbolType == 'w' || symbolType == 'v') {
			fmt.Printf("%16s %c %s\n", "", symbolType, symbolName)
		} else {
			fmt.Printf("%016x %c %s\n", sym.Value, symbolType, symbolName)
		}
	}

	return nil
}

const (
	sizeOfUint16 = 2
)

// getSymbolVersions extracts version information for dynamic symbols.
// Returns a map from symbol index to version string (e.g., "@GLIBC_2.17").
func getSymbolVersions(elfFile *safeelf.File) map[int]string {
	versionMap := make(map[int]string)

	// Find the .gnu.version section (contains version indices)
	var versionSection *safeelf.Section
	var verneedSection *safeelf.Section
	var verdefSection *safeelf.Section
	var dynstrSection *safeelf.Section

	for _, section := range elfFile.Sections {
		switch section.Name {
		case ".gnu.version":
			versionSection = section
		case ".gnu.version_r":
			verneedSection = section
		case ".gnu.version_d":
			verdefSection = section
		case ".dynstr":
			dynstrSection = section
		}
	}

	if versionSection == nil || dynstrSection == nil {
		return versionMap
	}

	// Read version indices
	versionData, err := versionSection.Data()
	if err != nil {
		return versionMap
	}

	// Read dynamic string table
	dynstrData, err := dynstrSection.Data()
	if err != nil {
		return versionMap
	}

	// Parse version entries to build version index -> version string map
	versionStrings := make(map[uint16]string)

	// Parse verneed (version requirements - imported symbols)
	if verneedSection != nil {
		verneedData, err := verneedSection.Data()
		if err == nil {
			parseVerneed(verneedData, dynstrData, versionStrings)
		}
	}

	// Parse verdef (version definitions - exported symbols)
	if verdefSection != nil {
		verdefData, err := verdefSection.Data()
		if err == nil {
			parseVerdef(verdefData, dynstrData, versionStrings)
		}
	}

	// Map each symbol to its version
	// Note: DynamicSymbols() skips the null symbol at index 0, but .gnu.version includes it
	// So .gnu.version[0] is for null, .gnu.version[1] is for DynamicSymbols()[0], etc.
	for i := 0; i < len(versionData)/sizeOfUint16; i++ {
		// Each entry is 2 bytes (uint16)
		versionIdx := uint16(versionData[i*sizeOfUint16]) | (uint16(versionData[i*sizeOfUint16+1]) << 8)

		// Version index 0 and 1 are special (local and global)
		if versionIdx > 1 {
			// Clear the hidden bit (bit 15)
			versionIdx &= 0x7fff
			if verStr, ok := versionStrings[versionIdx]; ok {
				// i=0 is null symbol, i=1 is DynamicSymbols()[0], etc.
				if i > 0 {
					versionMap[i-1] = verStr
				}
			}
		}
	}

	return versionMap
}

// parseVerneed parses the .gnu.version_r section to extract version strings.
func parseVerneed(verneedData, dynstrData []byte, versionStrings map[uint16]string) {
	const (
		cntOffset  = 2
		fileOffset = 4
		auxOffset  = 8
		nextOffset = 12
		readSize   = 16

		otherOffset = 6
		nameOffset  = 8
		nextAuxSize = 12
	)
	offset := 0
	for offset < len(verneedData) {
		if offset+readSize > len(verneedData) {
			break
		}

		// Read Verneed structure
		cnt := binary.LittleEndian.Uint16(verneedData[offset+cntOffset:])
		fileOffsetVal := binary.LittleEndian.Uint32(verneedData[offset+fileOffset:])
		aux := binary.LittleEndian.Uint32(verneedData[offset+auxOffset:])
		next := binary.LittleEndian.Uint32(verneedData[offset+nextOffset:])

		// Read the file name (library name) - not used in standard nm format
		_ = readString(dynstrData, int(fileOffsetVal))

		// Parse auxiliary version entries
		auxOffsetRead := offset + int(aux)
		for i := uint16(0); i < cnt; i++ {
			if auxOffsetRead+readSize > len(verneedData) {
				break
			}

			// Read Vernaux structure
			other := binary.LittleEndian.Uint16(verneedData[auxOffsetRead+otherOffset:])
			nameOffsetVal := binary.LittleEndian.Uint32(verneedData[auxOffsetRead+nameOffset:])
			nextAux := binary.LittleEndian.Uint32(verneedData[auxOffsetRead+nextAuxSize:])

			// Read version name
			versionName := readString(dynstrData, int(nameOffsetVal))

			// Store the version string with @ prefix (standard nm format)
			versionStrings[other] = "@" + versionName

			if nextAux == 0 {
				break
			}
			auxOffsetRead += int(nextAux)
		}

		if next == 0 {
			break
		}
		offset += int(next)
	}
}

// parseVerdef parses the .gnu.version_d section to extract version definitions.
func parseVerdef(verdefData, dynstrData []byte, versionStrings map[uint16]string) {
	const (
		ndxOffset   = 4
		cntOffset   = 6
		auxOffset   = 12
		nextOffset  = 16
		readSize    = 20
		auxReadSize = 8
	)
	offset := 0
	for offset < len(verdefData) {
		if offset+readSize > len(verdefData) {
			break
		}

		// Read Verdef structure
		ndx := binary.LittleEndian.Uint16(verdefData[offset+ndxOffset:])
		cnt := binary.LittleEndian.Uint16(verdefData[offset+cntOffset:])
		aux := binary.LittleEndian.Uint32(verdefData[offset+auxOffset:])
		next := binary.LittleEndian.Uint32(verdefData[offset+nextOffset:])

		// Parse auxiliary version entries (verdaux)
		// The first aux entry contains the actual version name
		if cnt > 0 {
			auxOffsetRead := offset + int(aux)
			if auxOffsetRead+auxReadSize <= len(verdefData) {
				// Read Verdaux structure
				nameOffset := binary.LittleEndian.Uint32(verdefData[auxOffsetRead:])

				// Read version name
				versionName := readString(dynstrData, int(nameOffset))

				// Store the version string with @@ prefix for base versions
				// (nm uses @@ for default versions and @ for non-default)
				versionStrings[ndx] = "@@" + versionName
			}
		}

		if next == 0 {
			break
		}
		offset += int(next)
	}
}

// readString reads a null-terminated string from a byte slice at the given offset.
func readString(data []byte, offset int) string {
	if offset >= len(data) {
		return ""
	}
	end := offset
	for end < len(data) && data[end] != 0 {
		end++
	}
	return string(data[offset:end])
}

// getSymbolType returns the nm-style symbol type character.
// This mimics the behavior of the GNU nm utility.
func getSymbolType(sym *safeelf.Symbol, elfFile *safeelf.File) rune {
	bind := safeelf.ST_BIND(sym.Info)
	typ := safeelf.ST_TYPE(sym.Info)

	// Undefined symbols
	if sym.Section == safeelf.SHN_UNDEF {
		if bind == safeelf.STB_WEAK {
			return 'w' // Weak undefined
		}
		return 'U'
	}

	// Common symbols (uninitialized data)
	if sym.Section == safeelf.SHN_COMMON {
		if bind == safeelf.STB_GLOBAL {
			return 'C'
		}
		return 'c'
	}

	// Absolute symbols
	if sym.Section == safeelf.SHN_ABS {
		if bind == safeelf.STB_GLOBAL {
			return 'A'
		}
		return 'a'
	}

	// Get the section
	if int(sym.Section) >= len(elfFile.Sections) {
		return '?'
	}
	section := elfFile.Sections[sym.Section]

	// Determine type based on section flags
	var typeChar rune

	if section.Flags&safeelf.SHF_EXECINSTR != 0 {
		// Code/text section
		typeChar = 't'
	} else if section.Flags&safeelf.SHF_ALLOC != 0 {
		if section.Flags&safeelf.SHF_WRITE != 0 {
			// Writable data section
			if section.Type == safeelf.SHT_NOBITS {
				// BSS (uninitialized data)
				typeChar = 'b'
			} else {
				// Initialized data
				typeChar = 'd'
			}
		} else {
			// Read-only data
			typeChar = 'r'
		}
	} else {
		// Debug or other sections
		if typ == safeelf.STT_FILE {
			return 'f'
		}
		typeChar = 'n'
	}

	// Special case for weak symbols
	if bind == safeelf.STB_WEAK {
		if typ == safeelf.STT_OBJECT {
			return 'V' // Weak object
		}
		return 'W' // Weak symbol
	}

	return typeChar
}
