// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && !no_rust_compression

package selector

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

func TestNewCompressorZstd(t *testing.T) {
	comp := NewCompressor(compression.ZstdKind, 3)
	require.NotNil(t, comp)

	original := []byte("Hello, World! This is a test of zstd compression via selector.")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	require.Greater(t, written, 0)

	assert.Equal(t, "zstd", comp.ContentEncoding())
}

func TestNewCompressorGzip(t *testing.T) {
	comp := NewCompressor(compression.GzipKind, 6)
	require.NotNil(t, comp)

	original := []byte("Hello, World! This is a test of gzip compression via selector.")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	require.Greater(t, written, 0)

	assert.Equal(t, "gzip", comp.ContentEncoding())
}

func TestNewCompressorZlib(t *testing.T) {
	comp := NewCompressor(compression.ZlibKind, 6)
	require.NotNil(t, comp)

	original := []byte("Hello, World! This is a test of zlib compression via selector.")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	require.Greater(t, written, 0)

	assert.Equal(t, "deflate", comp.ContentEncoding())
}

func TestNewCompressorNone(t *testing.T) {
	comp := NewCompressor(compression.NoneKind, 0)
	require.NotNil(t, comp)

	original := []byte("Hello, World! This is a test of noop compression via selector.")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	assert.Equal(t, len(original), written)
	assert.Equal(t, original, dst[:written])

	assert.Equal(t, "identity", comp.ContentEncoding())
}

func TestNewCompressorUnknown(t *testing.T) {
	// Unknown kind should fall back to noop
	comp := NewCompressor("unknown", 0)
	require.NotNil(t, comp)

	original := []byte("Hello, World!")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	assert.Equal(t, len(original), written)
	assert.Equal(t, original, dst[:written])

	assert.Equal(t, "identity", comp.ContentEncoding())
}

func TestNewNoopCompressor(t *testing.T) {
	comp := NewNoopCompressor()
	require.NotNil(t, comp)

	original := []byte("Hello, World!")

	bound := comp.CompressBound(len(original))
	dst := make([]byte, bound)
	written, err := comp.CompressInto(original, dst)
	require.NoError(t, err)
	assert.Equal(t, len(original), written)
	assert.Equal(t, original, dst[:written])
}

func TestStreamCompressorViaSelector(t *testing.T) {
	comp := NewCompressor(compression.ZstdKind, 3)
	require.NotNil(t, comp)

	var output bytes.Buffer
	stream := comp.NewStreamCompressor(&output)

	_, err := stream.Write([]byte("Hello, "))
	require.NoError(t, err)
	_, err = stream.Write([]byte("World!"))
	require.NoError(t, err)

	err = stream.Close()
	require.NoError(t, err)

	compressed := output.Bytes()
	require.NotEmpty(t, compressed)
}
