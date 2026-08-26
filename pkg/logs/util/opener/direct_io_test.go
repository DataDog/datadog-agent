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
	"runtime"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

// directIOChunkSize is the span one refill covers, and the threshold above which
// a read is served straight into a per-call buffer instead.
const directIOChunkSize = directIOAlignment * directIOChunkBlocks

// writeTestFile writes size bytes of recognisable content and returns both the
// path and the content, so a test can assert on the exact logical bytes.
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

func newTestReader(t *testing.T, path string) *directReader {
	t.Helper()
	reader, err := newDirectReader(openTestFile(t, path))
	require.NoError(t, err)
	return reader
}

func TestDirectReaderReadsWholeFile(t *testing.T) {
	path, content := writeTestFile(t, "whole.log", directIOChunkSize*2+137)

	got, err := io.ReadAll(newTestReader(t, path))
	require.NoError(t, err)
	require.Equal(t, content, got)
}

// TestDirectReaderSeekAndRead is the case fingerprinting actually exercises:
// seek to an arbitrary offset, then read an arbitrary count. Neither is block
// aligned, and the ranges below straddle block boundaries, chunk boundaries, and
// the threshold where a read bypasses the chunk buffer.
func TestDirectReaderSeekAndRead(t *testing.T) {
	path, content := writeTestFile(t, "ranges.log", directIOChunkSize*2+137)

	tests := []struct {
		name   string
		offset int64
		length int
	}{
		{name: "aligned start within chunk", offset: 0, length: 100},
		{name: "unaligned start within chunk", offset: 137, length: 100},
		{name: "spans block boundary", offset: directIOAlignment - 1, length: 3},
		{name: "spans chunk boundary", offset: directIOChunkSize - 10, length: 1000},
		{name: "exactly one chunk", offset: 0, length: directIOChunkSize},
		{name: "larger than chunk from unaligned start", offset: 137, length: directIOChunkSize + 999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newTestReader(t, path)

			position, err := reader.Seek(tt.offset, io.SeekStart)
			require.NoError(t, err)
			require.Equal(t, tt.offset, position)

			buffer := make([]byte, tt.length)
			read, err := io.ReadFull(reader, buffer)
			require.NoError(t, err)
			require.Equal(t, tt.length, read)
			require.Equal(t, content[tt.offset:tt.offset+int64(tt.length)], buffer)
		})
	}
}

// TestDirectReaderStopsAtEOF reads past the end across sizes that straddle
// alignment boundaries. The last aligned read runs past EOF, so a mishandled
// short read would either fail or leak zeroed padding into the result.
func TestDirectReaderStopsAtEOF(t *testing.T) {
	for _, size := range []int{0, 1, 511, 512, 777, 2047, 4096, 4097, 8193, directIOChunkSize + 1} {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			path, content := writeTestFile(t, "short.log", size)
			reader := newTestReader(t, path)

			got, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Len(t, got, size, "alignment padding must not escape the reader")
			require.Equal(t, content, got)

			// A read once already past the end must report EOF rather than
			// re-reading the trailing block forever.
			_, err = reader.Read(make([]byte, 16))
			require.ErrorIs(t, err, io.EOF)
		})
	}
}

// TestDirectReaderSeeksBackToStart covers the line strategy's fallback to a byte
// fingerprint, which rewinds and rereads the head of the file.
func TestDirectReaderSeeksBackToStart(t *testing.T) {
	path, content := writeTestFile(t, "rewind.log", directIOChunkSize*2)
	reader := newTestReader(t, path)

	head := make([]byte, 64)
	_, err := io.ReadFull(reader, head)
	require.NoError(t, err)

	_, err = reader.Seek(directIOChunkSize+500, io.SeekStart)
	require.NoError(t, err)
	middle := make([]byte, 64)
	_, err = io.ReadFull(reader, middle)
	require.NoError(t, err)
	require.Equal(t, content[directIOChunkSize+500:directIOChunkSize+564], middle)

	position, err := reader.Seek(0, io.SeekStart)
	require.NoError(t, err)
	require.Zero(t, position)

	again := make([]byte, 64)
	_, err = io.ReadFull(reader, again)
	require.NoError(t, err)
	require.Equal(t, head, again)
}

// TestDirectReaderDeepSeekStaysBounded is the regression test for a fingerprint
// configuration with a large count_to_skip. Reading a small range from deep in a
// file must cost roughly that range, not everything preceding it.
func TestDirectReaderDeepSeekStaysBounded(t *testing.T) {
	const fileSize = 64 << 20
	const skip = 60 << 20
	const count = 1024

	path := filepath.Join(t.TempDir(), "sparse.log")
	file, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(fileSize))
	require.NoError(t, file.Close())

	// The reader is built inside the measured window so the figure covers
	// everything one fingerprint costs, not just the final read.
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	reader := newTestReader(t, path)
	_, err = reader.Seek(skip, io.SeekStart)
	require.NoError(t, err)
	read, err := io.ReadFull(reader, make([]byte, count))
	require.NoError(t, err)
	require.Equal(t, count, read)

	runtime.ReadMemStats(&after)

	allocated := after.TotalAlloc - before.TotalAlloc
	require.Less(t, allocated, uint64(1<<20),
		"reading %d bytes at offset %d allocated %d bytes; reading the whole prefix would allocate about %d",
		count, skip, allocated, 2*skip)
}

// TestDirectReaderLargeReadStaysBounded covers a byte fingerprint configured
// with a count far larger than the read cap. Every requested byte must still
// come back, and the aligned buffer used to get there must not scale with the
// request, which would otherwise mean holding two copies of a large count.
func TestDirectReaderLargeReadStaysBounded(t *testing.T) {
	const size = directIOAlignment * directIOLargeReadBlocks * 4

	path, content := writeTestFile(t, "large.log", size)
	reader := newTestReader(t, path)
	buffer := make([]byte, size)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	read, err := io.ReadFull(reader, buffer)

	runtime.ReadMemStats(&after)

	require.NoError(t, err)
	require.Equal(t, size, read)
	require.Equal(t, content, buffer)

	allocated := after.TotalAlloc - before.TotalAlloc
	require.Less(t, allocated, uint64(size/2),
		"reading %d bytes allocated %d bytes of scratch; it must stay bounded by the read cap", size, allocated)
}

// TestDirectReaderHonoursFilesystemAlignments pins the behaviour for a
// filesystem reporting a memory alignment that differs from the offset
// alignment, which is what the block size query can return.
func TestDirectReaderHonoursFilesystemAlignments(t *testing.T) {
	path, content := writeTestFile(t, "alignment.log", 8192)

	reader, err := newDirectReaderWithAlignments(openTestFile(t, path), 512, 2048)
	require.NoError(t, err)

	_, err = reader.Seek(2049, io.SeekStart)
	require.NoError(t, err)
	buffer := make([]byte, 1000)
	_, err = io.ReadFull(reader, buffer)
	require.NoError(t, err)
	require.Equal(t, content[2049:3049], buffer)
}

func TestDirectReaderSeekWhence(t *testing.T) {
	path, content := writeTestFile(t, "whence.log", 4096)
	reader := newTestReader(t, path)

	position, err := reader.Seek(100, io.SeekStart)
	require.NoError(t, err)
	require.EqualValues(t, 100, position)

	position, err = reader.Seek(50, io.SeekCurrent)
	require.NoError(t, err)
	require.EqualValues(t, 150, position)

	position, err = reader.Seek(-96, io.SeekEnd)
	require.NoError(t, err)
	require.EqualValues(t, 4000, position)

	buffer := make([]byte, 96)
	_, err = io.ReadFull(reader, buffer)
	require.NoError(t, err)
	require.Equal(t, content[4000:], buffer)

	_, err = reader.Seek(-1, io.SeekStart)
	require.Error(t, err)
	_, err = reader.Seek(0, 42)
	require.Error(t, err)
}

func TestDirectReaderRejectsNilFile(t *testing.T) {
	_, err := newDirectReader(nil)
	require.ErrorIs(t, err, os.ErrInvalid)
}

func TestNewAlignedBuffer(t *testing.T) {
	for _, alignment := range []int{512, directIOAlignment, 64 * 1024} {
		t.Run(fmt.Sprintf("alignment_%d", alignment), func(t *testing.T) {
			buffer := newAlignedBuffer(3*alignment, alignment)
			require.Len(t, buffer, 3*alignment)
			require.Zero(t, uintptr(unsafe.Pointer(&buffer[0]))%uintptr(alignment))
		})
	}
}

func TestRoundUpToAlignment(t *testing.T) {
	value, ok := roundUpToAlignment(4097, directIOAlignment)
	require.True(t, ok)
	require.Equal(t, 8192, value)

	_, ok = roundUpToAlignment(-1, directIOAlignment)
	require.False(t, ok)
	_, ok = roundUpToAlignment(int(^uint(0)>>1), directIOAlignment)
	require.False(t, ok)
}
