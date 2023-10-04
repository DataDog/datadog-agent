// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zstd

package compression

import (
	zstd_0 "github.com/DataDog/zstd_0"
)

// TODO: the intake still uses a pre-v1 (unstable) version of the zstd compression format.
// The agent shouldn't use zstd compression until the intake supports a stable v1 format.

// ContentEncoding describes the HTTP header value associated with the compression method
// var instead of const to ease testing
var ContentEncoding = "zstd"

// Compress will compress the data with zstd
func Compress(src []byte) ([]byte, error) {
	return zstd_0.Compress(nil, src)
}

// Decompress will decompress the data with zstd
func Decompress(src []byte) ([]byte, error) {
	return zstd_0.Decompress(nil, src)
}

// CompressBound returns the worst case size needed for a destination buffer
func CompressBound(sourceLen int) int {
	return zstd_0.CompressBound(sourceLen)
}
