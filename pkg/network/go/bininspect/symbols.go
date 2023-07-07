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

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getSymbolNameByEntry extracts from the string data section the string in the given position.
// If the symbol name is shorter or longer than the given min and max (==len(preallocatedBuf)) then we return false.
// preAllocatedBuf is a pre allocated buffer with a constant length (== max length among all given symbols) to spare
// redundant allocations. We get it as a parameter and not putting it as a global, to be thread safe among concurrent
// and parallel calls.
func getSymbolNameByEntry(sectionReader io.ReaderAt, startPos, minLength int, preAllocatedBuf []byte) int {
	_, err := sectionReader.ReadAt(preAllocatedBuf, int64(startPos))
	if err != nil {
		return -1
	}

	foundNull := false
	nullIndex := minLength
	for ; nullIndex < len(preAllocatedBuf); nullIndex++ {
		if preAllocatedBuf[nullIndex] == 0 {
			foundNull = true
			break
		}
	}
	if !foundNull {
		return -1
	}

	for i := 1; i <= nullIndex; i++ {
		if preAllocatedBuf[i] == 0 {
			return i
		}
	}

	return -1
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

// fillSymbol reads the symbol entry from the symbol section with the first 4 bytes of the name entry (which
// we read using readSymbolEntryInStringTable).
func fillSymbol(symbol *elf.Symbol, byteOrder binary.ByteOrder, symbolName string, allocatedBufferForRead []byte, is64Bit bool) {
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

	const symbolChunkSize = int64(100)

	chunkSize := symbolChunkSize * int64(symbolSize)

	symbolsCache := make([]byte, chunkSize)
	var symbol elf.Symbol

	var locationInCache int64

	// Iterating through the symbol table. We skip the first symbolSize bytes as they are zeros.
	for readLocation := int64(symbolSize); uint64(readLocation) < symbolSection.Size; readLocation += int64(symbolSize) {
		// Should read again
		if (readLocation-int64(symbolSize))/int64(symbolSize)%symbolChunkSize == 0 {
			_, err := symbolSection.ReaderAt.ReadAt(symbolsCache, readLocation)
			if err != nil && err != io.EOF {
				log.Debugf("failed reading symbol entry %s", err)
				return nil, err
			}
			locationInCache = 0
		} else {
			locationInCache += int64(symbolSize)
		}

		stringEntry := int(f.ByteOrder.Uint32(symbolsCache[locationInCache : locationInCache+4]))
		if stringEntry == 0 {
			// symbol without a name.
			continue
		}

		// Trying to get string representation of symbol.
		// If the symbol name's length is not in the boundaries [minSymbolNameSize, maxSymbolNameSize+1] then we fail,
		// and continue to the next symbol.
		symbolNameSize := getSymbolNameByEntry(f.Sections[symbolSection.Link].ReaderAt, stringEntry, minSymbolNameSize, symbolNameBuf)

		if symbolNameSize <= 0 {
			continue
		}

		// Checking the symbol is relevant for us.
		if _, ok := wantedSymbols[string(symbolNameBuf[:symbolNameSize])]; !ok {
			continue
		}

		// Complete the symbol reading.
		fillSymbol(&symbol, f.ByteOrder, string(symbolNameBuf[:symbolNameSize]), symbolsCache[locationInCache+4:locationInCache+int64(symbolSize)-4], is64Bit)
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
	if regularSymbolsErr != nil && log.ShouldLog(seelog.TraceLvl) {
		log.Tracef("Failed getting regular symbols of elf file: %s", regularSymbolsErr)
	}

	var dynamicSymbols []elf.Symbol
	var dynamicSymbolsErr error
	if len(regularSymbols) != len(symbolSet) {
		dynamicSymbols, dynamicSymbolsErr = getSymbols(elfFile, elf.SHT_DYNSYM, symbolSet)
		if dynamicSymbolsErr != nil && log.ShouldLog(seelog.TraceLvl) {
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
