// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package http

import (
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentityContentType(t *testing.T) {
	payload := []byte("my payload")

	encodedPayload, err := IdentityContentType.encode(payload)
	assert.Nil(t, err)

	assert.Equal(t, payload, encodedPayload)
}

func TestIdentityContentTypeName(t *testing.T) {
	assert.Equal(t, IdentityContentType.name(), "identity")
}

func TestGzipContentEncoding(t *testing.T) {
	payload := []byte("my payload")

	encodedPayload, err := NewGzipContentEncoding(gzip.BestCompression).encode(payload)
	assert.Nil(t, err)

	decompressedPayload, err := decompress(encodedPayload)
	assert.Nil(t, err)

	assert.Equal(t, payload, decompressedPayload)
}

func TestGzipContentEncodingName(t *testing.T) {
	assert.Equal(t, NewGzipContentEncoding(gzip.BestCompression).name(), "gzip")
}

func decompress(payload []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	_, err = buffer.ReadFrom(reader)
	if err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
