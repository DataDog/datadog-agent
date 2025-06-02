// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package gzipimpl implements the compression component interface
package gzipimpl

import (
	"compress/gzip"
	"io"

	compression "github.com/DataDog/datadog-agent/comp/trace/compression/def"
)

const encoding = "gzip"

type compressor struct{}

// NewComponent creates a new compression component
func NewComponent() compression.Component {
	return &compressor{}
}

func (c *compressor) NewWriter(w io.Writer) (io.WriteCloser, error) {
	return gzip.NewWriterLevel(w, gzip.BestSpeed)
}

func (c *compressor) NewReader(w io.Reader) (io.ReadCloser, error) {
	return gzip.NewReader(w)
}

func (c *compressor) Encoding() string {
	return encoding
}
