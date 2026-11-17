// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package elfstream

import (
	"bytes"
	"compress/zlib"
	"context"
	"debug/elf" //nolint:depguard // Test independent decoding and raw encoding structs.
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// testELF layout: ELF header, one program header, padding, section headers at
// testShoff, then section payloads in header order.
const (
	testShoff     = 128
	shTypeField   = 4  // sh_type offset within a section header.
	shOffsetField = 24 // sh_offset offset within a section header.
)

// shdrOffset is the file offset of section header i in a testELF input.
func shdrOffset(i int) int { return testShoff + i*shdrSize }

func testELF(t testing.TB, order binary.ByteOrder) []byte {
	t.Helper()
	type section struct {
		name string
		data []byte
		h    elf.Section64
	}
	var syms bytes.Buffer
	for _, sym := range []elf.Sym64{{}, {Name: 1, Info: 0x12, Shndx: 1, Value: 0x401000, Size: 16}, {Name: 11, Info: 0x11, Shndx: 2, Value: 0x402000, Size: 8}} {
		require.NoError(t, binary.Write(&syms, order, sym))
	}
	var dynsym bytes.Buffer
	require.NoError(t, binary.Write(&dynsym, order, elf.Sym64{}))
	require.NoError(t, binary.Write(&dynsym, order, elf.Sym64{Name: 1, Info: 0x12, Shndx: 1, Value: 0x401000}))
	sections := []section{
		{},
		{name: ".text", data: []byte("DO_NOT_UPLOAD_EXECUTABLE"), h: elf.Section64{Type: uint32(elf.SHT_PROGBITS), Flags: uint64(elf.SHF_ALLOC | elf.SHF_EXECINSTR), Addr: 0x401000}},
		{name: ".rodata", data: []byte("DO_NOT_UPLOAD_SECRET_DATA"), h: elf.Section64{Type: uint32(elf.SHT_PROGBITS), Flags: uint64(elf.SHF_ALLOC), Addr: 0x402000}},
		{name: ".debug_info", data: []byte("debug information"), h: elf.Section64{Type: uint32(elf.SHT_PROGBITS)}},
		{name: ".note.gnu.build-id", data: []byte("build-id"), h: elf.Section64{Type: uint32(elf.SHT_NOTE)}},
		{name: ".gdb_index", data: []byte("DO_NOT_UPLOAD_GDB_INDEX"), h: elf.Section64{Type: uint32(elf.SHT_PROGBITS)}},
		{name: ".dynsym", data: dynsym.Bytes(), h: elf.Section64{Type: uint32(elf.SHT_DYNSYM), Flags: uint64(elf.SHF_ALLOC), Link: 7, Info: 1, Entsize: 24}},
		{name: ".dynstr", data: []byte("\x00exported\x00"), h: elf.Section64{Type: uint32(elf.SHT_STRTAB), Flags: uint64(elf.SHF_ALLOC)}},
		{name: ".strtab", data: []byte("\x00main.main\x00go:func.*\x00"), h: elf.Section64{Type: uint32(elf.SHT_STRTAB)}},
		{name: ".symtab", data: syms.Bytes(), h: elf.Section64{Type: uint32(elf.SHT_SYMTAB), Link: 8, Info: 1, Entsize: 24}},
		{name: ".shstrtab", h: elf.Section64{Type: uint32(elf.SHT_STRTAB)}},
	}
	names := []byte{0}
	for i := 1; i < len(sections); i++ {
		sections[i].h.Name = uint32(len(names))
		names = append(names, sections[i].name...)
		names = append(names, 0)
	}
	sections[10].data = names
	var ident [16]byte
	copy(ident[:], elf.ELFMAG)
	ident[elf.EI_CLASS], ident[elf.EI_DATA], ident[elf.EI_VERSION] = byte(elf.ELFCLASS64), byte(elf.ELFDATA2LSB), byte(elf.EV_CURRENT)
	if order == binary.BigEndian {
		ident[elf.EI_DATA] = byte(elf.ELFDATA2MSB)
	}
	h := elf.Header64{Ident: ident, Type: uint16(elf.ET_DYN), Machine: uint16(elf.EM_X86_64), Version: 1, Entry: 0x401000, Phoff: ehdrSize, Shoff: testShoff, Ehsize: ehdrSize, Phentsize: phdrSize, Phnum: 1, Shentsize: shdrSize, Shnum: uint16(len(sections)), Shstrndx: 10}
	position := h.Shoff + uint64(len(sections))*shdrSize
	for i := 1; i < len(sections); i++ {
		sections[i].h.Off, sections[i].h.Size, sections[i].h.Addralign = position, uint64(len(sections[i].data)), 1
		position += uint64(len(sections[i].data))
	}
	var buf bytes.Buffer
	require.NoError(t, binary.Write(&buf, order, h))
	require.NoError(t, binary.Write(&buf, order, elf.Prog64{Type: uint32(elf.PT_LOAD), Flags: uint32(elf.PF_R | elf.PF_X), Vaddr: 0x400000, Paddr: 0x400000, Filesz: position, Memsz: position, Align: 4096}))
	buf.Write(make([]byte, 8))
	for _, sec := range sections {
		require.NoError(t, binary.Write(&buf, order, sec.h))
	}
	for _, sec := range sections[1:] {
		buf.Write(sec.data)
	}
	return buf.Bytes()
}

func changeSection(t testing.TB, data []byte, index int, change func(*elf.Section64)) {
	t.Helper()
	var sh elf.Section64
	require.NoError(t, binary.Read(bytes.NewReader(data[shdrOffset(index):]), binary.LittleEndian, &sh))
	change(&sh)
	var b bytes.Buffer
	require.NoError(t, binary.Write(&b, binary.LittleEndian, sh))
	copy(data[shdrOffset(index):], b.Bytes())
}

func exportELF(t testing.TB, data []byte, opts StreamOptions) ([]byte, *elf.File) {
	t.Helper()
	plan, err := NewELFStream(context.Background(), bytes.NewReader(data), int64(len(data)), opts)
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, plan.Write(context.Background(), &out))
	require.Equal(t, plan.Size(), int64(out.Len()))
	ef, err := elf.NewFile(bytes.NewReader(out.Bytes()))
	require.NoError(t, err)
	t.Cleanup(func() { ef.Close() })
	return out.Bytes(), ef
}

func TestELFStreamMetadataPhase(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	r := &virtualELF{prefix: input, size: int64(len(input))}
	s, err := readELF64(t.Context(), r, r.size)
	require.NoError(t, err)
	var header elf.Header64
	require.NoError(t, binary.Read(bytes.NewReader(input), binary.LittleEndian, &header))
	require.Equal(t, header, s.header)
	for i, sec := range s.sections {
		var original elf.Section64
		require.NoError(t, binary.Read(bytes.NewReader(input[shdrOffset(i):]), binary.LittleEndian, &original))
		require.Equal(t, original, sec.header, "reading must not discard or rewrite headers")
		require.Equal(t, original.Off, sec.source)
		require.Zero(t, sec.sourceSize, "reading must not select payloads")
	}
	require.Equal(t, ".text", s.sections[1].name)
	require.Equal(t, ".shstrtab", s.sections[10].name)
	require.Len(t, s.programs, 1)
	require.Equal(t, uint64(len(input)), s.programs[0].Filesz, "program headers must still describe the input")

	// Later selection and layout use loaded metadata, not another input read.
	r.size, r.maxRead = 0, 0
	require.NoError(t, s.selectSections(nil))
	require.NoError(t, s.applySections(nil))
	require.NoError(t, s.checkRetainedSections(int64(len(input))))
	require.NoError(t, s.planProgramHeaders())
	require.NoError(t, s.planLayout())
	require.Zero(t, r.maxRead)
}

func TestELFStreamPreservesDebugMetadataNotExecutableData(t *testing.T) {
	t.Parallel()
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		t.Run(order.String(), func(t *testing.T) {
			input := testELF(t, order)
			out, ef := exportELF(t, input, StreamOptions{})
			for _, secret := range []string{"DO_NOT_UPLOAD_EXECUTABLE", "DO_NOT_UPLOAD_SECRET_DATA", "DO_NOT_UPLOAD_GDB_INDEX"} {
				require.NotContains(t, string(out), secret)
			}
			require.Equal(t, elf.SHT_NOBITS, ef.Section(".text").Type)
			require.Equal(t, uint64(0x401000), ef.Section(".text").Addr)
			require.Equal(t, uint64(0x400000), ef.Progs[0].Vaddr)
			require.Equal(t, uint64(ehdrSize+phdrSize), ef.Progs[0].Filesz, "only generated ELF/program headers are mapped")
			debug, err := ef.Section(".debug_info").Data()
			require.NoError(t, err)
			require.Equal(t, "debug information", string(debug))
			syms, err := ef.Symbols()
			require.NoError(t, err)
			require.Equal(t, "main.main", syms[0].Name)
			require.Equal(t, uint64(0x401000), syms[0].Value)
			require.Equal(t, "go:func.*", syms[1].Name)
		})
	}
}

func TestELFStreamProgramHeaderMapping(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	// Insert a PT_PHDR after the LOAD, before the section table. This models the
	// PHDR/LOAD arrangement used by native arm64 and externally linked binaries.
	const (
		phnumField  = 56 // e_phnum offset in the ELF header.
		shoffField  = 40 // e_shoff offset in the ELF header.
		fileszField = 32 // p_filesz offset in a program header.
		memszField  = 40 // p_memsz offset in a program header.
		phdrStart   = ehdrSize + phdrSize
		phdrEnd     = phdrStart + phdrSize
		tableSize   = 2 * phdrSize
		shoff       = testShoff + phdrSize
	)
	input = append(append(append([]byte{}, input[:phdrStart]...), make([]byte, phdrSize)...), input[phdrStart:]...)
	binary.LittleEndian.PutUint16(input[phnumField:], 2)
	binary.LittleEndian.PutUint64(input[shoffField:], shoff)
	for i := 1; i < 11; i++ {
		pos := shoff + i*shdrSize + shOffsetField
		binary.LittleEndian.PutUint64(input[pos:], binary.LittleEndian.Uint64(input[pos:])+phdrSize)
	}
	binary.LittleEndian.PutUint64(input[ehdrSize+fileszField:], uint64(len(input)))
	binary.LittleEndian.PutUint64(input[ehdrSize+memszField:], uint64(len(input)))
	var ph bytes.Buffer
	require.NoError(t, binary.Write(&ph, binary.LittleEndian, elf.Prog64{Type: uint32(elf.PT_PHDR), Off: ehdrSize, Vaddr: 0x400000 + ehdrSize, Paddr: 0x400000 + ehdrSize, Filesz: tableSize, Memsz: tableSize, Align: 8}))
	copy(input[phdrStart:phdrEnd], ph.Bytes())
	output, ef := exportELF(t, input, StreamOptions{})
	require.Equal(t, elf.PT_PHDR, ef.Progs[1].Type)
	require.Equal(t, uint64(ehdrSize), ef.Progs[1].Off)
	require.Equal(t, uint64(tableSize), ef.Progs[1].Filesz)
	require.Equal(t, ef.Progs[0].Vaddr+ehdrSize, ef.Progs[1].Vaddr)
	require.GreaterOrEqual(t, ef.Progs[0].Filesz, ef.Progs[1].Off+ef.Progs[1].Filesz)
	mapped, err := io.ReadAll(ef.Progs[1].Open())
	require.NoError(t, err)
	require.Equal(t, output[ehdrSize:phdrEnd], mapped)

	t.Run("interpreter_omitted", func(t *testing.T) {
		original, err := elf.NewFile(bytes.NewReader(input))
		require.NoError(t, err)
		defer original.Close()
		strings := original.Section(".dynstr")
		var interp bytes.Buffer
		require.NoError(t, binary.Write(&interp, binary.LittleEndian, elf.Prog64{Type: uint32(elf.PT_INTERP), Off: strings.Offset + 1, Filesz: strings.Size - 1, Memsz: strings.Size - 1, Align: 1}))
		copy(input[ehdrSize+phdrSize:ehdrSize+2*phdrSize], interp.Bytes())
		_, emitted := exportELF(t, input, StreamOptions{})
		require.Equal(t, elf.PT_NULL, emitted.Progs[1].Type, "an omitted interpreter must not leave a dangling program header")
		require.Zero(t, emitted.Progs[1].Filesz)
		require.Zero(t, emitted.Progs[1].Memsz)
	})
}

func TestELFStreamDynamicSymbols(t *testing.T) {
	t.Parallel()
	_, ef := exportELF(t, testELF(t, binary.LittleEndian), StreamOptions{KeepSections: []string{".dynsym", ".dynstr"}})
	syms, err := ef.DynamicSymbols()
	require.NoError(t, err)
	require.Len(t, syms, 1)
	require.Equal(t, "exported", syms[0].Name)
	require.Equal(t, elf.SHT_DYNSYM, ef.Section(".dynsym").Type)
}

func TestELFStreamDebugRelocations(t *testing.T) {
	t.Parallel()
	for _, typ := range []elf.SectionType{elf.SHT_REL, shtRela} {
		for _, index := range []int{2, 5} { // Before and after the target; 5 is named .gdb_index.
			for _, target := range []string{"debug", "discarded", "empty", "outside"} {
				t.Run(fmt.Sprintf("%s/%d/%s", typ, index, target), func(t *testing.T) {
					input := testELF(t, binary.LittleEndian)
					payload := make([]byte, 16)
					if typ == shtRela {
						payload = make([]byte, 24)
					}
					binary.LittleEndian.PutUint64(payload[8:], 1<<32|uint64(elf.R_X86_64_64))
					targetIndex := uint32(3)
					switch target {
					case "discarded":
						targetIndex = 1
					case "outside": // Omitted metadata is not validated.
						targetIndex = 99
					case "empty":
						changeSection(t, input, 3, func(sh *elf.Section64) { sh.Size = 0 })
					}
					changeSection(t, input, index, func(sh *elf.Section64) {
						sh.Type, sh.Flags, sh.Link, sh.Info = uint32(typ), 0, 9, targetIndex
						sh.Off, sh.Size, sh.Entsize, sh.Addralign = uint64(len(input)), uint64(len(payload)), uint64(len(payload)), 8
					})
					input = append(input, payload...)
					_, ef := exportELF(t, input, StreamOptions{})
					if target != "debug" {
						require.Equal(t, elf.SHT_NOBITS, ef.Sections[index].Type)
						return
					}
					require.Equal(t, typ, ef.Sections[index].Type)
					require.Equal(t, targetIndex, ef.Sections[index].Info)
					data, err := ef.Sections[index].Data()
					require.NoError(t, err)
					require.Equal(t, payload, data)
				})
			}
		}
	}
}

func TestELFStreamOmitsRelocationsOfReplacedSections(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	payload := make([]byte, 24)
	changeSection(t, input, 2, func(sh *elf.Section64) {
		sh.Type, sh.Flags, sh.Link, sh.Info = uint32(shtRela), 0, 9, 3 // Targets the kept debug section.
		sh.Off, sh.Size, sh.Entsize, sh.Addralign = uint64(len(input)), uint64(len(payload)), 24, 8
	})
	input = append(input, payload...)
	in, err := elf.NewFile(bytes.NewReader(input))
	require.NoError(t, err)
	target := in.Sections[3].Name
	_, ef := exportELF(t, input, StreamOptions{Sections: []StreamSection{{Name: target, Data: []byte("replacement")}}})
	require.Equal(t, elf.SHT_NOBITS, ef.Sections[2].Type, "relocations must not apply to replacement bytes")
	data, err := ef.Section(target).Data()
	require.NoError(t, err)
	require.Equal(t, "replacement", string(data))
}

func TestELFStreamRetainedRelocationsRequireRetainedSymbols(t *testing.T) {
	t.Parallel()
	for _, link := range []uint32{6, 8, 11} { // Omitted dynsym, strings, outside input.
		t.Run(strconv.FormatUint(uint64(link), 10), func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			payload := make([]byte, 24)
			changeSection(t, input, 2, func(sh *elf.Section64) {
				sh.Type, sh.Flags, sh.Link, sh.Info = uint32(shtRela), 0, link, 3
				sh.Off, sh.Size, sh.Entsize = uint64(len(input)), uint64(len(payload)), 24
			})
			input = append(input, payload...)
			_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{})
			require.ErrorContains(t, err, "relocation symbol table")
			if link == 6 {
				_, ef := exportELF(t, input, StreamOptions{KeepSections: []string{".dynsym"}})
				require.Equal(t, shtRela, ef.Sections[2].Type)
				require.Equal(t, elf.SHT_DYNSYM, ef.Sections[link].Type)
			}
			// No dependency is followed when the relocation payload is omitted.
			changeSection(t, input, 2, func(sh *elf.Section64) { sh.Info = 1 })
			_, ef := exportELF(t, input, StreamOptions{})
			require.Equal(t, elf.SHT_NOBITS, ef.Sections[2].Type)
		})
	}
}

func TestELFStreamRetainsRelocationsWithoutSymbols(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	payload := make([]byte, 24)
	changeSection(t, input, 2, func(sh *elf.Section64) {
		sh.Type, sh.Flags, sh.Link, sh.Info = uint32(shtRela), 0, 0, 3 // Link 0: no symbols referenced.
		sh.Off, sh.Size, sh.Entsize = uint64(len(input)), uint64(len(payload)), 24
	})
	input = append(input, payload...)
	_, ef := exportELF(t, input, StreamOptions{})
	require.Equal(t, shtRela, ef.Sections[2].Type)
	require.Zero(t, ef.Sections[2].Link)
}

func TestELFStreamManyAdditionalSymbols(t *testing.T) {
	t.Parallel()
	var opts StreamOptions
	for i := 0; i < 17; i++ {
		opts.Symbols = append(opts.Symbols, StreamSymbol{Name: fmt.Sprintf("marker_%d", i), Section: ".text", Value: 0x401000})
	}
	_, ef := exportELF(t, testELF(t, binary.LittleEndian), opts)
	symbols, err := ef.Symbols()
	require.NoError(t, err)
	require.Len(t, symbols, 2+len(opts.Symbols))
	for _, want := range opts.Symbols {
		require.True(t, slices.ContainsFunc(symbols, func(sym elf.Symbol) bool {
			return sym.Name == want.Name && sym.Value == want.Value && sym.Section == 1 && sym.Size == 0
		}), "missing or incorrect marker %q", want.Name)
	}
}

func TestELFStreamValidatesSuppliedSymbolMarkers(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	end := uint64(0x401000 + len("DO_NOT_UPLOAD_EXECUTABLE"))
	for name, mutate := range map[string]func(*StreamSymbol){
		"empty_name":      func(sym *StreamSymbol) { sym.Name = "" },
		"embedded_null":   func(sym *StreamSymbol) { sym.Name = "marker\x00suffix" },
		"missing_section": func(sym *StreamSymbol) { sym.Section = ".missing" },
		"null_section":    func(sym *StreamSymbol) { sym.Section = "" },
		"before_section":  func(sym *StreamSymbol) { sym.Value = 0x401000 - 1 },
		"after_section":   func(sym *StreamSymbol) { sym.Value = end + 1 },
	} {
		t.Run(name, func(t *testing.T) {
			marker := StreamSymbol{Name: "marker", Section: ".text", Value: 0x401000}
			mutate(&marker)
			_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{Symbols: []StreamSymbol{marker}})
			require.Error(t, err)
		})
	}
	_, ef := exportELF(t, input, StreamOptions{Symbols: []StreamSymbol{{Name: "end_marker", Section: ".text", Value: end}}})
	symbols, err := ef.Symbols()
	require.NoError(t, err)
	require.Equal(t, "end_marker", symbols[len(symbols)-1].Name)
	require.Equal(t, end, symbols[len(symbols)-1].Value)
	require.Zero(t, symbols[len(symbols)-1].Size)
}

func TestELFStreamSyntheticGoSectionsAndSymbols(t *testing.T) {
	t.Parallel()
	for _, withSymtab := range []bool{false, true} {
		t.Run(map[bool]string{false: "stripped", true: "existing_symtab"}[withSymtab], func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			if !withSymtab {
				changeSection(t, input, 9, func(sh *elf.Section64) { sh.Type = uint32(elf.SHT_NOBITS) })
				// A stripped file has no section named .symtab.
				changeSection(t, input, 9, func(sh *elf.Section64) { sh.Name = 0 })
			}
			_, ef := exportELF(t, input, StreamOptions{
				Sections: []StreamSection{{Name: ".gopclntab", Address: 0x500000, Data: []byte("pclntab")}, {Name: ".gofunc", Address: 0x600000, Data: []byte("gofunc")}},
				Symbols:  []StreamSymbol{{Name: "go:func.*", Section: ".gofunc", Value: 0x600000}, {Name: "runtime.text", Section: ".text", Value: 0x401000}},
			})
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
			strings := append([]byte{0}, bytes.Repeat([]byte{'x'}, prefixLength)...)
			nameOffset := len(strings)
			strings = append(strings, "go:func.*\x00"...)
			symtabOffset := binary.LittleEndian.Uint64(input[shdrOffset(9)+shOffsetField:])
			binary.LittleEndian.PutUint32(input[symtabOffset+48:], uint32(nameOffset))
			changeSection(t, input, 8, func(sh *elf.Section64) { sh.Off, sh.Size = uint64(len(input)), uint64(len(strings)) })
			input = append(input, strings...)
			_, ef := exportELF(t, input, StreamOptions{
				Sections: []StreamSection{{Name: ".gofunc", Address: 0x600000, Data: []byte("gofunc")}},
				Symbols:  []StreamSymbol{{Name: "go:func.*", Section: ".gofunc", Value: 0x600000}},
			})
			symbols, err := ef.Symbols()
			require.NoError(t, err)
			require.Len(t, symbols, 2, "replace the suffix-shared symbol, do not append a duplicate")
			require.Equal(t, "go:func.*", symbols[1].Name)
			require.Equal(t, uint64(0x600000), symbols[1].Value)
			require.Equal(t, ".gofunc", ef.Sections[symbols[1].Section].Name)
		})
	}
}

func TestELFStreamPreservesCompressedSectionBytes(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	var compressed bytes.Buffer
	payload := bytes.Repeat([]byte("debug info "), 100)
	require.NoError(t, binary.Write(&compressed, binary.LittleEndian, elf.Chdr64{Type: uint32(elf.COMPRESS_ZLIB), Size: uint64(len(payload)), Addralign: 1}))
	zw := zlib.NewWriter(&compressed)
	_, err := zw.Write(payload)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	changeSection(t, input, 3, func(sh *elf.Section64) {
		sh.Flags, sh.Off, sh.Size = uint64(elf.SHF_COMPRESSED), uint64(len(input)), uint64(compressed.Len())
	})
	input = append(input, compressed.Bytes()...)
	out, ef := exportELF(t, input, StreamOptions{})
	sec := ef.Section(".debug_info")
	decoded, err := sec.Data()
	require.NoError(t, err)
	require.Equal(t, payload, decoded)
	require.Equal(t, compressed.Bytes(), out[sec.Offset:sec.Offset+sec.FileSize])
}

type virtualELF struct {
	prefix  []byte
	size    int64
	maxRead int
}

func (v *virtualELF) ReadAt(p []byte, offset int64) (int, error) {
	v.maxRead = max(v.maxRead, len(p))
	if offset < 0 || offset >= v.size {
		return 0, io.EOF
	}
	n := min(int64(len(p)), v.size-offset)
	clear(p[:n])
	if offset < int64(len(v.prefix)) {
		copy(p[:n], v.prefix[offset:])
	}
	if n < int64(len(p)) {
		return int(n), io.EOF
	}
	return int(n), nil
}

func TestELFStreamRejectsOversizedSectionNames(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	// The table fits in the (virtual) input but exceeds the in-memory bound.
	changeSection(t, input, 10, func(sh *elf.Section64) { sh.Off, sh.Size = uint64(len(input)), maxSectionNames+1 })
	r := &virtualELF{prefix: input, size: int64(len(input)) + maxSectionNames + 1}
	_, err := NewELFStream(t.Context(), r, r.size, StreamOptions{})
	require.ErrorIs(t, err, ErrUnsupported)
	require.Less(t, r.maxRead, 4096, "the oversized table must not be read")
}

func TestELFStreamLargeSectionsUseBoundedReads(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	changeSection(t, input, 3, func(sh *elf.Section64) { sh.Off, sh.Size = uint64(len(input)), 64<<20 })
	r := &virtualELF{prefix: input, size: int64(len(input)) + 64<<20}
	plan, err := NewELFStream(t.Context(), r, r.size, StreamOptions{})
	require.NoError(t, err)
	require.Less(t, r.maxRead, 4096, "planning must not read debug payloads")
	require.NoError(t, plan.Write(t.Context(), io.Discard))
	require.LessOrEqual(t, r.maxRead, streamBufferSize)
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestELFStreamCancellationAndIOErrors(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	plan, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{})
	require.NoError(t, err)
	require.ErrorIs(t, plan.Write(t.Context(), writerFunc(func([]byte) (int, error) { return 0, io.ErrClosedPipe })), io.ErrClosedPipe)
	require.ErrorIs(t, plan.Write(t.Context(), writerFunc(func(p []byte) (int, error) { return len(p) - 1, nil })), io.ErrShortWrite)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.ErrorIs(t, plan.Write(ctx, writerFunc(func(p []byte) (int, error) { cancel(); return len(p), nil })), context.Canceled)
	// The input can disappear or shrink after planning. Never emit a successful,
	// silently padded output when a selected source range becomes truncated.
	debugOffset := binary.LittleEndian.Uint64(input[shdrOffset(3)+shOffsetField:])
	plan.input = bytes.NewReader(input[:debugOffset+1])
	require.ErrorIs(t, plan.Write(t.Context(), io.Discard), io.ErrUnexpectedEOF)
}

// Inputs fail explicitly. ErrUnsupported marks well-formed layouts that are not
// handled, so they can be reported apart from malformed inputs.
func TestELFStreamRejectsMalformedAndUnsupportedInput(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		mutate      func([]byte)
		unsupported bool
	}{
		"class32":       {func(p []byte) { p[elf.EI_CLASS] = byte(elf.ELFCLASS32) }, true},
		"byte_order":    {func(p []byte) { p[elf.EI_DATA] = 0 }, false},
		"object_type":   {func(p []byte) { binary.LittleEndian.PutUint16(p[16:], uint16(elf.ET_REL)) }, true},
		"version":       {func(p []byte) { binary.LittleEndian.PutUint32(p[20:], uint32(elf.EV_NONE)) }, true},
		"header_size":   {func(p []byte) { binary.LittleEndian.PutUint16(p[52:], 52) }, true},
		"program_size":  {func(p []byte) { binary.LittleEndian.PutUint16(p[54:], 32) }, true},
		"section_size":  {func(p []byte) { binary.LittleEndian.PutUint16(p[58:], 40) }, true},
		"names_index":   {func(p []byte) { binary.LittleEndian.PutUint16(p[62:], 11) }, false},
		"invalid_name":  {func(p []byte) { changeSection(t, p, 3, func(sh *elf.Section64) { sh.Name = math.MaxUint32 }) }, false},
		"name_no_nul":   {func(p []byte) { p[len(p)-1] = 'x' }, false},
		"null_section":  {func(p []byte) { binary.LittleEndian.PutUint32(p[shdrOffset(0)+shTypeField:], uint32(elf.SHT_PROGBITS)) }, false},
		"section_table": {func(p []byte) { binary.LittleEndian.PutUint64(p[40:], ^uint64(0)) }, false},
		"extended":      {func(p []byte) { binary.LittleEndian.PutUint16(p[60:], 0) }, true},
		"bad_link":      {func(p []byte) { changeSection(t, p, 9, func(s *elf.Section64) { s.Link = 1234 }) }, false},
		"bad_range":     {func(p []byte) { changeSection(t, p, 3, func(s *elf.Section64) { s.Size = ^uint64(0) }) }, false},
		"huge_alignment": {func(p []byte) {
			changeSection(t, p, 3, func(s *elf.Section64) { s.Addralign = 1 << 40 })
		}, true},
		"huge_names": {func(p []byte) { changeSection(t, p, 10, func(s *elf.Section64) { s.Size = 1 << 40 }) }, false},
		"shared_name_table": {func(p []byte) {
			changeSection(t, p, 9, func(s *elf.Section64) { s.Link = 10 })
		}, true},
		"truncated_compression_header": {func(p []byte) {
			changeSection(t, p, 8, func(s *elf.Section64) { s.Flags = uint64(elf.SHF_COMPRESSED) })
		}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			tc.mutate(input)
			_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{})
			require.Error(t, err)
			require.Equal(t, tc.unsupported, errors.Is(err, ErrUnsupported), "%v", err)
		})
	}
	for _, input := range [][]byte{nil, []byte("not ELF"), testELF(t, binary.LittleEndian)[:100]} {
		_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{})
		require.Error(t, err)
	}
}

func TestELFStreamNormalizesNullSection(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	changeSection(t, input, 0, func(sh *elf.Section64) { sh.Name = 1; sh.Flags = 1; sh.Addr = 2 })
	output, _ := exportELF(t, input, StreamOptions{})
	shoff := binary.LittleEndian.Uint64(output[40:])
	require.Equal(t, make([]byte, shdrSize), output[shoff:shoff+shdrSize])
}

func TestELFStreamIgnoresDiscardedSectionMetadata(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*elf.Section64){
		"offset":      func(sh *elf.Section64) { sh.Off = math.MaxUint64 },
		"size":        func(sh *elf.Section64) { sh.Size = 1 << 20 },
		"link":        func(sh *elf.Section64) { sh.Link = math.MaxUint32 },
		"alignment":   func(sh *elf.Section64) { sh.Addralign = 3 },
		"compression": func(sh *elf.Section64) { sh.Flags |= uint64(elf.SHF_COMPRESSED); sh.Size = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			changeSection(t, input, 2, mutate) // .rodata bytes are not uploaded.
			_, ef := exportELF(t, input, StreamOptions{})
			require.Equal(t, elf.SHT_NOBITS, ef.Section(".rodata").Type)
			data, err := ef.Section(".debug_info").Data()
			require.NoError(t, err)
			require.Equal(t, "debug information", string(data))
		})
	}
}

func TestELFStreamOmittedSectionSizeFitsReader(t *testing.T) {
	t.Parallel()
	for _, typ := range []elf.SectionType{elf.SHT_PROGBITS, elf.SHT_NOBITS} {
		for _, size := range []uint64{math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64} {
			t.Run(fmt.Sprintf("%s/%d", typ, size), func(t *testing.T) {
				input := testELF(t, binary.LittleEndian)
				changeSection(t, input, 2, func(sh *elf.Section64) { sh.Type, sh.Size = uint32(typ), size })
				if size > math.MaxInt64 {
					_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{})
					require.ErrorContains(t, err, "omitted section size")
					return
				}
				_, ef := exportELF(t, input, StreamOptions{})
				require.Equal(t, elf.SHT_NOBITS, ef.Sections[2].Type)
				require.Equal(t, size, ef.Sections[2].Size)
			})
		}
	}
}

func TestELFStreamMetadataReadsRespectInputSize(t *testing.T) {
	t.Parallel()
	for _, table := range []string{"header", "sections", "programs", "names"} {
		t.Run(table, func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			limit := int64(64)
			switch table {
			case "header":
				limit = 63
			case "sections":
				limit = int64(shdrOffset(11)) - 1
			case "programs":
				// Keep sections readable, but move program headers beyond the limit.
				binary.LittleEndian.PutUint64(input[32:], uint64(len(input)))
				input = append(input, input[ehdrSize:ehdrSize+phdrSize]...)
				limit = int64(len(input) - 1)
			case "names":
				limit = int64(len(input) - 1)
			}
			_, err := NewELFStream(t.Context(), bytes.NewReader(input), limit, StreamOptions{})
			require.Error(t, err)
			require.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF), "%v", err)
		})
	}
}

func TestELFStreamRejectsRetainedSectionMetadata(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*elf.Section64){
		"range":       func(sh *elf.Section64) { sh.Off = math.MaxUint64 },
		"link":        func(sh *elf.Section64) { sh.Link = math.MaxUint32 },
		"alignment":   func(sh *elf.Section64) { sh.Addralign = 3 },
		"compression": func(sh *elf.Section64) { sh.Flags |= uint64(elf.SHF_COMPRESSED); sh.Size = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			changeSection(t, input, 3, mutate) // .debug_info bytes are uploaded.
			_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{})
			require.Error(t, err)
		})
	}
}

func TestELFStreamRetainedLinksCannotTargetAppendedSections(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	changeSection(t, input, 3, func(sh *elf.Section64) { sh.Link = 11 })
	_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{
		Sections: []StreamSection{{Name: ".extra", Data: []byte("metadata")}},
	})
	require.ErrorContains(t, err, "invalid retained section 3 link")
}

func TestELFStreamRetainsEmptyNotes(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	changeSection(t, input, 4, func(sh *elf.Section64) { sh.Size = 0 })
	_, ef := exportELF(t, input, StreamOptions{})
	require.Equal(t, elf.SHT_NOTE, ef.Sections[4].Type, "zero bytes must not mean unselected")
	require.Zero(t, ef.Sections[4].Size)
}

func TestELFStreamChecksReplacementNotOriginalPayload(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*elf.Section64){
		"range":       func(sh *elf.Section64) { sh.Off = math.MaxUint64 },
		"link":        func(sh *elf.Section64) { sh.Link = math.MaxUint32 },
		"alignment":   func(sh *elf.Section64) { sh.Addralign = 3 },
		"compression": func(sh *elf.Section64) { sh.Flags |= uint64(elf.SHF_COMPRESSED); sh.Size = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			changeSection(t, input, 3, mutate)
			_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{})
			require.Error(t, err, "the original payload is unsafe if retained")
			replacement := StreamSection{Name: ".debug_info", Address: 0x700000, Data: []byte("replacement debug data")}
			_, ef := exportELF(t, input, StreamOptions{Sections: []StreamSection{replacement}})
			section := ef.Section(replacement.Name)
			require.Equal(t, elf.SHT_PROGBITS, section.Type)
			require.Equal(t, replacement.Address, section.Addr)
			require.Zero(t, section.Flags&elf.SHF_COMPRESSED)
			data, err := section.Data()
			require.NoError(t, err)
			require.Equal(t, replacement.Data, data)
		})
	}
}

func TestELFStreamLongSectionNames(t *testing.T) {
	t.Parallel()
	name := ".debug_" + string(bytes.Repeat([]byte("x"), 8192))
	for _, prefix := range []string{"", "shared_prefix"} {
		t.Run("input/"+prefix, func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			var nameOffset uint32
			changeSection(t, input, 10, func(sh *elf.Section64) {
				nameOffset = uint32(sh.Size)
				sh.Size += uint64(len(prefix) + len(name) + 1)
			})
			input = append(input, []byte(prefix+name+"\x00")...)
			changeSection(t, input, 3, func(sh *elf.Section64) { sh.Name = nameOffset + uint32(len(prefix)) })
			if prefix != "" {
				// Two names can share bytes, including a name starting inside another.
				changeSection(t, input, 2, func(sh *elf.Section64) { sh.Name = nameOffset })
			}
			_, ef := exportELF(t, input, StreamOptions{})
			require.Equal(t, name, ef.Sections[3].Name)
			data, err := ef.Sections[3].Data()
			require.NoError(t, err)
			require.Equal(t, "debug information", string(data))
			if prefix != "" {
				require.Equal(t, prefix+name, ef.Sections[2].Name)
				require.Equal(t, elf.SHT_NOBITS, ef.Sections[2].Type)
			}
		})
	}
	t.Run("supplied", func(t *testing.T) {
		_, ef := exportELF(t, testELF(t, binary.LittleEndian), StreamOptions{
			Sections: []StreamSection{{Name: name, Data: []byte("supplied debug data")}},
		})
		data, err := ef.Section(name).Data()
		require.NoError(t, err)
		require.Equal(t, "supplied debug data", string(data))
	})
}

func TestELFStreamAppendsSectionNames(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	original, err := elf.NewFile(bytes.NewReader(input))
	require.NoError(t, err)
	names, err := original.Sections[10].Data()
	require.NoError(t, err)
	_, ef := exportELF(t, input, StreamOptions{Sections: []StreamSection{{Name: ".added", Data: []byte("x")}}})
	got, err := ef.Sections[10].Data()
	require.NoError(t, err)
	// The input table is kept as-is, so existing offsets stay valid; new names follow it.
	require.Equal(t, names, got[:len(names)])
	require.Equal(t, append([]byte(".added"), 0), got[len(names):])
	for i, sec := range original.Sections {
		require.Equal(t, sec.Name, ef.Sections[i].Name)
	}
	require.Equal(t, ".added", ef.Sections[len(ef.Sections)-1].Name)
}

func TestELFStreamSharedSectionNamesDoNotExpand(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	name := bytes.Repeat([]byte("x"), 1<<20)
	var nameOffset uint32
	var namesSize uint64
	changeSection(t, input, 10, func(sh *elf.Section64) {
		nameOffset = uint32(sh.Size)
		sh.Size += uint64(len(name) + 1)
		namesSize = sh.Size
	})
	input = append(append(input, name...), 0)
	for _, index := range []int{1, 2} {
		changeSection(t, input, index, func(sh *elf.Section64) { sh.Name = nameOffset })
	}
	_, ef := exportELF(t, input, StreamOptions{})
	// Sections sharing one name offset must not duplicate the name in the output.
	require.EqualValues(t, namesSize, ef.Sections[10].Size)
	require.Equal(t, string(name), ef.Sections[1].Name)
	require.Equal(t, string(name), ef.Sections[2].Name)
}

func TestELFStreamRequiredTablesWithUnexpectedNames(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	gdbIndexName := binary.LittleEndian.Uint32(input[shdrOffset(5):])
	changeSection(t, input, 10, func(s *elf.Section64) { s.Name = gdbIndexName })
	// The ELF name table has no mandated name. A misleading name must not
	// override the fact that the table is structurally required.
	_, ef := exportELF(t, input, StreamOptions{})
	require.Equal(t, elf.SHT_STRTAB, ef.Sections[10].Type)
	changeSection(t, input, 9, func(s *elf.Section64) { s.Name = gdbIndexName })
	_, ef = exportELF(t, input, StreamOptions{Symbols: []StreamSymbol{{Name: "runtime.text", Section: ".text", Value: 0x401000}}})
	symbols, err := ef.Symbols()
	require.NoError(t, err)
	require.Equal(t, "runtime.text", symbols[len(symbols)-1].Name)
}

func TestELFStreamOmitsVersionTables(t *testing.T) {
	t.Parallel()
	for name, typ := range map[string]elf.SectionType{
		".gnu.version":   elf.SHT_GNU_VERSYM,
		".gnu.version_d": elf.SHT_GNU_VERDEF,
		".gnu.version_r": elf.SHT_GNU_VERNEED,
	} {
		t.Run(name, func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			original, err := elf.NewFile(bytes.NewReader(input))
			require.NoError(t, err)
			defer original.Close()
			expectedSymbols, err := original.DynamicSymbols()
			require.NoError(t, err)
			payload, err := original.Sections[3].Data()
			require.NoError(t, err)
			names, err := original.Sections[10].Data()
			require.NoError(t, err)
			nameOffset := uint32(len(names))
			debugName := binary.LittleEndian.Uint32(input[shdrOffset(3):])
			names = append(names, []byte(name+"\x00")...)
			changeSection(t, input, 10, func(sh *elf.Section64) { sh.Off, sh.Size = uint64(len(input)), uint64(len(names)) })
			input = append(input, names...)
			strings := []byte("\x00libexample.so\x00EXAMPLE_1\x00")
			changeSection(t, input, 2, func(sh *elf.Section64) {
				sh.Type, sh.Flags, sh.Off, sh.Size = uint32(elf.SHT_STRTAB), 0, uint64(len(input)), uint64(len(strings))
			})
			input = append(input, strings...)
			changeSection(t, input, 3, func(sh *elf.Section64) {
				sh.Name, sh.Type, sh.Link = nameOffset, uint32(typ), 2
				if typ == elf.SHT_GNU_VERSYM {
					sh.Link = 6
				}
			})

			omitted, ef := exportELF(t, input, StreamOptions{KeepSections: []string{".dynsym"}})
			require.Equal(t, elf.SHT_NOBITS, ef.Section(name).Type, "dynsym must not implicitly retain version payloads")
			require.Equal(t, elf.SHT_NOBITS, ef.Sections[2].Type, "unused version strings must also be omitted")
			require.NotContains(t, string(omitted), string(payload))
			require.NotContains(t, string(omitted), string(strings))
			actualSymbols, err := ef.DynamicSymbols()
			require.NoError(t, err)
			require.Equal(t, expectedSymbols, actualSymbols, "ordinary dynamic symbols/strings remain unchanged")
			changeSection(t, input, 3, func(sh *elf.Section64) { sh.Name = debugName })
			_, misnamed := exportELF(t, input, StreamOptions{KeepSections: []string{".dynsym"}})
			require.Equal(t, elf.SHT_NOBITS, misnamed.Sections[3].Type, "a debug-like name must not implicitly retain a version table")
			changeSection(t, input, 3, func(sh *elf.Section64) { sh.Name = nameOffset })

			for _, keep := range [][]string{{".dynsym", name}, {name}} {
				out, ef := exportELF(t, input, StreamOptions{KeepSections: keep})
				require.Equal(t, elf.SHT_NOBITS, ef.Section(name).Type, "explicit selection must not retain version payloads")
				require.Equal(t, elf.SHT_NOBITS, ef.Sections[2].Type, "version selection must not retain linked strings")
				require.NotContains(t, string(out), string(payload))
				require.NotContains(t, string(out), string(strings))
				if len(keep) == 2 {
					require.Equal(t, omitted, out, "requesting a version section must not change output")
				}
			}

			// Type, not name, determines whether this is excluded version metadata.
			changeSection(t, input, 3, func(sh *elf.Section64) { sh.Name = debugName })
			_, ef = exportELF(t, input, StreamOptions{KeepSections: []string{".debug_info"}})
			require.Equal(t, elf.SHT_NOBITS, ef.Sections[3].Type)
			changeSection(t, input, 3, func(sh *elf.Section64) { sh.Name, sh.Link = nameOffset, math.MaxUint32 })
			_, err = NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{KeepSections: []string{name}})
			require.NoError(t, err, "discarded version links are not followed")

			// The same string table is still retained if requested independently.
			_, ef = exportELF(t, input, StreamOptions{KeepSections: []string{name, ".rodata"}})
			require.Equal(t, elf.SHT_NOBITS, ef.Section(name).Type)
			require.Equal(t, elf.SHT_STRTAB, ef.Sections[2].Type)
			actual, err := ef.Sections[2].Data()
			require.NoError(t, err)
			require.Equal(t, strings, actual)
		})
	}
}

func TestELFStreamRejectsInvalidAdditionalSections(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", ".go\x00data", ".symtab", ".strtab", ".shstrtab", ".dynsym", ".dynstr"} {
		t.Run(name, func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{
				KeepSections: []string{".dynsym"},
				Sections:     []StreamSection{{Name: name, Data: []byte("replacement")}},
			})
			if name == "" || name == ".go\x00data" {
				require.ErrorContains(t, err, "invalid additional section name")
			} else {
				require.ErrorContains(t, err, "cannot replace section")
			}
		})
	}
}

func TestELFStreamMoreThanSixteenAdditionalSections(t *testing.T) {
	t.Parallel()
	var opts StreamOptions
	for i := 0; i < 17; i++ {
		opts.Sections = append(opts.Sections, StreamSection{Name: fmt.Sprintf(".go_metadata_%d", i), Data: []byte("metadata")})
	}
	_, ef := exportELF(t, testELF(t, binary.LittleEndian), opts)
	for _, extra := range opts.Sections {
		data, err := ef.Section(extra.Name).Data()
		require.NoError(t, err)
		require.Equal(t, extra.Data, data)
	}
}

func TestELFStreamRepeatable(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	plan, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{})
	require.NoError(t, err)
	var a, b bytes.Buffer
	require.NoError(t, plan.Write(t.Context(), &a))
	require.NoError(t, plan.Write(t.Context(), &b))
	require.Equal(t, a.Bytes(), b.Bytes())
}

func FuzzELFStream(f *testing.F) {
	f.Add(testELF(f, binary.LittleEndian))
	f.Add(testELF(f, binary.BigEndian))
	f.Add([]byte("not ELF"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 2<<20 {
			t.Skip()
		}
		plan, err := NewELFStream(t.Context(), bytes.NewReader(data), int64(len(data)), StreamOptions{})
		if err != nil {
			return
		}
		// A plan may contain at most the input payload plus bounded headers/names.
		if plan.Size() > 16<<20 {
			return
		}
		var out bytes.Buffer
		err = plan.Write(t.Context(), &out)
		if !errors.Is(err, context.Canceled) {
			require.NoError(t, err)
		}
		require.Equal(t, plan.Size(), int64(out.Len()))
		_, err = elf.NewFile(bytes.NewReader(out.Bytes()))
		require.NoError(t, err)
	})
}
