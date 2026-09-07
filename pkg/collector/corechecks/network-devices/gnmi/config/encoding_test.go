// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultEncoding(t *testing.T) {
	require.Equal(t, Encoding(4), encodingJSONIETF)
	require.Equal(t, encodingJSONIETF, DefaultEncoding)
}

func TestParseEncoding(t *testing.T) {
	tests := []struct {
		input    string
		expected Encoding
	}{
		{"", encodingJSONIETF},
		{"json_ietf", encodingJSONIETF},
		{"json-ietf", encodingJSONIETF},
		{"json", encodingJSON},
		{"proto", encodingProto},
	}

	for _, test := range tests {
		encoding, err := ParseEncoding(test.input)
		require.NoError(t, err, test.input)
		assert.Equal(t, test.expected, encoding, test.input)
	}

	_, err := ParseEncoding("unsupported")
	require.Error(t, err)
}

func TestEncodingName(t *testing.T) {
	assert.Equal(t, "json_ietf", EncodingName(encodingJSONIETF))
	assert.Equal(t, "proto", EncodingName(encodingProto))
}
