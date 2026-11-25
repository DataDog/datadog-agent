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
	_, err := readELF64(t.Context(), input, size)
	return err
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
	}
	require.Equal(t, ".text", s.sections[1].name)
	require.Equal(t, ".shstrtab", s.sections[10].name)
	require.Len(t, s.programHeaders, 1)
	require.Equal(t, uint64(len(input)), s.programHeaders[0].Filesz, "program headers must still describe the input")
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
