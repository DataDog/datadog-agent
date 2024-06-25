// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !zstd

package compression

import (
	"errors"
	"io"

	compressiondef "github.com/DataDog/datadog-agent/comp/trace/compression/def"
)

const ZstdAvailable = false

var ErrNotAvailable = errors.New("zstd not available. `zstd` build tag not used")

type zstdCompressor struct{}

// NewComponent creates a new compression component
func NewZstdCompressor() compressiondef.Component {
	return &zstdCompressor{}
}

func (c *zstdCompressor) NewWriter(w io.Writer) (io.WriteCloser, error) {
	return nil, ErrNotAvailable
}

func (c *zstdCompressor) NewReader(w io.Reader) (io.ReadCloser, error) {
	return nil, ErrNotAvailable
}
