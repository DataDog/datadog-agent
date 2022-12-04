// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux
// +build linux

package bininspect

import (
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"io"
	"math"
)

// getSymbolNameByEntry extracts from the string data section the string in the given position.
// If the symbol name is shorter or longer than the given min and max (==len(buf)) then we return an error.
func getSymbolNameByEntry(sectionReader io.ReaderAt, startPos, minLength int, buf []byte) (string, bool) {
	_, err := sectionReader.ReadAt(buf, int64(startPos))
	if err != nil {
		return "", false
	}
	for i := 0; i < len(buf); i++ {
		if buf[i] == 0 {
			if i < minLength {
				break
			}
			return string(buf[0:i]), true
		}
	}

	return "", false
}

// getSymbolBoundaries extracts the minimum and maximum lengths of the symbols.
func getSymbolBoundaries(set common.StringSet) (int, int) {
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
// symbol name entry in the string data section.
func readSymbolEntryInStringTable(symbolSectionReader io.ReaderAt, byteOrder binary.ByteOrder, readLocation int64, allocatedBufferForRead []byte) (int, bool) {
	if _, err := symbolSectionReader.ReadAt(allocatedBufferForRead, readLocation); err != nil {
		return 0, false
	}
	return int(byteOrder.Uint32(allocatedBufferForRead)), true
}

// readRestOfSymbol64 reads the symbol entry from the symbol section with the first 4 bytes of the name entry (which
// we read using readSymbolEntryInStringTable).
func readRestOfSymbol64(symbol *elf.Symbol, symbolSectionReader io.ReaderAt, byteOrder binary.ByteOrder, symbolName string, readLocation int64, allocatedBufferForRead []byte) error {
	if _, err := symbolSectionReader.ReadAt(allocatedBufferForRead, readLocation); err != nil {
		return err
	}

	infoAndOther := byteOrder.Uint16(allocatedBufferForRead[0:2])
	symbol.Name = symbolName
	symbol.Info = uint8(infoAndOther >> 8)
	symbol.Other = uint8(infoAndOther)
	symbol.Section = elf.SectionIndex(byteOrder.Uint16(allocatedBufferForRead[2:4]))
	symbol.Value = byteOrder.Uint64(allocatedBufferForRead[4:12])
	symbol.Size = byteOrder.Uint64(allocatedBufferForRead[12:20])
	return nil
}

// readRestOfSymbol32 reads the symbol entry from the symbol section with the first 4 bytes of the name entry (which
// we read using readSymbolEntryInStringTable).
func readRestOfSymbol32(symbol *elf.Symbol, symbolSectionReader io.ReaderAt, byteOrder binary.ByteOrder, symbolName string, readLocation int64, allocatedBufferForRead []byte) error {
	if _, err := symbolSectionReader.ReadAt(allocatedBufferForRead, readLocation); err != nil {
		return err
	}
	symbol.Name = symbolName
	symbol.Value = uint64(byteOrder.Uint32(allocatedBufferForRead[0:4]))
	symbol.Size = uint64(byteOrder.Uint32(allocatedBufferForRead[4:8]))

	infoAndOther := byteOrder.Uint16(allocatedBufferForRead[8:10])
	symbol.Info = uint8(infoAndOther >> 8)
	symbol.Other = uint8(infoAndOther)
	symbol.Section = elf.SectionIndex(byteOrder.Uint16(allocatedBufferForRead[10:12]))
	return nil
}

// getSymbols64 extracts the given symbol list from the binary.
func getSymbols64(f *elf.File, typ elf.SectionType, wantedSymbols common.StringSet) ([]elf.Symbol, error) {
	// Getting the relevant symbol section.
	symbolSection := f.SectionByType(typ)
	if symbolSection == nil {
		return nil, elf.ErrNoSymbols
	}

	// Checking the symbol section size is aligned to a multiplication of Sym64Size.
	if symbolSection.Size%elf.Sym64Size != 0 {
		return nil, errors.New("length of symbol section is not a multiple of Sym64Size")
	}

	// Checking the symbol section link is valid.
	if symbolSection.Link <= 0 || symbolSection.Link >= uint32(len(f.Sections)) {
		return nil, errors.New("section has invalid string table link")
	}

	// Allocating entries for all wanted symbols.
	symbols := make([]elf.Symbol, 0, len(wantedSymbols))

	// Copying the wanted symbol set, so we can modify it during runtime.
	copyOfWantedSymbols := wantedSymbols.Clone()
	// Extracting the min and max symbol length.
	minSymbolNameSize, maxSymbolNameSize := getSymbolBoundaries(copyOfWantedSymbols)
	// Pre-allocating a buffer to read the symbol string into.
	// The size of the buffer is maxSymbolNameSize + 1, for null termination.
	symbolNameBuf := make([]byte, maxSymbolNameSize+1)
	// Pre allocating a buffer for reading the symbol entry in the string table.
	allocatedBufferForSymbolNameRead := make([]byte, 4)
	// Pre allocating a buffer for reading the rest of the symbol fields from the symbol section.
	allocatedBufferForSymbolRead := make([]byte, elf.Sym64Size-len(allocatedBufferForSymbolNameRead))

	// Iterating through the symbol table. We skip the first elf.Sym64Size bytes as they are zeros.
	for readLocation := int64(elf.Sym64Size); uint64(readLocation) < symbolSection.Size; readLocation += elf.Sym64Size {
		// Reading the symbol entry in the string table.
		stringEntry, ok := readSymbolEntryInStringTable(symbolSection.ReaderAt, f.ByteOrder, readLocation, allocatedBufferForSymbolNameRead)
		if !ok {
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
		if _, ok := copyOfWantedSymbols[symbolName]; !ok {
			continue
		}

		// Complete the symbol reading.
		var symbol elf.Symbol
		if err := readRestOfSymbol64(&symbol, symbolSection.ReaderAt, f.ByteOrder, symbolName, readLocation+4, allocatedBufferForSymbolRead); err != nil {
			continue
		}
		symbols = append(symbols, symbol)
		// If relevant, delete it for optimization purposes.
		delete(copyOfWantedSymbols, symbolName)

		// If no symbols left, stop running.
		if len(copyOfWantedSymbols) == 0 {
			break
		}
	}

	return symbols, nil
}

// getSymbols32 extracts the given symbol list from the binary.
func getSymbols32(f *elf.File, typ elf.SectionType, wantedSymbols common.StringSet) ([]elf.Symbol, error) {
	// Getting the relevant symbol section.
	symbolSection := f.SectionByType(typ)
	if symbolSection == nil {
		return nil, elf.ErrNoSymbols
	}

	// Checking the symbol section size is aligned to a multiplication of Sym64Size.
	if symbolSection.Size%elf.Sym32Size != 0 {
		return nil, errors.New("length of symbol section is not a multiple of Sym32Size")
	}

	// Checking the symbol section link is valid.
	if symbolSection.Link <= 0 || symbolSection.Link >= uint32(len(f.Sections)) {
		return nil, errors.New("section has invalid string table link")
	}

	// Allocating entries for all wanted symbols.
	symbols := make([]elf.Symbol, 0, len(wantedSymbols))

	// Copying the wanted symbol set, so we can modify it during runtime.
	copyOfWantedSymbols := wantedSymbols.Clone()
	// Extracting the min and max symbol length.
	minSymbolNameSize, maxSymbolNameSize := getSymbolBoundaries(copyOfWantedSymbols)
	// Pre-allocating a buffer to read the symbol string into.
	// The size of the buffer is maxSymbolNameSize + 1, for null termination.
	symbolNameBuf := make([]byte, maxSymbolNameSize+1)
	// Pre allocating a buffer for reading the symbol entry in the string table.
	allocatedBufferForSymbolNameRead := make([]byte, 4)
	// Pre allocating a buffer for reading the rest of the symbol fields from the symbol section.
	allocatedBufferForSymbolRead := make([]byte, elf.Sym32Size-len(allocatedBufferForSymbolNameRead))

	// Iterating through the symbol table. We skip the first elf.Sym32Size bytes as they are zeros.
	for readLocation := int64(elf.Sym32Size); uint64(readLocation) < symbolSection.Size; readLocation += elf.Sym32Size {
		// Reading the symbol entry in the string table.
		stringEntry, ok := readSymbolEntryInStringTable(symbolSection.ReaderAt, f.ByteOrder, readLocation, allocatedBufferForSymbolNameRead)
		if !ok {
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
		if _, ok := copyOfWantedSymbols[symbolName]; !ok {
			continue
		}

		// Complete the symbol reading.
		var symbol elf.Symbol
		if err := readRestOfSymbol32(&symbol, symbolSection.ReaderAt, f.ByteOrder, symbolName, readLocation+4, allocatedBufferForSymbolRead); err != nil {
			continue
		}
		symbols = append(symbols, symbol)
		// If relevant, delete it for optimization purposes.
		delete(copyOfWantedSymbols, symbolName)
		// If no symbols left, stop running.
		if len(copyOfWantedSymbols) == 0 {
			break
		}
	}

	return symbols, nil
}

func getSymbols(f *elf.File, typ elf.SectionType, wanted map[string]struct{}) ([]elf.Symbol, error) {
	switch f.Class {
	case elf.ELFCLASS64:
		return getSymbols64(f, typ, wanted)

	case elf.ELFCLASS32:
		return getSymbols32(f, typ, wanted)
	}

	return nil, errors.New("not implemented")
}

func GetAllSymbolsByName(elfFile *elf.File, symbolSet common.StringSet) (map[string]elf.Symbol, error) {
	regularSymbols, regularSymbolsErr := getSymbols(elfFile, elf.SHT_SYMTAB, symbolSet)
	if regularSymbolsErr != nil {
		log.Debugf("Failed getting regular symbols of elf file: %s", regularSymbolsErr)
	}

	var dynamicSymbols []elf.Symbol
	var dynamicSymbolsErr error
	if len(regularSymbols) != len(symbolSet) {
		dynamicSymbols, dynamicSymbolsErr = getSymbols(elfFile, elf.SHT_DYNSYM, symbolSet)
		if dynamicSymbolsErr != nil {
			log.Debugf("Failed getting dynamic symbols of elf file: %s", dynamicSymbolsErr)
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
