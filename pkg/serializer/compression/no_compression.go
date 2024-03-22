// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !zlib && !zstd

// Package compression provides a set of functions for compressing with zlib / zstd
package compression

import (
	"bytes"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compression "github.com/DataDog/datadog-agent/pkg/serializer/compression/internal"
	"github.com/DataDog/datadog-agent/pkg/serializer/compression/utils"
)

// NewCompressorStrategy returns a new Compressor based on serializer_compressor_kind
func NewCompressorStrategy(cfg config.Component) utils.Compressor {
	return compression.NewNoopStrategy()
}

// NewStreamCompressor returns a Zipper to be used by the stream Compressor
func NewStreamCompressor(output *bytes.Buffer, cfg config.Component) utils.StreamCompressor {
	return compression.NewNoopStreamCompressor(output)
}
