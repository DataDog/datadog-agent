// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && !no_rust_compression

package rustimpl

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZstdCompressDecompress(t *testing.T) {
	comp := NewZstd(3)
	defer comp.(*RustCompressor).Close()

	original := []byte("Hello, World! This is a test of zstd compression.")

	compressed, err := comp.Compress(original)
	require.NoError(t, err)
	require.NotEmpty(t, compressed)

	decompressed, err := comp.Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestGzipCompressDecompress(t *testing.T) {
	comp := NewGzip(6)
	defer comp.(*RustCompressor).Close()

	original := []byte("Hello, World! This is a test of gzip compression.")

	compressed, err := comp.Compress(original)
	require.NoError(t, err)
	require.NotEmpty(t, compressed)

	decompressed, err := comp.Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestZlibCompressDecompress(t *testing.T) {
	comp := NewZlib(6)
	defer comp.(*RustCompressor).Close()

	original := []byte("Hello, World! This is a test of zlib compression.")

	compressed, err := comp.Compress(original)
	require.NoError(t, err)
	require.NotEmpty(t, compressed)

	decompressed, err := comp.Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestNoopCompressDecompress(t *testing.T) {
	comp := NewNoop()
	defer comp.(*RustCompressor).Close()

	original := []byte("Hello, World! This is a test of noop compression.")

	compressed, err := comp.Compress(original)
	require.NoError(t, err)
	assert.Equal(t, original, compressed)

	decompressed, err := comp.Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestEmptyInput(t *testing.T) {
	comp := NewZstd(3)
	defer comp.(*RustCompressor).Close()

	compressed, err := comp.Compress([]byte{})
	require.NoError(t, err)
	assert.Empty(t, compressed)

	decompressed, err := comp.Decompress([]byte{})
	require.NoError(t, err)
	assert.Empty(t, decompressed)
}

func TestLargeInput(t *testing.T) {
	comp := NewZstd(3)
	defer comp.(*RustCompressor).Close()

	// Create 1MB of compressible data
	original := bytes.Repeat([]byte("ABCDEFGHIJ"), 100000)

	compressed, err := comp.Compress(original)
	require.NoError(t, err)
	require.NotEmpty(t, compressed)

	// Should compress well since it's repetitive
	assert.Less(t, len(compressed), len(original)/10)

	decompressed, err := comp.Decompress(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestCompressBound(t *testing.T) {
	tests := []struct {
		name    string
		newComp func() *RustCompressor
	}{
		{"zstd", func() *RustCompressor { return NewZstd(3).(*RustCompressor) }},
		{"gzip", func() *RustCompressor { return NewGzip(6).(*RustCompressor) }},
		{"zlib", func() *RustCompressor { return NewZlib(6).(*RustCompressor) }},
		{"noop", func() *RustCompressor { return NewNoop().(*RustCompressor) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp := tc.newComp()
			defer comp.Close()

			bound := comp.CompressBound(1000)
			assert.Greater(t, bound, 0)
		})
	}
}

func TestContentEncoding(t *testing.T) {
	tests := []struct {
		name     string
		newComp  func() *RustCompressor
		expected string
	}{
		{"zstd", func() *RustCompressor { return NewZstd(3).(*RustCompressor) }, "zstd"},
		{"gzip", func() *RustCompressor { return NewGzip(6).(*RustCompressor) }, "gzip"},
		{"zlib", func() *RustCompressor { return NewZlib(6).(*RustCompressor) }, "deflate"},
		{"noop", func() *RustCompressor { return NewNoop().(*RustCompressor) }, "identity"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp := tc.newComp()
			defer comp.Close()

			assert.Equal(t, tc.expected, comp.ContentEncoding())
		})
	}
}

func TestStreamCompressor(t *testing.T) {
	tests := []struct {
		name    string
		newComp func() *RustCompressor
	}{
		{"zstd", func() *RustCompressor { return NewZstd(3).(*RustCompressor) }},
		{"gzip", func() *RustCompressor { return NewGzip(6).(*RustCompressor) }},
		{"zlib", func() *RustCompressor { return NewZlib(6).(*RustCompressor) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp := tc.newComp()
			defer comp.Close()

			var output bytes.Buffer
			stream := comp.NewStreamCompressor(&output)

			// Write data in chunks
			chunks := [][]byte{
				[]byte("Hello, "),
				[]byte("World! "),
				[]byte("This is a test."),
			}

			for _, chunk := range chunks {
				n, err := stream.Write(chunk)
				require.NoError(t, err)
				assert.Equal(t, len(chunk), n)
			}

			err := stream.Flush()
			require.NoError(t, err)

			err = stream.Close()
			require.NoError(t, err)

			// Decompress and verify
			compressed := output.Bytes()
			require.NotEmpty(t, compressed)

			decompressed, err := comp.Decompress(compressed)
			require.NoError(t, err)

			expected := "Hello, World! This is a test."
			assert.Equal(t, expected, string(decompressed))
		})
	}
}

func TestCompressInto(t *testing.T) {
	tests := []struct {
		name    string
		newComp func() *RustCompressor
	}{
		{"zstd", func() *RustCompressor { return NewZstd(3).(*RustCompressor) }},
		{"gzip", func() *RustCompressor { return NewGzip(6).(*RustCompressor) }},
		{"zlib", func() *RustCompressor { return NewZlib(6).(*RustCompressor) }},
		{"noop", func() *RustCompressor { return NewNoop().(*RustCompressor) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp := tc.newComp()
			defer comp.Close()

			original := []byte("Hello, World! This is a test of CompressInto.")

			// Allocate buffer with compress bound
			bound := comp.CompressBound(len(original))
			dst := make([]byte, bound)

			// Compress directly into the buffer
			written, err := comp.CompressInto(original, dst)
			require.NoError(t, err)
			require.Greater(t, written, 0)
			require.LessOrEqual(t, written, bound)

			// Verify decompression works
			decompressed, err := comp.Decompress(dst[:written])
			require.NoError(t, err)
			assert.Equal(t, original, decompressed)
		})
	}
}

func TestCompressIntoLargeData(t *testing.T) {
	comp := NewZstd(3).(*RustCompressor)
	defer comp.Close()

	// Create 1MB of compressible data
	original := bytes.Repeat([]byte("ABCDEFGHIJ"), 100000)

	// Allocate buffer with compress bound
	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)

	// Compress directly into the buffer
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	require.Greater(t, written, 0)

	// Should compress well since it's repetitive
	assert.Less(t, written, len(original)/10)

	// Verify decompression works
	decompressed, err := comp.Decompress(dst[:written])
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestCompressIntoBufferTooSmall(t *testing.T) {
	comp := NewZstd(3).(*RustCompressor)
	defer comp.Close()

	original := []byte("Hello, World! This is a test of buffer too small error.")

	// Allocate a buffer that's definitely too small
	dst := make([]byte, 1)

	// Should fail with buffer too small
	_, err := comp.CompressInto(original, dst)
	assert.Equal(t, ErrBufferTooSmall, err)
}

func TestCompressIntoEmptyInput(t *testing.T) {
	comp := NewZstd(3).(*RustCompressor)
	defer comp.Close()

	dst := make([]byte, 100)

	// Empty input should return 0 bytes written
	written, err := comp.CompressInto([]byte{}, dst)
	require.NoError(t, err)
	assert.Equal(t, 0, written)
}

func TestCompressIntoEmptyBuffer(t *testing.T) {
	comp := NewZstd(3).(*RustCompressor)
	defer comp.Close()

	original := []byte("Hello, World!")

	// Empty buffer should fail
	_, err := comp.CompressInto(original, []byte{})
	assert.Equal(t, ErrBufferTooSmall, err)
}

func TestCompressIntoMatchesCompress(t *testing.T) {
	// Verify that CompressInto produces the same output as Compress
	tests := []struct {
		name    string
		newComp func() *RustCompressor
	}{
		{"zstd", func() *RustCompressor { return NewZstd(3).(*RustCompressor) }},
		{"gzip", func() *RustCompressor { return NewGzip(6).(*RustCompressor) }},
		{"zlib", func() *RustCompressor { return NewZlib(6).(*RustCompressor) }},
		{"noop", func() *RustCompressor { return NewNoop().(*RustCompressor) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp := tc.newComp()
			defer comp.Close()

			original := []byte("Hello, World! This is a test to verify CompressInto matches Compress.")

			// Compress using both methods
			compressed1, err := comp.Compress(original)
			require.NoError(t, err)

			bound := comp.CompressBound(len(original))
			dst := make([]byte, bound)
			written, err := comp.CompressInto(original, dst)
			require.NoError(t, err)

			compressed2 := dst[:written]

			// Both should decompress to the same data
			decompressed1, err := comp.Decompress(compressed1)
			require.NoError(t, err)

			decompressed2, err := comp.Decompress(compressed2)
			require.NoError(t, err)

			assert.Equal(t, original, decompressed1)
			assert.Equal(t, original, decompressed2)

			// For deterministic algorithms, output should be identical
			// Note: gzip includes timestamps so may differ slightly
			if tc.name != "gzip" {
				assert.Equal(t, compressed1, compressed2)
			}
		})
	}
}
