// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeToStringUTF16LE(t *testing.T) {
	cases := map[string]struct {
		data     []byte
		expected string
	}{
		"empty":          {data: []byte{}, expected: ""},
		"even length":    {data: []byte{'h', 0, 'i', 0}, expected: "hi"},
		"single byte":    {data: []byte{'h'}, expected: ""},
		"odd length":     {data: []byte{'h', 0, 'i', 0, '!'}, expected: "hi"},
		"surrogate pair": {data: []byte{0x3d, 0xd8, 0x00, 0xde}, expected: "\U0001F600"},
		"odd after pair": {data: []byte{0x3d, 0xd8, 0x00, 0xde, 0x41}, expected: "\U0001F600"},
	}
	for _, encoding := range []string{"utf16le", "ucs2", "ucs-2"} {
		for name, tc := range cases {
			t.Run(encoding+"/"+name, func(t *testing.T) {
				got, err := encodeToString(tc.data, encoding)
				require.NoError(t, err)
				assert.Equal(t, tc.expected, got)
			})
		}
	}
}

func TestParseResponseBodyUTF16LEOddLength(t *testing.T) {
	data := []byte{'h', 0, 'i', 0, '!'}
	var got interface{}
	var err error
	require.NotPanics(t, func() {
		got, err = ParseResponseBody("text/plain", data, Raw, "utf16le", 200)
	})
	require.NoError(t, err)
	assert.Equal(t, "hi", got)
}
