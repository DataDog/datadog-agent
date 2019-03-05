// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build zlib

package compression

import (
	"bytes"
	"compress/zlib"
	"io/ioutil"
)

// ContentEncoding describes the HTTP header value associated with the compression method
// var instead of const to ease testing
var ContentEncoding = "deflate"

// Compress will compress the data with zlib
func Compress(dst []byte, src []byte) ([]byte, error) {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, err := w.Write(src)
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	dst = b.Bytes()
	return dst, nil
}

// Decompress will decompress the data with zlib
func Decompress(dst []byte, src []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	dst, err = ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return dst, nil
}

//  CompressBound returns the worst case size needed for a destination buffer
func CompressBound(sourceLen int) int {
	// From https://code.woboq.org/gcc/zlib/compress.c.html#compressBound
	return sourceLen + (sourceLen >> 12) + (sourceLen >> 14) + (sourceLen >> 25) + 13
}
