// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package http2

import (
	"bytes"
	"fmt"

	"golang.org/x/net/http2/hpack"
)

// NewHeadersFrameMessage creates a new HTTP2 data frame message with the given header fields.
func NewHeadersFrameMessage(headerFields []hpack.HeaderField) ([]byte, error) {
	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)

	for _, value := range headerFields {
		if err := enc.WriteField(value); err != nil {
			return nil, fmt.Errorf("error encoding field: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// ComposeMessage concatenates the given byte slices into a single byte slice.
func ComposeMessage(slices ...[]byte) []byte {
	var result []byte

	for _, s := range slices {
		result = append(result, s...)
	}

	return result
}
