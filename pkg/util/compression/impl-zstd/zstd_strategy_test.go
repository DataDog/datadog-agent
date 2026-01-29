// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package zstdimpl

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

func TestZstdCompressInto(t *testing.T) {
	comp := New(Requires{Level: 3})
	require.NotNil(t, comp)

	original := []byte("Hello, World! This is a test of zstd compression.")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	require.Greater(t, written, 0)
	require.LessOrEqual(t, written, bound)
}

func TestZstdCompressIntoEmptyInput(t *testing.T) {
	comp := New(Requires{Level: 3})
	require.NotNil(t, comp)

	dst := make([]byte, 100)
	written, err := comp.CompressInto([]byte{}, dst)
	require.NoError(t, err)
	assert.Equal(t, 0, written)
}

func TestZstdCompressIntoBufferTooSmall(t *testing.T) {
	comp := New(Requires{Level: 3})
	require.NotNil(t, comp)

	original := []byte("Hello, World! This is a test of buffer too small error.")

	// Allocate a buffer that's definitely too small
	dst := make([]byte, 1)

	// Should fail with buffer too small
	_, err := comp.CompressInto(original, dst)
	assert.Equal(t, compression.ErrBufferTooSmall, err)
}

func TestZstdCompressIntoLargeData(t *testing.T) {
	comp := New(Requires{Level: 3})
	require.NotNil(t, comp)

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

func TestZstdCompressBound(t *testing.T) {
	comp := New(Requires{Level: 3})
	require.NotNil(t, comp)

	bound := comp.CompressBound(1000)
	assert.Greater(t, bound, 0)
	assert.GreaterOrEqual(t, bound, 1000) // bound should be at least input size
}

func TestZstdContentEncoding(t *testing.T) {
	comp := New(Requires{Level: 3})
	require.NotNil(t, comp)

	assert.Equal(t, compression.ZstdEncoding, comp.ContentEncoding())
}

func TestZstdStreamCompressor(t *testing.T) {
	comp := New(Requires{Level: 3})
	require.NotNil(t, comp)

	var output bytes.Buffer
	stream := comp.NewStreamCompressor(&output)
	require.NotNil(t, stream)

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
}

func TestZstdBufferOwnership(t *testing.T) {
	// This test verifies that CompressInto writes to the provided buffer
	// and doesn't silently write to a reallocated buffer.
	comp := New(Requires{Level: 3})
	require.NotNil(t, comp)

	original := []byte("Test data for buffer ownership verification.")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	originalCap := cap(dst)

	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	require.Greater(t, written, 0)

	// Verify the buffer wasn't reallocated (same capacity)
	assert.Equal(t, originalCap, cap(dst))

	// Verify the data is actually in dst (not in some other buffer)
	assert.NotEqual(t, byte(0), dst[0], "First byte should be non-zero (compressed data)")
}
