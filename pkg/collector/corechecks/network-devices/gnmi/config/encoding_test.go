// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"testing"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultEncoding(t *testing.T) {
	require.Equal(t, gnmipb.Encoding_JSON_IETF, DefaultEncoding)
}

func TestParseEncoding(t *testing.T) {
	tests := []struct {
		input    string
		expected gnmipb.Encoding
	}{
		{"", gnmipb.Encoding_JSON_IETF},
		{"json_ietf", gnmipb.Encoding_JSON_IETF},
		{"json-ietf", gnmipb.Encoding_JSON_IETF},
		{"json", gnmipb.Encoding_JSON},
		{"proto", gnmipb.Encoding_PROTO},
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
	assert.Equal(t, "json_ietf", EncodingName(gnmipb.Encoding_JSON_IETF))
	assert.Equal(t, "proto", EncodingName(gnmipb.Encoding_PROTO))
}
