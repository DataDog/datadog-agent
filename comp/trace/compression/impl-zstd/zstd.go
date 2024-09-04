// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package compressionimpl implements the compression component interface
package compressionimpl

import (
	"io"

	compression "github.com/DataDog/datadog-agent/comp/trace/compression/def"

	"github.com/DataDog/zstd"
)

const encoding = "zstd"

type compressor struct {
}

type writer struct {
	underlyingWriter io.Writer
}

func (w *writer) Write(data []byte) (n int, err error) {
	res, err := zstd.CompressLevel(nil, data, zstd.BestSpeed)
	if err != nil {
		return 0, err
	}
	n, err = w.underlyingWriter.Write(res)
	return n, err
}

func (w *writer) Close() (err error) {
	return nil
}

// NewComponent creates a new compression component
func NewComponent() compression.Component {
	return &compressor{}
}

func (c *compressor) NewWriter(w io.Writer) (io.WriteCloser, error) {
	return &writer{
		underlyingWriter: w,
	}, nil
}

func (c *compressor) NewReader(w io.Reader) (io.ReadCloser, error) {
	return zstd.NewReader(w), nil
}

func (c *compressor) Encoding() string {
	return encoding
}
