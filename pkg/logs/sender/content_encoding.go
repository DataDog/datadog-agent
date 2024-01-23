// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"bytes"
	"compress/gzip"
)

// ContentEncoding encodes the payload
type ContentEncoding interface {
	name() string
	encode(payload []byte) ([]byte, error)
}

// IdentityContentType encodes the payload using the identity function
var IdentityContentType ContentEncoding = &identityContentType{}

type identityContentType struct{}

func (c *identityContentType) name() string {
	panic("not called")
}

func (c *identityContentType) encode(payload []byte) ([]byte, error) {
	panic("not called")
}

// GzipContentEncoding encodes the payload using gzip algorithm
type GzipContentEncoding struct {
	level int
}

// NewGzipContentEncoding creates a new Gzip content type
func NewGzipContentEncoding(level int) *GzipContentEncoding {
	if level < gzip.NoCompression {
		level = gzip.NoCompression
	} else if level > gzip.BestCompression {
		level = gzip.BestCompression
	}

	return &GzipContentEncoding{
		level,
	}
}

func (c *GzipContentEncoding) name() string {
	return "gzip"
}

func (c *GzipContentEncoding) encode(payload []byte) ([]byte, error) {
	var compressedPayload bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&compressedPayload, c.level)
	if err != nil {
		return nil, err
	}
	_, err = gzipWriter.Write(payload)
	if err != nil {
		return nil, err
	}
	err = gzipWriter.Flush()
	if err != nil {
		return nil, err
	}
	err = gzipWriter.Close()
	if err != nil {
		return nil, err
	}
	return compressedPayload.Bytes(), nil
}
