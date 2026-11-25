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

// checkInput runs the checks NewELFStream applies to input, without options.
func checkInput(t testing.TB, input io.ReaderAt, size int64) error {
	t.Helper()
	_, err := NewELFStream(t.Context(), input, size, StreamOptions{})
	return err
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

// withPayload points section index at payload, appended to the input.
func withPayload(t testing.TB, input []byte, index int, payload []byte, change func(*elf.Section64)) []byte {
	t.Helper()
	changeSection(t, input, index, func(sh *elf.Section64) {
		sh.Off, sh.Size = uint64(len(input)), uint64(len(payload))
		if change != nil {
			change(sh)
		}
	})
	return append(input, payload...)
}

func TestELFStreamMetadataPhase(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	r := &virtualELF{prefix: input, size: int64(len(input))}
	s, err := readELF64(t.Context(), r, r.size)
	require.NoError(t, err)
	var header elf.Header64
	require.NoError(t, binary.Read(bytes.NewReader(input), binary.LittleEndian, &header))
	require.Equal(t, header, s.elfHeader)
	for i, sec := range s.sections {
		var original elf.Section64
		require.NoError(t, binary.Read(bytes.NewReader(input[shdrOffset(i):]), binary.LittleEndian, &original))
		require.Equal(t, original, sec.header, "reading must not discard or rewrite headers")
		require.Equal(t, original.Off, sec.inputOffset)
		require.Zero(t, sec.inputSize, "reading must not select payloads")
	}
	require.Equal(t, ".text", s.sections[1].name)
	require.Equal(t, ".shstrtab", s.sections[10].name)
	require.Len(t, s.programHeaders, 1)
	require.Equal(t, uint64(len(input)), s.programHeaders[0].Filesz, "program headers must still describe the input")

	// Later selection and layout use loaded metadata, not another input read.
	r.size, r.maxRead = 0, 0
	require.NoError(t, s.selectSections(nil))
	require.NoError(t, s.applySections(nil))
	require.NoError(t, s.checkRetainedSections(int64(len(input))))
	require.NoError(t, s.planProgramHeaders())
	require.NoError(t, s.planLayout())
	require.Zero(t, r.maxRead)
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
	require.ErrorIs(t, checkInput(t, r, r.size), ErrUnsupported)
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
		"version":       {func(p []byte) { binary.LittleEndian.PutUint32(p[20:], uint32(elf.EV_NONE)) }, false},
		"ident_version": {func(p []byte) { p[elf.EI_VERSION] = byte(elf.EV_NONE) }, false},
		"bad_magic":     {func(p []byte) { p[0] = 'X' }, false},
		"bad_class":     {func(p []byte) { p[elf.EI_CLASS] = 3 }, false},
		"header_size":   {func(p []byte) { binary.LittleEndian.PutUint16(p[52:], 52) }, false},
		"program_size":  {func(p []byte) { binary.LittleEndian.PutUint16(p[54:], 32) }, false},
		"section_size":  {func(p []byte) { binary.LittleEndian.PutUint16(p[58:], 40) }, false},
		"large_section": {func(p []byte) { binary.LittleEndian.PutUint16(p[58:], 72) }, true},
		"names_index":   {func(p []byte) { binary.LittleEndian.PutUint16(p[62:], 11) }, false},
		"invalid_name":  {func(p []byte) { changeSection(t, p, 3, func(sh *elf.Section64) { sh.Name = math.MaxUint32 }) }, false},
		"name_no_nul":   {func(p []byte) { p[len(p)-1] = 'x' }, false},
		"null_section":  {func(p []byte) { binary.LittleEndian.PutUint32(p[shdrOffset(0)+shTypeField:], uint32(elf.SHT_PROGBITS)) }, false},
		"section_table": {func(p []byte) { binary.LittleEndian.PutUint64(p[40:], ^uint64(0)) }, false},
		"extended":      {func(p []byte) { binary.LittleEndian.PutUint16(p[60:], 0) }, true},
		"huge_names":    {func(p []byte) { changeSection(t, p, 10, func(s *elf.Section64) { s.Size = 1 << 40 }) }, false},
		// Checked while planning, after reading.
		"symtab_size": {func(p []byte) { changeSection(t, p, 9, func(s *elf.Section64) { s.Size++ }) }, false},
		"symtab_info": {func(p []byte) { changeSection(t, p, 9, func(s *elf.Section64) { s.Info = 1000 }) }, false},
		"bad_link":    {func(p []byte) { changeSection(t, p, 9, func(s *elf.Section64) { s.Link = 1234 }) }, false},
		"bad_range":   {func(p []byte) { changeSection(t, p, 3, func(s *elf.Section64) { s.Size = ^uint64(0) }) }, false},
		"huge_alignment": {func(p []byte) {
			changeSection(t, p, 3, func(s *elf.Section64) { s.Addralign = 1 << 40 })
		}, true},
		"truncated_compression_header": {func(p []byte) {
			changeSection(t, p, 8, func(s *elf.Section64) { s.Flags = uint64(elf.SHF_COMPRESSED) })
		}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			tc.mutate(input)
			err := checkInput(t, bytes.NewReader(input), int64(len(input)))
			require.Error(t, err)
			require.Equal(t, tc.unsupported, errors.Is(err, ErrUnsupported), "%v", err)
		})
	}
	for _, input := range [][]byte{nil, []byte("not ELF"), testELF(t, binary.LittleEndian)[:100]} {
		require.Error(t, checkInput(t, bytes.NewReader(input), int64(len(input))))
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
			err := checkInput(t, bytes.NewReader(input), limit)
			require.Error(t, err)
			require.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF), "%v", err)
		})
	}
}

func TestELFStreamRetainsEmptyNotes(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	changeSection(t, input, 4, func(sh *elf.Section64) { sh.Size = 0 })
	_, ef := exportELF(t, input, StreamOptions{})
	require.Equal(t, elf.SHT_NOTE, ef.Sections[4].Type, "zero bytes must not mean unselected")
	require.Zero(t, ef.Sections[4].Size)
}

func TestELFStreamPreservesDebugMetadataNotExecutableData(t *testing.T) {
	t.Parallel()
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		t.Run(order.String(), func(t *testing.T) {
			input := testELF(t, order)
			out, ef := exportELF(t, input, StreamOptions{KeepSections: []string{".dynsym", ".dynstr"}})
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
			dynsyms, err := ef.DynamicSymbols()
			require.NoError(t, err)
			require.Len(t, dynsyms, 1)
			require.Equal(t, "exported", dynsyms[0].Name)
		})
	}
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
					input = withPayload(t, input, index, payload, func(sh *elf.Section64) {
						sh.Type, sh.Flags, sh.Link, sh.Info = uint32(typ), 0, 9, targetIndex
						sh.Entsize, sh.Addralign = uint64(len(payload)), 8
					})
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

// A retained relocation section keeps its symbol table; one whose target is
// omitted or replaced is omitted itself, without following its links.
func TestELFStreamRelocationLinks(t *testing.T) {
	t.Parallel()
	replaced := StreamOptions{Sections: []StreamSection{{Name: ".debug_info", Data: []byte("replacement")}}}
	for name, tc := range map[string]struct {
		link, target uint32
		opts         StreamOptions
		want         elf.SectionType // Zero when the input is rejected.
	}{
		"no_symbols":       {0, 3, StreamOptions{}, shtRela},
		"kept_dynsym":      {6, 3, StreamOptions{KeepSections: []string{".dynsym"}}, shtRela},
		"omitted_dynsym":   {6, 3, StreamOptions{}, 0},
		"omitted_strings":  {8, 3, StreamOptions{}, 0},
		"outside_input":    {11, 3, StreamOptions{}, 0},
		"discarded_target": {11, 1, StreamOptions{}, elf.SHT_NOBITS},
		"replaced_target":  {9, 3, replaced, elf.SHT_NOBITS},
	} {
		t.Run(name, func(t *testing.T) {
			input := withPayload(t, testELF(t, binary.LittleEndian), 2, make([]byte, 24), func(sh *elf.Section64) {
				sh.Type, sh.Flags, sh.Link, sh.Info, sh.Entsize = uint32(shtRela), 0, tc.link, tc.target, 24
			})
			if tc.want == 0 {
				_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), tc.opts)
				require.ErrorContains(t, err, "relocation symbol table")
				return
			}
			_, ef := exportELF(t, input, tc.opts)
			require.Equal(t, tc.want, ef.Sections[2].Type)
			if tc.want == shtRela {
				require.Equal(t, tc.link, ef.Sections[2].Link)
			}
		})
	}
}

func TestELFStreamWrite(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	plan, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{})
	require.NoError(t, err)
	var a, b bytes.Buffer
	require.NoError(t, plan.Write(t.Context(), &a))
	require.NoError(t, plan.Write(t.Context(), &b))
	require.Equal(t, a.Bytes(), b.Bytes(), "a plan can be written more than once")
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

// Header fields of omitted sections are never used, those of retained sections
// are checked, and a replacement is checked instead of the input payload.
func TestELFStreamSectionMetadata(t *testing.T) {
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
			changeSection(t, input, 2, mutate) // .rodata is omitted.
			_, ef := exportELF(t, input, StreamOptions{})
			require.Equal(t, elf.SHT_NOBITS, ef.Section(".rodata").Type)

			changeSection(t, input, 3, mutate) // .debug_info is retained.
			_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{})
			require.Error(t, err)

			replacement := StreamSection{Name: ".debug_info", Address: 0x700000, Data: []byte("replacement debug data")}
			_, ef = exportELF(t, input, StreamOptions{Sections: []StreamSection{replacement}})
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

func TestELFStreamSectionNames(t *testing.T) {
	t.Parallel()
	name := ".debug_" + string(bytes.Repeat([]byte("x"), 8192))
	t.Run("input", func(t *testing.T) {
		t.Parallel()
		input := testELF(t, binary.LittleEndian)
		prefix := "shared_prefix"
		var nameOffset uint32
		var namesSize uint64
		changeSection(t, input, 10, func(sh *elf.Section64) {
			nameOffset = uint32(sh.Size)
			sh.Size += uint64(len(prefix) + len(name) + 1)
			namesSize = sh.Size
		})
		input = append(input, []byte(prefix+name+"\x00")...)
		// Two names can share bytes, including a name starting inside another.
		changeSection(t, input, 2, func(sh *elf.Section64) { sh.Name = nameOffset })
		changeSection(t, input, 3, func(sh *elf.Section64) { sh.Name = nameOffset + uint32(len(prefix)) })
		_, ef := exportELF(t, input, StreamOptions{})
		require.Equal(t, prefix+name, ef.Sections[2].Name)
		require.Equal(t, name, ef.Sections[3].Name)
		require.EqualValues(t, namesSize, ef.Sections[10].Size, "shared names must not be duplicated")
		data, err := ef.Sections[3].Data()
		require.NoError(t, err)
		require.Equal(t, "debug information", string(data))
	})
	t.Run("supplied", func(t *testing.T) {
		t.Parallel()
		input := testELF(t, binary.LittleEndian)
		original, err := elf.NewFile(bytes.NewReader(input))
		require.NoError(t, err)
		names, err := original.Sections[10].Data()
		require.NoError(t, err)
		_, ef := exportELF(t, input, StreamOptions{Sections: []StreamSection{{Name: name, Data: []byte("supplied")}}})
		got, err := ef.Sections[10].Data()
		require.NoError(t, err)
		// The input table is kept as-is, so existing offsets stay valid; new names follow it.
		require.Equal(t, names, got[:len(names)])
		require.Equal(t, name+"\x00", string(got[len(names):]))
		for i, sec := range original.Sections {
			require.Equal(t, sec.Name, ef.Sections[i].Name)
		}
		data, err := ef.Sections[len(ef.Sections)-1].Data()
		require.NoError(t, err)
		require.Equal(t, "supplied", string(data))
	})
	t.Run("unexpected_table_name", func(t *testing.T) {
		t.Parallel()
		input := testELF(t, binary.LittleEndian)
		// The name table has no mandated name; a misleading one must not
		// override the fact that it is structurally required.
		gdbIndexName := binary.LittleEndian.Uint32(input[shdrOffset(5):])
		changeSection(t, input, 10, func(s *elf.Section64) { s.Name = gdbIndexName })
		_, ef := exportELF(t, input, StreamOptions{})
		require.Equal(t, elf.SHT_STRTAB, ef.Sections[10].Type)
	})
}

// Version tables are omitted by type, whatever their name or selection, and do
// not retain their linked strings.
func TestELFStreamOmitsVersionTables(t *testing.T) {
	t.Parallel()
	for _, typ := range []elf.SectionType{elf.SHT_GNU_VERSYM, elf.SHT_GNU_VERDEF, elf.SHT_GNU_VERNEED} {
		t.Run(typ.String(), func(t *testing.T) {
			input := testELF(t, binary.LittleEndian)
			versionStrings := []byte("\x00libexample.so\x00EXAMPLE_1\x00")
			input = withPayload(t, input, 2, versionStrings, func(sh *elf.Section64) { sh.Type, sh.Flags = uint32(elf.SHT_STRTAB), 0 })
			// .debug_info becomes the version table: its debug name must not retain it.
			changeSection(t, input, 3, func(sh *elf.Section64) {
				sh.Type, sh.Link = uint32(typ), 2
				if typ == elf.SHT_GNU_VERSYM {
					sh.Link = 6
				}
			})
			for _, keep := range [][]string{nil, {".dynsym"}, {".debug_info"}} {
				out, ef := exportELF(t, input, StreamOptions{KeepSections: keep})
				require.Equal(t, elf.SHT_NOBITS, ef.Sections[3].Type)
				require.Equal(t, elf.SHT_NOBITS, ef.Sections[2].Type, "linked strings must not be retained")
				require.NotContains(t, string(out), "debug information")
				require.NotContains(t, string(out), "EXAMPLE_1")
				if len(keep) == 1 && keep[0] == ".dynsym" {
					syms, err := ef.DynamicSymbols()
					require.NoError(t, err)
					require.Len(t, syms, 1)
					require.Equal(t, "exported", syms[0].Name)
				}
			}
			// Omitted links are not followed, and the strings can still be
			// retained on their own.
			changeSection(t, input, 3, func(sh *elf.Section64) { sh.Link = math.MaxUint32 })
			_, ef := exportELF(t, input, StreamOptions{KeepSections: []string{".rodata"}})
			require.Equal(t, elf.SHT_NOBITS, ef.Sections[3].Type)
			data, err := ef.Sections[2].Data()
			require.NoError(t, err)
			require.Equal(t, versionStrings, data)
		})
	}
}

func TestELFStreamAdditionalSections(t *testing.T) {
	t.Parallel()
	input := testELF(t, binary.LittleEndian)
	var opts StreamOptions
	for i := 0; i < 17; i++ {
		opts.Sections = append(opts.Sections, StreamSection{Name: fmt.Sprintf(".go_metadata_%d", i), Data: []byte("metadata")})
	}
	_, ef := exportELF(t, input, opts)
	for _, extra := range opts.Sections {
		data, err := ef.Section(extra.Name).Data()
		require.NoError(t, err)
		require.Equal(t, extra.Data, data)
	}

	for _, tc := range []struct{ name, err string }{
		{"", "invalid additional section name"},
		{".go\x00data", "invalid additional section name"},
		{".symtab", "cannot replace section"},
		{".strtab", "cannot replace section"},
		{".shstrtab", "cannot replace section"},
		{".dynsym", "cannot replace section"},
		{".dynstr", "cannot replace section"},
	} {
		_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{
			KeepSections: []string{".dynsym"},
			Sections:     []StreamSection{{Name: tc.name, Data: []byte("replacement")}},
		})
		require.ErrorContains(t, err, tc.err, "%q", tc.name)
	}
	changeSection(t, input, 3, func(sh *elf.Section64) { sh.Link = 11 })
	_, err := NewELFStream(t.Context(), bytes.NewReader(input), int64(len(input)), StreamOptions{
		Sections: []StreamSection{{Name: ".extra", Data: []byte("metadata")}},
	})
	require.ErrorContains(t, err, "invalid retained section 3 link", "retained links cannot target appended sections")
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
