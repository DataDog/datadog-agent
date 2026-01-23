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
