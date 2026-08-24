// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package opener

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestDirectIOFileProvidesLogicalUnalignedReads(t *testing.T) {
	content := make([]byte, directIOAlignment*3+137)
	for i := range content {
		content[i] = byte(i % 251)
	}

	path := filepath.Join(t.TempDir(), "direct.log")
	require.NoError(t, os.WriteFile(path, content, 0600))
	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	direct := newDirectIOFile(file)
	_, err = direct.Seek(37, io.SeekStart)
	require.NoError(t, err)

	buffer := make([]byte, 2048)
	n, err := io.ReadFull(direct, buffer)
	require.NoError(t, err)
	require.Equal(t, len(buffer), n)
	require.Equal(t, content[37:37+len(buffer)], buffer)
	require.NotEmpty(t, direct.aligned)
	require.Zero(t, uintptr(unsafe.Pointer(&direct.aligned[0]))%uintptr(direct.memoryAlignment))

	firstScratch := uintptr(unsafe.Pointer(&direct.aligned[0]))
	small := make([]byte, 31)
	n, err = direct.ReadAt(small, 100)
	require.NoError(t, err)
	require.Equal(t, len(small), n)
	require.Equal(t, content[100:100+len(small)], small)
	require.Equal(t, firstScratch, uintptr(unsafe.Pointer(&direct.aligned[0])), "smaller reads should reuse the aligned buffer")

	offset, err := direct.Seek(-10, io.SeekEnd)
	require.NoError(t, err)
	require.Equal(t, int64(len(content)-10), offset)
	tail, err := io.ReadAll(direct)
	require.NoError(t, err)
	require.Equal(t, content[len(content)-10:], tail)
}

// TestDirectIOFileWriteToUsesAlignedReadPath guards against the embedded
// *os.File's WriteTo being promoted: io.Copy prefers io.WriterTo over Read, so a
// promoted WriteTo would read the descriptor directly and bypass alignment. The
// file size is deliberately not a multiple of the alignment, and the copy starts
// from a non-aligned offset, so any bypass would surface as wrong bytes.
func TestDirectIOFileWriteToUsesAlignedReadPath(t *testing.T) {
	content := make([]byte, directIOAlignment*2+321)
	for i := range content {
		content[i] = byte(i%251 + 1)
	}
	path := filepath.Join(t.TempDir(), "writeto.log")
	require.NoError(t, os.WriteFile(path, content, 0600))
	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	direct := newDirectIOFile(file)

	// Seek only moves the wrapper's logical offset, never the descriptor's, so a
	// promoted WriteTo would copy from 0 and return the whole file instead.
	const start = 77
	_, err = direct.Seek(start, io.SeekStart)
	require.NoError(t, err)

	var sink bytes.Buffer
	copied, err := io.Copy(&sink, direct)
	require.NoError(t, err)
	require.Equal(t, int64(len(content)-start), copied)
	require.Equal(t, content[start:], sink.Bytes())
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

func TestDirectIOFileUsesFilesystemAlignments(t *testing.T) {
	content := make([]byte, 8192)
	for i := range content {
		content[i] = byte(i % 251)
	}
	path := filepath.Join(t.TempDir(), "alignment.log")
	require.NoError(t, os.WriteFile(path, content, 0600))
	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	direct := newDirectIOFileWithAlignments(file, 512, 2048)
	got := make([]byte, 2049)
	n, err := direct.ReadAt(got, 2047)
	require.NoError(t, err)
	require.Equal(t, len(got), n)
	require.Equal(t, content[2047:4096], got)
	require.Zero(t, uintptr(unsafe.Pointer(&direct.aligned[0]))%512)
	require.Zero(t, len(direct.aligned)%2048)
}

func TestDirectIOFileDoesNotCopyAlignmentPaddingAtEOF(t *testing.T) {
	for _, size := range []int{511, 512, 777, 2047, 2048, 2049, 4095, 4096, 4097} {
		t.Run(fmt.Sprintf("size_%d", size), func(t *testing.T) {
			content := make([]byte, size)
			for i := range content {
				content[i] = byte(i%251 + 1)
			}

			path := filepath.Join(t.TempDir(), "short.log")
			require.NoError(t, os.WriteFile(path, content, 0600))
			file, err := os.Open(path)
			require.NoError(t, err)
			t.Cleanup(func() { _ = file.Close() })

			direct := newDirectIOFile(file)
			buffer := make([]byte, size+257)
			for i := range buffer {
				buffer[i] = 0xcc
			}

			n, err := io.ReadFull(direct, buffer)
			require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			require.Equal(t, size, n)
			require.Equal(t, content, buffer[:n])
			for _, value := range buffer[n:] {
				require.Equal(t, byte(0xcc), value, "alignment padding must not escape the wrapper")
			}
		})
	}
}
