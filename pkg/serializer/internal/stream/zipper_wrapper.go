// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build zlib

package stream

import (
	"bytes"
	"compress/zlib"
	"errors"
	"io"

	"github.com/DataDog/zstd"
)

// Zipper is the interface that all types of zippers (zlib, zstd etc) should implement
type Zipper interface {
	Write([]byte) (int, error)
	Close() error
	Flush()
}

// ZipperWrapper is a wrapper around zlib and zstd
type ZipperWrapper struct {
	kind       string
	zlibWriter *zlib.Writer
	zstdWriter *zstd.Writer
}

var _ Zipper = &ZipperWrapper{}

// NewZipperWrapper returns a new instance of ZipperWrapper
func NewZipperWrapper(kind string) (*ZipperWrapper, error) {
	switch kind {
	case "zlib":
		return &ZipperWrapper{kind: kind}, nil
	case "zstd":
		return &ZipperWrapper{kind: kind}, nil
	}
	return nil, errors.New("invalid zipper kind. choose 'zlib' or 'zstd'")
}

// SetWriter creates the approriate writer for ZipperWrapper based on kind
func (z *ZipperWrapper) SetWriter(output *bytes.Buffer) {
	switch z.kind {
	case "zlib":
		z.zlibWriter = zlib.NewWriter(output)
	case "zstd":
		z.zstdWriter = zstd.NewWriter(output)
	}
}

// Write uses the appropriate zipper to write based on kind
func (z *ZipperWrapper) Write(b []byte) (int, error) {
	switch z.kind {
	case "zlib":
		return z.zlibWriter.Write(b)
	case "zstd":
		return z.zstdWriter.Write(b)
	}
	return 0, nil
}

// NewZipperReader returns the appropriate reader based on kind
func (z *ZipperWrapper) NewZipperReader(r io.Reader) (io.ReadCloser, error) {
	switch z.kind {
	case "zlib":
		return zlib.NewReader(r)
	case "zstd":
		return zstd.NewReader(r), nil
	}
	return nil, errors.New("invalid zipper kind. choose 'zlib' or 'zstd'")
}

// Flush uses the appropriate writer based on kind to flush
func (z *ZipperWrapper) Flush() {
	switch z.kind {
	case "zlib":
		z.zlibWriter.Flush()
	case "zstd":
		z.zstdWriter.Flush()
	}
}

// Close uses the appropriate writer based on kind to close
func (z *ZipperWrapper) Close() error {
	switch z.kind {
	case "zlib":
		return z.zlibWriter.Close()
	case "zstd":
		return z.zstdWriter.Close()
	}
	return errors.New("invalid zipper kind. choose 'zlib' or 'zstd'")
}
