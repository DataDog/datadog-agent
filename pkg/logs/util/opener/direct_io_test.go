// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package opener

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

const directIODefaultWindowSize = directIOAlignment * directIODefaultWindowBlocks

func writeTestFile(t *testing.T, name string, size int) (string, []byte) {
	t.Helper()
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i%251 + 1)
	}
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, content, 0600))
	return path, content
}

func openTestFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func newTestDirectReader(t *testing.T, path string, limit int) *directFingerprintReader {
	t.Helper()
	reader, err := openDirectFingerprintStream(openTestFile(t, path), limit)
	require.NoError(t, err)
	return reader
}

func TestReadDirectFingerprintRangeReturnsLogicalRange(t *testing.T) {
	path, content := writeTestFile(t, "ranges.log", directIODefaultWindowSize*2+137)

	tests := []struct {
		name  string
		skip  int
		count int
		want  []byte
	}{
		{name: "aligned start", skip: 0, count: 100, want: content[:100]},
		{name: "unaligned start", skip: 137, count: 100, want: content[137:237]},
		{name: "spans block boundary", skip: directIOAlignment - 1, count: 3, want: content[directIOAlignment-1 : directIOAlignment+2]},
		{name: "deep skip stays bounded", skip: directIODefaultWindowSize + 500, count: 64, want: content[directIODefaultWindowSize+500 : directIODefaultWindowSize+564]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := openTestFile(t, path)
			got, err := readDirectFingerprintRangeFromFile(file, tt.skip, tt.count, directIOAlignment, directIOAlignment)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestReadDirectFingerprintRangeStopsAtEOF(t *testing.T) {
	for _, size := range []int{0, 1, 511, 512, 777, 2047, 4096, 4097, 8193} {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			path, content := writeTestFile(t, "short.log", size)
			file := openTestFile(t, path)
			got, err := readDirectFingerprintRangeFromFile(file, 0, size+16, directIOAlignment, directIOAlignment)
			require.NoError(t, err)
			require.Equal(t, content, got)
		})
	}
}

func TestDirectFingerprintReaderReadsUpToLimit(t *testing.T) {
	path, content := writeTestFile(t, "limited.log", directIODefaultWindowSize*2)

	limit := 2061
	reader := newTestDirectReader(t, path, limit)
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, content[:limit], got)
	require.NoError(t, reader.Close())
}

func TestDirectFingerprintReaderStopsAtEOF(t *testing.T) {
	path, content := writeTestFile(t, "eof.log", 777)
	reader := newTestDirectReader(t, path, directIODefaultWindowSize*4)

	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, content, got)

	_, err = reader.Read(make([]byte, 16))
	require.ErrorIs(t, err, io.EOF)
}

func TestDirectFingerprintReaderLargeLimitStaysBounded(t *testing.T) {
	const size = directIOAlignment * directIOMaxWindowBlocks * 4
	path, content := writeTestFile(t, "large.log", size)
	reader := newTestDirectReader(t, path, size)

	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, content, got)
	require.LessOrEqual(t, len(reader.window), directIOAlignment*directIOMaxWindowBlocks)
}

func TestDirectFingerprintReaderHonoursFilesystemAlignments(t *testing.T) {
	path, content := writeTestFile(t, "alignment.log", 8192)

	reader, err := openDirectFingerprintStreamWithAlignments(openTestFile(t, path), 3049, 512, 2048)
	require.NoError(t, err)

	buffer := make([]byte, 1000)
	_, err = io.ReadFull(reader, buffer)
	require.NoError(t, err)
	require.Equal(t, content[:1000], buffer)
}

func TestDirectFingerprintReaderRejectsNilFile(t *testing.T) {
	_, err := openDirectFingerprintStream(nil, 1024)
	require.ErrorIs(t, err, os.ErrInvalid)
}

func TestDirectFingerprintReaderWindowAlignment(t *testing.T) {
	path, _ := writeTestFile(t, "aligned.log", directIOAlignment)

	for _, alignment := range []int{512, directIOAlignment, 64 * 1024} {
		t.Run(fmt.Sprintf("alignment_%d", alignment), func(t *testing.T) {
			reader, err := openDirectFingerprintStreamWithAlignments(openTestFile(t, path), directIOAlignment, alignment, directIOAlignment)
			require.NoError(t, err)
			require.Zero(t, uintptr(unsafe.Pointer(&reader.window[0]))%uintptr(alignment))

			previousLength := len(reader.window)
			require.NoError(t, reader.resizeWindow(directIOAlignment*directIOMaxWindowBlocks))
			require.Zero(t, uintptr(unsafe.Pointer(&reader.window[0]))%uintptr(alignment))
			require.Greater(t, len(reader.window), previousLength)

			require.NoError(t, reader.Close())
			require.Nil(t, reader.windowMapping)
		})
	}
}

func TestReadDirectFingerprintRangeRejectsInvalidArgs(t *testing.T) {
	path, _ := writeTestFile(t, "args.log", 16)
	_, err := readDirectFingerprintRangeFromFile(openTestFile(t, path), -1, 4, directIOAlignment, directIOAlignment)
	require.ErrorIs(t, err, os.ErrInvalid)
}
