// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && !no_rust_compression

// Package selector provides compressor selection based on build configuration.
// When cgo is available (default), this uses the Rust compression library.
// Use the no_rust_compression build tag to fall back to Go implementations.
package selector

import (
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	rustimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-rust"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewCompressor creates a new compressor using the Rust implementation.
func NewCompressor(kind string, level int) compression.Compressor {
	switch kind {
	case compression.ZlibKind:
		return rustimpl.NewZlib(level)
	case compression.ZstdKind:
		return rustimpl.NewZstd(level)
	case compression.GzipKind:
		return rustimpl.NewGzip(level)
	case compression.NoneKind:
		return rustimpl.NewNoop()
	default:
		log.Errorf("Unknown compression kind '%s', falling back to noop", kind)
		return rustimpl.NewNoop()
	}
}

// NewNoopCompressor creates a new no-op compressor using the Rust implementation.
func NewNoopCompressor() compression.Compressor {
	return rustimpl.NewNoop()
}
