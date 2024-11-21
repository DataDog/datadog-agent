// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	compression "github.com/DataDog/datadog-agent/comp/serializer/compression/def"
)

// Compressor wraps the compression component.
// (TODO: This may not be needed)
type Compressor struct {
	compression compression.Component
}

// NewCompressor creates a new Compressor.
func NewCompressor(compression compression.Component) *Compressor {
	return &Compressor{
		compression: compression,
	}
}

func (c *Compressor) name() string {
	return c.compression.ContentEncoding()
}

func (c *Compressor) encode(payload []byte) ([]byte, error) {
	return c.compression.Compress(payload)
}
