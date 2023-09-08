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

// Flusher is the interface that requires the Flush function
type Flusher interface {
	Flush() error
}

// Zipper is the interface that zlib and zstd should implement
type Zipper interface {
	io.WriteCloser
	Flusher
}

// NewZipperWriter returns a zipper of either zstd or zlib depending on the kind param
func NewZipperWriter(kind string, output *bytes.Buffer) (Zipper, error) {
	switch kind {
	case "zlib":
		return zlib.NewWriter(output), nil
	case "zstd":
		return zstd.NewWriter(output), nil
	}
	return nil, errors.New("invalid zipper kind. choose 'zlib' or 'zstd'")
}
