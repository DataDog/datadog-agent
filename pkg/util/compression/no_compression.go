// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !zlib && !zstd

package compression

// ContentEncoding describes the HTTP header value associated with the compression method
// empty here since there's no compression
// var instead of const to ease testing
var ContentEncoding = ""

// Compress will not compress anything
func Compress(src []byte) ([]byte, error) {
	return src, nil
}

// Decompress will not decompress anything
func Decompress(src []byte) ([]byte, error) {
	return src, nil
}

// CompressBound returns the worst case size needed for a destination buffer
func CompressBound(sourceLen int) int {
	return sourceLen
}
