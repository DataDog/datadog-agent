// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLocalhost(t *testing.T) {

	tests := []struct {
		key      Key
		expected bool
	}{
		// the isLocalhost function checks only the srcip, but set them both
		{
			key: Key{
				KeyTuple: KeyTuple{
					SrcIPHigh: 0,
					SrcIPLow:  uint64(binary.LittleEndian.Uint32([]uint8{127, 0, 0, 1})),
					DstIPHigh: 0,
					DstIPLow:  uint64(binary.LittleEndian.Uint32([]uint8{127, 0, 0, 1})),
				},
			},
			expected: true,
		},
		{
			key: Key{
				KeyTuple: KeyTuple{
					SrcIPHigh: 0,
					SrcIPLow:  uint64(binary.LittleEndian.Uint32([]uint8{192, 168, 1, 1})),
					DstIPHigh: 0,
					DstIPLow:  uint64(binary.LittleEndian.Uint32([]uint8{192, 168, 1, 1})),
				},
			},
			expected: false,
		},
		{
			key: Key{
				KeyTuple: KeyTuple{
					SrcIPHigh: 0,
					SrcIPLow:  binary.LittleEndian.Uint64([]uint8{0, 0, 0, 0, 0, 0, 0, 1}),
					DstIPHigh: 0,
					DstIPLow:  binary.LittleEndian.Uint64([]uint8{0, 0, 0, 0, 0, 0, 0, 1}),
				},
			},
			expected: true,
		},
		{
			key: Key{
				KeyTuple: KeyTuple{
					SrcIPHigh: binary.LittleEndian.Uint64([]uint8{0xf, 0xe, 0x8, 0, 0, 0, 0, 0}),
					SrcIPLow:  binary.LittleEndian.Uint64([]uint8{0x1, 0x9, 0x3, 0xe, 0x4, 0xc, 0xd, 0x6, 0xf, 0xf, 0xa, 0x4}),
					DstIPHigh: binary.LittleEndian.Uint64([]uint8{0xf, 0xe, 0x8, 0, 0, 0, 0, 0}),
					DstIPLow:  binary.LittleEndian.Uint64([]uint8{0x1, 0x9, 0x3, 0xe, 0x4, 0xc, 0xd, 0x6, 0xf, 0xf, 0xa, 0x4}),
				},
			},
			expected: false,
		},
	}
	for idx, tt := range tests {
		is := isLocalhost(tt.key)
		assert.Equal(t, tt.expected, is, "Unexpected result %v for test %v", is, idx)
	}

}
