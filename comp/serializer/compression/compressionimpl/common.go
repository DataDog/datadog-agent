// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package compressionimpl provides a set of functions for compressing with zlib / zstd
package compressionimpl

// ZlibKind defines a const value for the zlib compressor
const ZlibKind = "zlib"

// ZstdKind  defines a const value for the zstd compressor
const ZstdKind = "zstd"

// NoneKind defines a const value for disabling compression
const NoneKind = "none"
