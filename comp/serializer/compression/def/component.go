// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package compression provides a compression implementation based on the configuration or available build tags.
package compression

import (
	"bytes"
	"io"
)

// team: agent-metrics-logs

// ZlibEncoding is the content-encoding value for Zlib
const ZlibEncoding = "deflate"

// ZstdEncoding is the content-encoding value for Zstd
const ZstdEncoding = "zstd"

// Component is the component type.
type Component interface {
	Compress(src []byte) ([]byte, error)
	Decompress(src []byte) ([]byte, error)
	CompressBound(sourceLen int) int
	ContentEncoding() string
	NewStreamCompressor(output *bytes.Buffer) StreamCompressor
}

// StreamCompressor is the interface that zlib and zstd should implement
type StreamCompressor interface {
	io.WriteCloser
	Flush() error
}
