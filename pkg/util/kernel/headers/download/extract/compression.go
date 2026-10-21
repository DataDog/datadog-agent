// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package extract

import (
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/xi2/xz"

	"github.com/DataDog/datadog-agent/pkg/zstd"
)

// compressionFormat makes decoder selection explicit at archive call sites.
type compressionFormat uint8

const (
	compressionGzip compressionFormat = iota
	compressionZstd
	compressionBzip2
	compressionXZ
)

func openCompressed(reader io.Reader, format compressionFormat) (io.ReadCloser, error) {
	switch format {
	case compressionGzip:
		return gzip.NewReader(reader)
	case compressionZstd:
		return zstd.NewReader(reader)
	case compressionBzip2:
		return io.NopCloser(bzip2.NewReader(reader)), nil
	case compressionXZ:
		r, err := xz.NewReader(reader, 0)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(r), nil
	default:
		return nil, fmt.Errorf("unsupported compression format %d", format)
	}
}
