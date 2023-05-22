// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getSymbolNameByEntry extracts from the string data section the string in the given position.
// If the symbol name is shorter or longer than the given min and max (==len(preallocatedBuf)) then we return false.
// preAllocatedBuf is a pre allocated buffer with a constant length (== max length among all given symbols) to spare
// redundant allocations. We get it as a parameter and not putting it as a global, to be thread safe among concurrent
// and parallel calls.
func getSymbolNameByEntry(sectionReader io.ReaderAt, startPos, minLength int, preAllocatedBuf []byte) (string, bool) {
	_, err := sectionReader.ReadAt(preAllocatedBuf, int64(startPos))
	if err != nil {
		return "", false
	}
	for i := 0; i < len(preAllocatedBuf); i++ {
		if preAllocatedBuf[i] == 0 {
			if i < minLength {
				break
			}
			return string(preAllocatedBuf[0:i]), true
		}
	}

	return "", false
}

// getSymbolLengthBoundaries extracts the minimum and maximum lengths of the symbols.
func getSymbolLengthBoundaries(set common.StringSet) (int, int) {
	if len(set) == 0 {
		return 0, 0
	}
	maxSymbolName := 0
	minSymbolName := math.MaxInt
	for k := range set {
		if len(k) > maxSymbolName {
			maxSymbolName = len(k)
		}
		if len(k) < minSymbolName {
			minSymbolName = len(k)
		}
	}

	return minSymbolName, maxSymbolName
}

// readSymbolEntryInStringTable reads the first 4 bytes from the symbol section current location, which represents the
// symbol name entry in the string data section. Returns error in case of failure in reading the symbol entry.
func readSymbolEntryInStringTable(symbolSectionReader io.ReaderAt, byteOrder binary.ByteOrder, readLocation int64, allocatedBufferForRead []byte) (int, error) {
	if _, err := symbolSectionReader.ReadAt(allocatedBufferForRead, readLocation); err != nil {
		return 0, err
	}
	return int(byteOrder.Uint32(allocatedBufferForRead)), nil
}

// fillSymbol reads the symbol entry from the symbol section with the first 4 bytes of the name entry (which
// we read using readSymbolEntryInStringTable).
func fillSymbol(symbol *elf.Symbol, symbolSectionReader io.ReaderAt, byteOrder binary.ByteOrder, symbolName string, readLocation int64, allocatedBufferForRead []byte, is64Bit bool) error {
	if _, err := symbolSectionReader.ReadAt(allocatedBufferForRead, readLocation); err != nil {
		return err
	}
	symbol.Name = symbolName
	if is64Bit {
		infoAndOther := byteOrder.Uint16(allocatedBufferForRead[0:2])
		symbol.Info = uint8(infoAndOther >> 8)
		symbol.Other = uint8(infoAndOther)
		symbol.Section = elf.SectionIndex(byteOrder.Uint16(allocatedBufferForRead[2:4]))
		symbol.Value = byteOrder.Uint64(allocatedBufferForRead[4:12])
		symbol.Size = byteOrder.Uint64(allocatedBufferForRead[12:20])
	} else {
		infoAndOther := byteOrder.Uint16(allocatedBufferForRead[8:10])
		symbol.Info = uint8(infoAndOther >> 8)
		symbol.Other = uint8(infoAndOther)
		symbol.Section = elf.SectionIndex(byteOrder.Uint16(allocatedBufferForRead[10:12]))
		symbol.Value = uint64(byteOrder.Uint32(allocatedBufferForRead[0:4]))
		symbol.Size = uint64(byteOrder.Uint32(allocatedBufferForRead[4:8]))
	}

	return nil
}

// getSymbolsUnified extracts the given symbol list from the binary.
func getSymbolsUnified(f *elf.File, typ elf.SectionType, wantedSymbols common.StringSet, is64Bit bool) ([]elf.Symbol, error) {
	symbolSize := elf.Sym32Size
	if is64Bit {
		symbolSize = elf.Sym64Size
	}
	// Getting the relevant symbol section.
	symbolSection := f.SectionByType(typ)
	if symbolSection == nil {
		return nil, elf.ErrNoSymbols
	}

	// Checking the symbol section size is aligned to a multiplication of symbolSize.
	if symbolSection.Size%uint64(symbolSize) != 0 {
		return nil, fmt.Errorf("length of symbol section is not a multiple of %d", symbolSize)
	}

	// Checking the symbol section link is valid.
	if symbolSection.Link <= 0 || symbolSection.Link >= uint32(len(f.Sections)) {
		return nil, errors.New("section has invalid string table link")
	}

	// Allocating entries for all wanted symbols.
	symbols := make([]elf.Symbol, 0, len(wantedSymbols))

	// Extracting the min and max symbol length.
	minSymbolNameSize, maxSymbolNameSize := getSymbolLengthBoundaries(wantedSymbols)
	// Pre-allocating a buffer to read the symbol string into.
	// The size of the buffer is maxSymbolNameSize + 1, for null termination.
	symbolNameBuf := make([]byte, maxSymbolNameSize+1)
	// Pre allocating a buffer for reading the symbol entry in the string table.
	allocatedBufferForSymbolNameRead := make([]byte, 4)
	// Pre allocating a buffer for reading the rest of the symbol fields from the symbol section.
	allocatedBufferForSymbolRead := make([]byte, symbolSize-len(allocatedBufferForSymbolNameRead))

	// Iterating through the symbol table. We skip the first symbolSize bytes as they are zeros.
	for readLocation := int64(symbolSize); uint64(readLocation) < symbolSection.Size; readLocation += int64(symbolSize) {
		// Reading the symbol entry in the string table.
		stringEntry, err := readSymbolEntryInStringTable(symbolSection.ReaderAt, f.ByteOrder, readLocation, allocatedBufferForSymbolNameRead)
		if err != nil {
			log.Debugf("failed reading symbol entry %s", err)
			continue
		}

		// Trying to get string representation of symbol.
		// If the symbol name's length is not in the boundaries [minSymbolNameSize, maxSymbolNameSize+1] then we fail,
		// and continue to the next symbol.
		symbolName, ok := getSymbolNameByEntry(f.Sections[symbolSection.Link].ReaderAt, stringEntry, minSymbolNameSize, symbolNameBuf)
		if !ok {
			continue
		}

		// Checking the symbol is relevant for us.
		if _, ok := wantedSymbols[symbolName]; !ok {
			continue
		}

		// Complete the symbol reading.
		var symbol elf.Symbol
		if err := fillSymbol(&symbol, symbolSection.ReaderAt, f.ByteOrder, symbolName, readLocation+4, allocatedBufferForSymbolRead, is64Bit); err != nil {
			continue
		}
		symbols = append(symbols, symbol)

		// If no symbols left, stop running.
		if len(symbols) == len(wantedSymbols) {
			break
		}
	}

	return symbols, nil
}

func getSymbols(f *elf.File, typ elf.SectionType, wanted map[string]struct{}) ([]elf.Symbol, error) {
	switch f.Class {
	case elf.ELFCLASS64:
		return getSymbolsUnified(f, typ, wanted, true)

	case elf.ELFCLASS32:
		return getSymbolsUnified(f, typ, wanted, false)
	}

	return nil, errors.New("not implemented")
}

func GetAllSymbolsByName(elfFile *elf.File, symbolSet common.StringSet) (map[string]elf.Symbol, error) {
	regularSymbols, regularSymbolsErr := getSymbols(elfFile, elf.SHT_SYMTAB, symbolSet)
	if regularSymbolsErr != nil {
		log.Tracef("Failed getting regular symbols of elf file: %s", regularSymbolsErr)
	}

	var dynamicSymbols []elf.Symbol
	var dynamicSymbolsErr error
	if len(regularSymbols) != len(symbolSet) {
		dynamicSymbols, dynamicSymbolsErr = getSymbols(elfFile, elf.SHT_DYNSYM, symbolSet)
		if dynamicSymbolsErr != nil {
			log.Tracef("Failed getting dynamic symbols of elf file: %s", dynamicSymbolsErr)
		}
	}

	// Only if we failed getting both regular and dynamic symbols - then we abort.
	if regularSymbolsErr != nil && dynamicSymbolsErr != nil {
		return nil, fmt.Errorf("could not open symbol sections to resolve symbol offset: %v, %v", regularSymbolsErr, dynamicSymbolsErr)
	}

	symbolByName := make(map[string]elf.Symbol, len(regularSymbols)+len(dynamicSymbols))

	for _, regularSymbol := range regularSymbols {
		symbolByName[regularSymbol.Name] = regularSymbol
	}

	for _, dynamicSymbol := range dynamicSymbols {
		symbolByName[dynamicSymbol.Name] = dynamicSymbol
	}

	if len(symbolByName) != len(symbolSet) {
		missingSymbols := make([]string, 0, len(symbolSet)-len(symbolByName))
		for symbolName := range symbolSet {
			if _, ok := symbolByName[symbolName]; !ok {
				missingSymbols = append(missingSymbols, symbolName)
			}

		}
		return nil, fmt.Errorf("failed to find symbols %#v", missingSymbols)
	}

	return symbolByName, nil
}
