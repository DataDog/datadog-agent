// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package usdt

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// The fixtures are built from testdata/sdt_fixture.c, see testdata/Makefile.
func TestReadProbes(t *testing.T) {
	for _, tc := range []struct {
		file    string
		machine safeelf.Machine
		nop     []byte
		args    Spec
	}{
		{
			file:    "sdt_fixture_x86_64.so",
			machine: safeelf.EM_X86_64,
			nop:     []byte{0x90},
			// -4@-20(%rbp) 8@%rax 1@$1 -2@%r10w
			args: specOf(derefArg(-4, x86RBP, -20), regArg(8, x86RAX), constArg(1, 1), regArg(-2, x86R10)),
		},
		{
			file:    "sdt_fixture_arm64.so",
			machine: safeelf.EM_AARCH64,
			nop:     []byte{0x1f, 0x20, 0x03, 0xd5},
			// -4@[sp, 60] 8@x1 1@1 -2@w10
			args: specOf(derefArg(-4, 248, 60), regArg(8, 8), constArg(1, 1), regArg(-2, 80)),
		},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join("testdata", tc.file)
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			f, err := safeelf.Open(path)
			require.NoError(t, err)
			defer f.Close()
			require.Equal(t, tc.machine, f.Machine)

			probes, err := ReadProbes(f)
			require.NoError(t, err)

			var names []string
			for _, p := range probes {
				assert.Equal(t, "test", p.Provider)
				names = append(names, p.Name)
			}
			// the type 1 note is ignored
			require.Equal(t, []string{"args", "noargs", "noargs", "prelinked", "nobase", "semaphore", "sib", "data", "notnop"}, names)

			var text *safeelf.Prog
			for _, prog := range f.Progs {
				if prog.Type == safeelf.PT_LOAD && prog.Flags&safeelf.PF_X != 0 {
					text = prog
				}
			}
			require.NotNil(t, text)
			require.NotEqual(t, text.Vaddr, text.Off, "the fixture must have file offsets different from virtual addresses")

			// The first 7 notes describe consecutive nops of fixture(): the
			// location of the "prelinked" site is only right if adjusted.
			nopSize := uint64(len(tc.nop))
			for i, p := range probes[:7] {
				assert.Equal(t, probes[0].Location+uint64(i)*nopSize, p.Location, p.Name)
				assert.Equal(t, p.Location-text.Vaddr+text.Off, p.FileOffset, p.Name)
				assert.Equal(t, tc.nop, content[p.FileOffset:p.FileOffset+nopSize], p.Name)
			}

			for _, p := range probes[:6] {
				assert.NoError(t, p.Err, p.Name)
			}

			args := probes[0]
			assert.Equal(t, tc.args, args.Spec)

			for _, p := range probes[1:3] {
				assert.Empty(t, p.Args)
				assert.Equal(t, Spec{}, p.Spec)
			}

			prelinked := probes[3]
			want, err := ParseArgs(tc.machine, prelinked.Args)
			require.NoError(t, err)
			assert.Equal(t, want, prelinked.Spec)
			assert.EqualValues(t, 1, prelinked.Spec.Cnt)

			for i, p := range probes {
				if p.Name != "semaphore" {
					assert.Zero(t, p.Semaphore, p.Name)
					assert.Zero(t, p.SemaphoreFileOffset, p.Name)
					continue
				}
				semaphores := f.Section(".probes")
				require.NotNil(t, semaphores)
				assert.Equal(t, 5, i)
				assert.Equal(t, semaphores.Addr, p.Semaphore)
				assert.Equal(t, semaphores.Offset, p.SemaphoreFileOffset)
				assert.NotEqual(t, p.Semaphore, p.SemaphoreFileOffset)
			}

			sib := probes[6]
			assert.Error(t, sib.Err)
			assert.Equal(t, Spec{}, sib.Spec)

			data := probes[7]
			assert.ErrorContains(t, data.Err, "probe test:data at")
			assert.ErrorContains(t, data.Err, "site is not in an executable segment")
			assert.Zero(t, data.FileOffset)

			notNop := probes[8]
			assert.ErrorContains(t, notNop.Err, "site is not on a nop instruction")
			assert.Zero(t, notNop.FileOffset)
		})
	}
}

func TestReadProbesNoNotes(t *testing.T) {
	// ELF64 header without any section nor segment
	var header bytes.Buffer
	header.Write([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	for _, v := range []any{
		uint16(3), uint16(safeelf.EM_X86_64), uint32(1), // type, machine, version
		uint64(0), uint64(0), uint64(0), uint32(0), // entry, phoff, shoff, flags
		uint16(64), uint16(56), uint16(0), uint16(64), uint16(0), uint16(0), // ehsize, phentsize, phnum, shentsize, shnum, shstrndx
	} {
		require.NoError(t, binary.Write(&header, binary.LittleEndian, v))
	}
	f, err := safeelf.NewFile(bytes.NewReader(header.Bytes()))
	require.NoError(t, err)

	probes, err := ReadProbes(f)
	assert.NoError(t, err)
	assert.Empty(t, probes)
}

func TestReadProbesNoteSectionTooLarge(t *testing.T) {
	f, err := safeelf.Open(filepath.Join("testdata", "sdt_fixture_x86_64.so"))
	require.NoError(t, err)
	defer f.Close()
	sec := f.Section(noteSection)
	require.NotNil(t, sec)

	// a section claiming more data than the limit is not read
	sec.Size = maxNoteSectionSize + 1
	_, err = ReadProbes(f)
	assert.ErrorContains(t, err, "too large")
}

func note(order binary.ByteOrder, name string, typ uint32, desc []byte, align int) []byte {
	var b bytes.Buffer
	pad := func() {
		for b.Len()%align != 0 {
			b.WriteByte(0)
		}
	}
	_ = binary.Write(&b, order, []uint32{uint32(len(name) + 1), uint32(len(desc)), typ})
	b.WriteString(name)
	b.WriteByte(0)
	pad()
	b.Write(desc)
	pad()
	return b.Bytes()
}

func TestSplitNotes(t *testing.T) {
	le := binary.LittleEndian
	first := note(le, "stapsdt", 3, []byte("abcde"), 4)
	second := note(binary.BigEndian, "GNU", 1, []byte("12345678"), 8)

	notes, err := splitNotes(slices.Concat(first, first), le, 4)
	require.NoError(t, err)
	assert.Equal(t, []rawNote{{[]byte("stapsdt"), 3, []byte("abcde")}, {[]byte("stapsdt"), 3, []byte("abcde")}}, notes)

	// the padding of the last note may be missing
	notes, err = splitNotes(first[:len(first)-3], le, 4)
	require.NoError(t, err)
	assert.Equal(t, []rawNote{{[]byte("stapsdt"), 3, []byte("abcde")}}, notes)

	notes, err = splitNotes(slices.Concat(second, second), binary.BigEndian, 8)
	require.NoError(t, err)
	assert.Equal(t, []rawNote{{[]byte("GNU"), 1, []byte("12345678")}, {[]byte("GNU"), 1, []byte("12345678")}}, notes)

	_, err = splitNotes(slices.Concat(first, first[:11]), le, 4)
	assert.ErrorContains(t, err, "truncated note header")
	_, err = splitNotes(first[:len(first)-4], le, 4)
	assert.ErrorContains(t, err, "truncated note")
	// huge sizes must not overflow
	_, err = splitNotes([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 3, 0, 0, 0}, le, 4)
	assert.ErrorContains(t, err, "truncated note")
}

func TestParseNoteDesc(t *testing.T) {
	le := binary.LittleEndian
	desc64 := le.AppendUint64(le.AppendUint64(le.AppendUint64(nil, 0x1234), 0x5678), 0x9abc)
	n, err := parseNoteDesc(slices.Concat(desc64, []byte("prov\x00name\x008@%rax\x00")), 8, le)
	require.NoError(t, err)
	assert.Equal(t, sdtNote{location: 0x1234, base: 0x5678, semaphore: 0x9abc, provider: "prov", name: "name", args: "8@%rax"}, n)

	be := binary.BigEndian
	desc32 := be.AppendUint32(be.AppendUint32(be.AppendUint32(nil, 0x1234), 0x5678), 0)
	n, err = parseNoteDesc(slices.Concat(desc32, []byte("prov\x00name\x00\x00")), 4, be)
	require.NoError(t, err)
	assert.Equal(t, sdtNote{location: 0x1234, base: 0x5678, provider: "prov", name: "name"}, n)

	for _, bad := range [][]byte{
		desc64[:23],
		slices.Concat(desc64, []byte("prov\x00name\x00args")),
		slices.Concat(desc64, []byte("\x00name\x00\x00")),
		slices.Concat(desc64, []byte("prov\x00\x00\x00")),
	} {
		_, err := parseNoteDesc(bad, 8, le)
		assert.Error(t, err, "%q", bad)
	}
}
