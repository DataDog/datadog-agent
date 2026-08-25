// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !cgo

package zstd

import (
	"io"
	"os"
	"strconv"

	kzstd "github.com/klauspost/compress/zstd"
)

// nocgoEncoderOptions returns the klauspost encoder options for the given
// level. The ZSTD_NOCGO_CONCURRENCY and ZSTD_NOCGO_WINDOW environment
// variables can be used to tune concurrency and the window size; missing or
// invalid values fall back to sensible defaults.
func nocgoEncoderOptions(level int) []kzstd.EOption {
	conc, err := strconv.Atoi(os.Getenv("ZSTD_NOCGO_CONCURRENCY"))
	if err != nil {
		conc = 1
	}
	window, err := strconv.Atoi(os.Getenv("ZSTD_NOCGO_WINDOW"))
	if err != nil {
		window = 1 << 15
	}
	return []kzstd.EOption{
		kzstd.WithEncoderLevel(kzstd.EncoderLevelFromZstd(level)),
		kzstd.WithEncoderConcurrency(conc),
		kzstd.WithLowerEncoderMem(true),
		kzstd.WithWindowSize(window),
		// WithZeroFrames ensures empty input produces a valid zstd frame
		// instead of empty output, so the result can be decompressed by the
		// CGO backend. See https://github.com/klauspost/compress/pull/155.
		kzstd.WithZeroFrames(true),
	}
}

// CompressLevel compresses src at the given level using the pure-Go backend.
func CompressLevel(dst, src []byte, level int) ([]byte, error) {
	enc, err := kzstd.NewWriter(nil, nocgoEncoderOptions(level)...)
	if err != nil {
		return nil, err
	}
	return enc.EncodeAll(src, dst), nil
}

// Decompress decompresses src using the pure-Go backend.
func Decompress(dst, src []byte) ([]byte, error) {
	dec, err := kzstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	return dec.DecodeAll(src, dst)
}

// NewWriter returns a streaming zstd writer at DefaultCompression.
func NewWriter(w io.Writer) (Writer, error) {
	return NewWriterLevel(w, DefaultCompression)
}

// NewWriterLevel returns a streaming zstd writer at the given level.
func NewWriterLevel(w io.Writer, level int) (Writer, error) {
	return kzstd.NewWriter(w, nocgoEncoderOptions(level)...)
}

// NewReader returns a streaming zstd reader.
func NewReader(r io.Reader) (io.ReadCloser, error) {
	dec, err := kzstd.NewReader(r)
	if err != nil {
		return nil, err
	}
	return dec.IOReadCloser(), nil
}
