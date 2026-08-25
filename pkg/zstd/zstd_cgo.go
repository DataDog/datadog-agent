// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo

package zstd

import (
	"io"

	ddzstd "github.com/DataDog/zstd"
)

// CompressLevel compresses src at the given level using the native libzstd.
func CompressLevel(dst, src []byte, level Level) ([]byte, error) {
	return ddzstd.CompressLevel(dst, src, int(level))
}

// Decompress decompresses src. If dst is large enough it is reused, otherwise
// a new buffer is allocated.
func Decompress(dst, src []byte) ([]byte, error) {
	return ddzstd.Decompress(dst, src)
}

// NewWriter returns a streaming zstd writer at DefaultCompression.
func NewWriter(w io.Writer) (Writer, error) {
	return ddzstd.NewWriter(w), nil
}

// NewWriterLevel returns a streaming zstd writer at the given level.
func NewWriterLevel(w io.Writer, level Level) (Writer, error) {
	return ddzstd.NewWriterLevel(w, int(level)), nil
}

// NewReader returns a streaming zstd reader.
func NewReader(r io.Reader) (io.ReadCloser, error) {
	return ddzstd.NewReader(r), nil
}
