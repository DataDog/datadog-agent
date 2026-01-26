// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && !no_rust_compression

package rustimpl

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZstdCompressInto(t *testing.T) {
	comp := NewZstd(3)
	defer comp.(*RustCompressor).Close()

	original := []byte("Hello, World! This is a test of zstd compression.")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	require.Greater(t, written, 0)
	require.LessOrEqual(t, written, bound)
}

func TestGzipCompressInto(t *testing.T) {
	comp := NewGzip(6)
	defer comp.(*RustCompressor).Close()

	original := []byte("Hello, World! This is a test of gzip compression.")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	require.Greater(t, written, 0)
	require.LessOrEqual(t, written, bound)
}

func TestZlibCompressInto(t *testing.T) {
	comp := NewZlib(6)
	defer comp.(*RustCompressor).Close()

	original := []byte("Hello, World! This is a test of zlib compression.")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	require.Greater(t, written, 0)
	require.LessOrEqual(t, written, bound)
}

func TestNoopCompressInto(t *testing.T) {
	comp := NewNoop()
	defer comp.(*RustCompressor).Close()

	original := []byte("Hello, World! This is a test of noop compression.")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	assert.Equal(t, len(original), written)
	assert.Equal(t, original, dst[:written])
}

func TestEmptyInput(t *testing.T) {
	comp := NewZstd(3)
	defer comp.(*RustCompressor).Close()

	dst := make([]byte, 100)
	written, err := comp.CompressInto([]byte{}, dst)
	require.NoError(t, err)
	assert.Equal(t, 0, written)
}

func TestLargeInput(t *testing.T) {
	comp := NewZstd(3)
	defer comp.(*RustCompressor).Close()

	// Create 1MB of compressible data
	original := bytes.Repeat([]byte("ABCDEFGHIJ"), 100000)

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	require.Greater(t, written, 0)

	// Should compress well since it's repetitive
	assert.Less(t, written, len(original)/10)
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

			// Verify compressed output is non-empty
			compressed := output.Bytes()
			require.NotEmpty(t, compressed)
		})
	}
}

func TestCompressIntoAllAlgorithms(t *testing.T) {
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

func TestCompressIntoConsistency(t *testing.T) {
	// Verify that CompressInto produces consistent output
	tests := []struct {
		name    string
		newComp func() *RustCompressor
	}{
		{"zstd", func() *RustCompressor { return NewZstd(3).(*RustCompressor) }},
		{"zlib", func() *RustCompressor { return NewZlib(6).(*RustCompressor) }},
		{"noop", func() *RustCompressor { return NewNoop().(*RustCompressor) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp := tc.newComp()
			defer comp.Close()

			original := []byte("Hello, World! This is a test to verify CompressInto is consistent.")

			bound := comp.CompressBound(len(original))
			dst1 := make([]byte, bound)
			dst2 := make([]byte, bound)

			written1, err := comp.CompressInto(original, dst1)
			require.NoError(t, err)

			written2, err := comp.CompressInto(original, dst2)
			require.NoError(t, err)

			// For deterministic algorithms, output should be identical
			// Note: gzip includes timestamps so may differ slightly
			assert.Equal(t, written1, written2)
			assert.Equal(t, dst1[:written1], dst2[:written2])
		})
	}
}

// ==================== Thread Safety Tests ====================

func TestConcurrentCompression(t *testing.T) {
	comp := NewZstd(3).(*RustCompressor)
	defer comp.Close()

	original := []byte("Hello, World! This is a test of concurrent compression.")
	numGoroutines := 10
	numIterations := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines*numIterations)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				bound := comp.CompressBound(len(original))
				dst := make([]byte, bound)
				written, err := comp.CompressInto(original, dst)
				if err != nil {
					errors <- err
					continue
				}
				if written <= 0 {
					errors <- assert.AnError
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent compression error: %v", err)
	}
}

func TestConcurrentCompressors(t *testing.T) {
	// Test that multiple compressors can be used concurrently
	numGoroutines := 5
	numIterations := 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errors := make(chan error, numGoroutines*numIterations)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			// Each goroutine has its own compressor
			comp := NewZstd(3).(*RustCompressor)
			defer comp.Close()

			original := []byte("Hello from goroutine! Testing separate compressors.")

			for j := 0; j < numIterations; j++ {
				bound := comp.CompressBound(len(original))
				dst := make([]byte, bound)
				written, err := comp.CompressInto(original, dst)
				if err != nil {
					errors <- err
					continue
				}
				if written <= 0 {
					errors <- assert.AnError
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent compressors error: %v", err)
	}
}

// ==================== Finalizer Tests ====================

func TestFinalizerCleanup(t *testing.T) {
	// This test verifies that the finalizer can clean up resources
	// when Close() is not called. We can't directly test the finalizer
	// runs, but we can verify it doesn't crash.

	for i := 0; i < 100; i++ {
		// Create compressors without calling Close()
		_ = NewZstd(3)
		_ = NewGzip(6)
		_ = NewZlib(6)
	}

	// Force GC to potentially trigger finalizers
	// (Note: finalizers are not guaranteed to run immediately)
}

func TestDoubleClose(t *testing.T) {
	comp := NewZstd(3).(*RustCompressor)

	// Close twice should not panic
	comp.Close()
	comp.Close()

	// Operations after close should return errors
	_, err := comp.CompressInto([]byte("test"), make([]byte, 100))
	assert.Equal(t, ErrInvalidHandle, err)
}
