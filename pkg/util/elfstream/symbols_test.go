// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package elfstream

import (
	"bytes"
	"debug/elf" //nolint:depguard // Test independent decoding and raw encoding structs.
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// goFuncOptions adds a .gofunc section and its go:func.* marker.
func goFuncOptions() StreamOptions {
	return StreamOptions{
		Sections: []StreamSection{{Name: ".gofunc", Address: 0x600000, Data: []byte("gofunc")}},
		Symbols:  []StreamSymbol{{Name: "go:func.*", Section: ".gofunc", Value: 0x600000}},
	}
}

func TestELFStreamSyntheticGoSectionsAndSymbols(t *testing.T) {
	t.Parallel()
	for _, withSymtab := range []bool{false, true} {
		t.Run(map[bool]string{false: "stripped", true: "existing_symtab"}[withSymtab], func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			if !withSymtab {
				// A stripped file has no section named .symtab.
				changeSection(t, input, 9, func(sh *elf.Section64) { sh.Type, sh.Name = uint32(elf.SHT_NOBITS), 0 })
			}
			opts := goFuncOptions()
			opts.Sections = append(opts.Sections, StreamSection{Name: ".gopclntab", Address: 0x500000, Data: []byte("pclntab")})
			opts.Symbols = append(opts.Symbols, StreamSymbol{Name: "runtime.text", Section: ".text", Value: 0x401000})
			_, ef := exportELF(t, input, opts)
			pcln, err := ef.Section(".gopclntab").Data()
			require.NoError(t, err)
			require.Equal(t, "pclntab", string(pcln))
			syms, err := ef.Symbols()
			require.NoError(t, err)
			found := map[string]int{}
			for _, sym := range syms {
				found[sym.Name]++
				switch sym.Name {
				case "go:func.*":
					require.Equal(t, uint64(0x600000), sym.Value)
					require.Equal(t, ".gofunc", ef.Sections[sym.Section].Name)
					require.Zero(t, sym.Size, "relocated metadata markers must not retain an extent larger than the trimmed section")
				case "runtime.text":
					require.Equal(t, uint64(0x401000), sym.Value)
				}
			}
			require.Equal(t, 1, found["go:func.*"])
			require.Equal(t, 1, found["runtime.text"])
		})
	}
}

func TestELFStreamReplacesSuffixSharedSymbolNames(t *testing.T) {
	t.Parallel()
	// Also strings that span the read buffer, once and across several refills.
	for _, prefixLength := range []int{5, streamBufferSize - 4, 2*streamBufferSize + 3, 3*streamBufferSize - 4} {
		t.Run(strconv.Itoa(prefixLength), func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			strtab := append([]byte{0}, bytes.Repeat([]byte{'x'}, prefixLength)...)
			nameOffset := len(strtab)
			strtab = append(strtab, "go:func.*\x00"...)
			symtabOffset := binary.LittleEndian.Uint64(input[shdrOffset(9)+shOffsetField:])
			binary.LittleEndian.PutUint32(input[symtabOffset+2*symSize:], uint32(nameOffset))
			input = withPayload(t, input, 8, strtab, nil)
			_, ef := exportELF(t, input, goFuncOptions())
			symbols, err := ef.Symbols()
			require.NoError(t, err)
			require.Len(t, symbols, 2, "replace the suffix-shared symbol, do not append a duplicate")
			require.Equal(t, "go:func.*", symbols[1].Name)
			require.Equal(t, uint64(0x600000), symbols[1].Value)
			require.Equal(t, ".gofunc", ef.Sections[symbols[1].Section].Name)
		})
	}
}

func TestELFStreamAppendsMarkers(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	// The symbol table is found by type, not by name.
	gdbIndexName := binary.LittleEndian.Uint32(input[shdrOffset(5):])
	changeSection(t, input, 9, func(s *elf.Section64) { s.Name = gdbIndexName })
	var opts StreamOptions
	for i := 0; i < 17; i++ {
		opts.Symbols = append(opts.Symbols, StreamSymbol{Name: fmt.Sprintf("marker_%d", i), Section: ".text", Value: 0x401000})
	}
	// Markers have no size, so one can sit at the end of its section.
	end := uint64(0x401000 + len("DO_NOT_UPLOAD_EXECUTABLE"))
	opts.Symbols = append(opts.Symbols, StreamSymbol{Name: "end_marker", Section: ".text", Value: end})
	_, ef := exportELF(t, input, opts)
	symbols, err := ef.Symbols()
	require.NoError(t, err)
	require.Len(t, symbols, 2+len(opts.Symbols))
	for _, want := range opts.Symbols {
		require.True(t, slices.ContainsFunc(symbols, func(sym elf.Symbol) bool {
			return sym.Name == want.Name && sym.Value == want.Value && sym.Section == 1 && sym.Size == 0
		}), "missing or incorrect marker %q", want.Name)
	}
}

func TestELFStreamRejectsInvalidMarkers(t *testing.T) {
	t.Parallel()
	marker := func(change func(*StreamSymbol)) StreamOptions {
		sym := StreamSymbol{Name: "marker", Section: ".text", Value: 0x401000}
		change(&sym)
		return StreamOptions{Symbols: []StreamSymbol{sym}}
	}
	end := uint64(0x401000 + len("DO_NOT_UPLOAD_EXECUTABLE"))
	cases := map[string]struct {
		input       func(t *testing.T, input []byte) []byte
		opts        StreamOptions
		unsupported bool
	}{
		"empty_name":      {opts: marker(func(sym *StreamSymbol) { sym.Name = "" })},
		"embedded_null":   {opts: marker(func(sym *StreamSymbol) { sym.Name = "marker\x00suffix" })},
		"long_name":       {opts: marker(func(sym *StreamSymbol) { sym.Name = string(bytes.Repeat([]byte("x"), streamBufferSize+1)) })},
		"missing_section": {opts: marker(func(sym *StreamSymbol) { sym.Section = ".missing" })},
		"null_section":    {opts: marker(func(sym *StreamSymbol) { sym.Section = "" })},
		"before_section":  {opts: marker(func(sym *StreamSymbol) { sym.Value = 0x401000 - 1 })},
		"after_section":   {opts: marker(func(sym *StreamSymbol) { sym.Value = end + 1 })},
		// The name appears twice, once as a suffix; no symbol needs to reference it.
		"repeated_name": {
			input: func(t *testing.T, input []byte) []byte {
				return withPayload(t, input, 8, []byte("\x00go:func.*\x00x.go:func.*\x00"), nil)
			},
			opts:        goFuncOptions(),
			unsupported: true,
		},
		"duplicate_symbols": {
			input: func(t *testing.T, input []byte) []byte {
				symtab := make([]byte, 3*symSize) // Null entry, then two symbols named "go:func.*".
				binary.LittleEndian.PutUint32(symtab[symSize:], 1)
				binary.LittleEndian.PutUint32(symtab[2*symSize:], 1)
				input = withPayload(t, input, 8, []byte("\x00go:func.*\x00"), nil)
				return withPayload(t, input, 9, symtab, func(sh *elf.Section64) { sh.Info = 1 })
			},
			opts:        goFuncOptions(),
			unsupported: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			if tc.input != nil {
				input = tc.input(t, input)
			}
			_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), tc.opts)
			require.Error(t, err)
			require.Equal(t, tc.unsupported, errors.Is(err, ErrUnsupported), "%v", err)
		})
	}
}
