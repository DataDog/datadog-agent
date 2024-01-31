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

// HeadersFrameOptions is used to hold the needed headers field and dynamic table update, if needed.
type HeadersFrameOptions struct {
	Headers                []hpack.HeaderField
	DynamicTableUpdateSize uint32
}

// NewHeadersFrameMessage creates a new HTTP2 data frame message with the given header fields.
func NewHeadersFrameMessage(frameOptions HeadersFrameOptions) ([]byte, error) {
	var buf bytes.Buffer
	enc := hpack.NewEncoder(&buf)

	if frameOptions.DynamicTableUpdateSize > 0 {
		// we set the max dynamic table size to 100 to be able to test different cases of literal header parsing.
		enc.SetMaxDynamicTableSizeLimit(frameOptions.DynamicTableUpdateSize)
	}

	for _, value := range frameOptions.Headers {
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
