package telemetry

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/native"
	"github.com/cilium/ebpf"
)

const (
	lowerOrder7Mask = 0x7f
	higherOrderBit  = 0x80
	BPFStackLimit   = 512
)

// https://en.wikipedia.org/wiki/LEB128
func decodeULEB128(in io.Reader) (uint64, error) {
	var result, shift uint64
	for {
		b := []byte{0}
		if _, err := in.Read(b); err != nil {
			return 0, fmt.Errorf("failed to decode ULEB128: %w", err)
		}
		result |= uint64(b[0]&lowerOrder7Mask) << shift
		if b[0]&higherOrderBit == 0 {
			return result, nil
		}
		shift += 7
	}
}

type stackSizes map[string]uint64

type symbolKey struct {
	index int    // section index for the section containing this symbol
	value uint64 // offset into section
}

// This function finds all the symbols matching the program names, and indexes them by
// their section index, and offset into section. The offset into the section is needed to
// deduplicate programs in the same section. This may happen for example when there are multiple
// kprobes on the same function.
func findFunctionSymbols(elfFile *elf.File, programSpecs map[string]*ebpf.ProgramSpec) (map[symbolKey]string, error) {
	syms, err := elfFile.Symbols()
	if err != nil {
		return nil, fmt.Errorf("failed to read elf symbols: %w", err)
	}

	symbols := make(map[symbolKey]string, len(programSpecs))
	for _, sym := range syms {
		if _, ok := programSpecs[sym.Name]; ok {
			symbols[symbolKey{int(sym.Section), sym.Value}] = sym.Name
		}
	}

	return symbols, nil
}

// This function parses the '.stack_sizes' section and records the stack usage of each function.
// It assumes that each function will have a corresponding '.stack_sizes' section. It returns an error otherwise.
// The '.stack_sizes' section contains an unsigned 64 bit integer and an ULEB128 integer.
// The unsigned 64 bit integer corresponds to the 'st_value' field of the symbol. For functions this is the offset
// in the section to the start of the function.
// The ULEB128 integer represents the stack size of the function.
func parseStackSizesSections(bytecode io.ReaderAt, programSpecs map[string]*ebpf.ProgramSpec) (stackSizes, error) {
	objFile, err := elf.NewFile(bytecode)
	if err != nil {
		return nil, fmt.Errorf("failed to open bytecode: %w", err)
	}

	symbols, err := findFunctionSymbols(objFile, programSpecs)
	if err != nil {
		return nil, fmt.Errorf("failed to find function symbols: %v", err)
	}

	sizes := make(stackSizes, len(programSpecs))
	for _, section := range objFile.Sections {
		if section.Name != ".stack_sizes" {
			continue
		}

		sectionReader := section.Open()
		for {
			var s uint64
			if err := binary.Read(sectionReader, native.Endian, &s); err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("error reading '.stack_sizes' section: %w", err)
			}

			size, err := decodeULEB128(sectionReader)
			if err != nil {
				return nil, err
			}

			if _, ok := symbols[symbolKey{int(section.Link), s}]; ok {
				sizes[symbols[symbolKey{int(section.Link), s}]] = size
			}
		}
	}

	var notFound []string
	for pName, _ := range programSpecs {
		if _, ok := sizes[pName]; !ok {
			notFound = append(notFound, pName)
		}
	}
	if notFound != nil {
		return nil, fmt.Errorf("failed to find stack sizes for programs: [%s]", strings.Join(notFound, ", "))
	}

	return sizes, nil
}

func (sizes stackSizes) stackHas8BytesFree(function string) bool {
	return sizes[function] <= (BPFStackLimit - 8)
}
