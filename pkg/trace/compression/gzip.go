// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package compression

import (
	"io"

	compressiondef "github.com/DataDog/datadog-agent/comp/trace/compression/def"

	"compress/gzip"
)

type gzipCompressor struct{}

// NewComponent creates a new compression component
func NewGZIPCompressor() compressiondef.Component {
	return &gzipCompressor{}
}

func (c *gzipCompressor) NewWriter(w io.Writer) (io.WriteCloser, error) {
	return gzip.NewWriterLevel(w, gzip.BestSpeed)
}

func (c *gzipCompressor) NewReader(w io.Reader) (io.ReadCloser, error) {
	return gzip.NewReader(w)
}
