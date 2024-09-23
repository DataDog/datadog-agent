// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !zlib && !zstd

// Package compressionimpl provides a set of functions for compressing with zlib / zstd
package compressionimpl

import (
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl/strategy"
)

// NewCompressor returns a new Compressor based on serializer_compressor_kind
// This function is called only when the zlib build tag is included
func (*CompressorFactory) NewCompressor(_kind string, _level int, _option string, _valid []string) compression.Component {
	return strategy.NewNoopStrategy()
}
