// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func testElfPath(t *testing.T) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	// It doesn't matter which architecture we are on, so just open arm64
	// always.
	return filepath.Join(curDir, "..", "..", "usm", "testdata",
		"site-packages", "ddtrace", "libssl.so.arm64")
}

func openTestElf(t *testing.T) *safeelf.File {
	elfFile, err := safeelf.Open(testElfPath(t))
	require.NoError(t, err)

	return elfFile
}

// zeroPaddedReaderAt stands in for a sparse file.
type zeroPaddedReaderAt struct {
	data []byte
	size int64
}

func (r *zeroPaddedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= r.size {
		return 0, io.EOF
	}
	end := min(off+int64(len(p)), r.size)
	clear(p[:end-off])
	if off < int64(len(r.data)) {
		copy(p, r.data[off:min(end, int64(len(r.data)))])
	}
	n := int(end - off)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func testElfWithDynsym(t *testing.T, offset, size func(origOffset, origSize uint64) uint64) []byte {
	data, err := os.ReadFile(testElfPath(t))
	require.NoError(t, err)

	elfFile, err := safeelf.NewFile(bytes.NewReader(data))
	require.NoError(t, err)
	dynsym := elfFile.SectionByType(safeelf.SHT_DYNSYM)
	require.NotNil(t, dynsym)
	idx := -1
	for i, section := range elfFile.Sections {
		if section == dynsym {
			idx = i
		}
	}
	require.GreaterOrEqual(t, idx, 0)

	le := binary.LittleEndian
	shoff := le.Uint64(data[0x28:])
	shentsize := uint64(le.Uint16(data[0x3a:]))
	header := data[shoff+uint64(idx)*shentsize:]
	le.PutUint64(header[24:], offset(dynsym.Offset, dynsym.Size))
	le.PutUint64(header[32:], size(dynsym.Offset, dynsym.Size))
	return data
}

func scanDynsym(t *testing.T, r io.ReaderAt) ([]safeelf.Symbol, error) {
	elfFile, err := safeelf.NewFile(r)
	require.NoError(t, err)
	// The missing symbol forces a scan of the whole section.
	filter := newStringSetSymbolFilter(common.StringSet{"SSL_connect": {}, "SSL_notthere": {}})
	return getSymbols(elfFile, safeelf.SHT_DYNSYM, filter)
}

func keepOffset(origOffset, _ uint64) uint64 { return origOffset }

func TestSymbolSectionBeyondFileEnd(t *testing.T) {
	data := testElfWithDynsym(t, keepOffset, func(_, origSize uint64) uint64 { return origSize })
	symbols, err := scanDynsym(t, bytes.NewReader(data))
	require.NoError(t, err)
	require.Len(t, symbols, 1)
	require.Equal(t, "SSL_connect", symbols[0].Name)

	data = testElfWithDynsym(t, keepOffset, func(_, origSize uint64) uint64 {
		return origSize + 1000*safeelf.Sym64Size
	})
	_, err = scanDynsym(t, bytes.NewReader(data))
	require.ErrorContains(t, err, "truncated")
}

func TestSymbolSectionEntryLimit(t *testing.T) {
	scanZeros := func(numEntries uint64) error {
		size := numEntries * safeelf.Sym64Size
		data, err := os.ReadFile(testElfPath(t))
		require.NoError(t, err)
		start := uint64(len(data))
		data = testElfWithDynsym(t,
			func(_, _ uint64) uint64 { return start },
			func(_, _ uint64) uint64 { return size })
		_, err = scanDynsym(t, &zeroPaddedReaderAt{data: data, size: int64(start + size)})
		return err
	}

	require.NoError(t, scanZeros(maxSymbolEntries))
	require.ErrorContains(t, scanZeros(maxSymbolEntries+1), "more than the maximum")
	require.ErrorContains(t, scanZeros(1<<56), "more than the maximum")
}

func TestAllFound(t *testing.T) {
	elfFile := openTestElf(t)
	// The test ELF currently only has dynamic symbols.
	allSymbols, err := elfFile.DynamicSymbols()
	require.NoError(t, err)
	require.NotEmpty(t, allSymbols)

	symbolSet := make(common.StringSet, len(allSymbols))
	for _, sym := range allSymbols {
		symbolSet[sym.Name] = struct{}{}
	}

	symbols, err := GetAllSymbolsInSetByName(elfFile, symbolSet)
	require.NoError(t, err)
	for sym := range symbolSet {
		require.Contains(t, symbols, sym)
		require.Equal(t, symbols[sym].Name, sym)
	}
}

func TestAllMissing(t *testing.T) {
	elfFile := openTestElf(t)
	symbolSet := common.StringSet{
		"SSL_connect_not": {},
		"foo":             {},
	}

	_, err := GetAllSymbolsInSetByName(elfFile, symbolSet)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSymbolsNotFound)
	msg := err.Error()
	assert.Contains(t, msg, "SSL_connect_not")
	assert.Contains(t, msg, "foo")
}

func TestSomeMissing(t *testing.T) {
	elfFile := openTestElf(t)
	symbolSet := common.StringSet{
		"SSL_connect":  {},
		"SSL_invalid":  {},
		"SSL_set_bio":  {},
		"SSL_notthere": {},
	}

	_, err := GetAllSymbolsInSetByName(elfFile, symbolSet)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrSymbolsNotFound)
	msg := err.Error()
	assert.Contains(t, msg, "SSL_invalid")
	assert.Contains(t, msg, "SSL_notthere")
	assert.NotContains(t, msg, "SSL_connect")
	assert.NotContains(t, msg, "SSL_set_bio")
}

func TestInfix(t *testing.T) {
	elfFile := openTestElf(t)

	symbol, err := GetAnySymbolWithInfix(elfFile, "_read_", len("SSL_read_e"), len("SSL_read_ex"))
	require.NoError(t, err)
	require.NotNil(t, symbol)
	require.Equal(t, "SSL_read_ex", symbol.Name)

	symbol, err = GetAnySymbolWithInfix(elfFile, "SSL_read_ex", len("SSL_read_ex"), len("SSL_read_ex"))
	require.NoError(t, err)
	require.NotNil(t, symbol)
	require.Equal(t, "SSL_read_ex", symbol.Name)

	symbol, err = GetAnySymbolWithInfix(elfFile, "read_e", len("SSL_read_e"), len("SSL_read_ex")-1)
	require.Error(t, err)
	require.Nil(t, symbol)
	msg := err.Error()
	assert.Contains(t, msg, "read_e")

	symbol, err = GetAnySymbolWithInfix(elfFile, "^foo", 3, 5)
	require.Error(t, err)
	require.Nil(t, symbol)
	msg = err.Error()
	assert.Contains(t, msg, "foo")

	symbol, err = GetAnySymbolWithInfix(elfFile, "S", 1, len("SSL_connect"))
	require.NoError(t, err)
	require.NotNil(t, symbol)
	require.Equal(t, "SSL_connect", symbol.Name)
}

func TestDuplicateSymbolNames(t *testing.T) {
	data, err := os.ReadFile(testElfPath(t))
	require.NoError(t, err)
	elfFile, err := safeelf.NewFile(bytes.NewReader(data))
	require.NoError(t, err)
	dynsym := elfFile.SectionByType(safeelf.SHT_DYNSYM)
	require.NotNil(t, dynsym)
	syms, err := elfFile.DynamicSymbols()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(syms), 3)

	// Name every entry but the last after the first one.
	// DynamicSymbols skips the null entry, so syms[i] is table entry i+1.
	le := binary.LittleEndian
	entry := func(i int) []byte { return data[dynsym.Offset+uint64(i+1)*safeelf.Sym64Size:] }
	first, last := syms[0], syms[len(syms)-1]
	for i := 1; i < len(syms)-1; i++ {
		le.PutUint32(entry(i), le.Uint32(entry(0)))
	}
	elfFile, err = safeelf.NewFile(bytes.NewReader(data))
	require.NoError(t, err)

	filter := newStringSetSymbolFilter(common.StringSet{first.Name: {}, last.Name: {}})
	symbols, err := getSymbols(elfFile, safeelf.SHT_DYNSYM, filter)
	require.NoError(t, err)
	require.Len(t, symbols, 2)
	require.Equal(t, first.Name, symbols[0].Name)
	require.Equal(t, first.Value, symbols[0].Value)
	require.Equal(t, last.Name, symbols[1].Name)

	filter = newStringSetSymbolFilter(common.StringSet{first.Name: {}, "SSL_notthere": {}})
	symbols, err = getSymbols(elfFile, safeelf.SHT_DYNSYM, filter)
	require.NoError(t, err)
	require.Len(t, symbols, 1)
	require.Equal(t, first.Name, symbols[0].Name)
}
