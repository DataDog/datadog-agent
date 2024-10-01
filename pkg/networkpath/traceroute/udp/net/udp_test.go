// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Contains BSD-2-Clause code (c) 2015-present Andrea Barberio

package net

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUDPMarshalBinary(t *testing.T) {
	want := []byte{
		0x12, 0x34, // src port
		0x23, 0x45, // dst port
		0x00, 0x08, // len
		0x00, 0x00, // csum
	}
	udp := UDP{
		Src:  0x1234,
		Dst:  0x2345,
		Len:  8,
		Csum: 0,
	}
	b, err := udp.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, want, b)
}

// TODO: fix broken test
// func TestMarshalBinaryIPv4(t *testing.T) {
// 	want := []byte{
// 		0x12, 0x34, // src port
// 		0x23, 0x45, // dst port
// 		0x00, 0x08, // len
// 		0xef, 0xab, // csum
// 	}
// 	ip := IPv4{
// 		Src:   net.IP{192, 168, 10, 1},
// 		Dst:   net.IP{8, 8, 8, 8},
// 		Proto: ProtoUDP,
// 	}
// 	udp := UDP{
// 		Src:  0x1234,
// 		Dst:  0x2345,
// 		Len:  8,
// 		Csum: 0,
// 	}
// 	udp.SetPrev(&ip)
// 	b, err := udp.MarshalBinary()
// 	require.NoError(t, err)
// 	require.Equal(t, want, b)
// }

func TestUDPUnmarshalBinary(t *testing.T) {
	b := []byte{
		0x12, 0x34, // src port
		0x23, 0x45, // dst port
		0x00, 0x08, // len
		0xff, 0x35, // csum
	}
	var u UDP
	err := u.UnmarshalBinary(b)
	require.NoError(t, err)
	assert.Equal(t, uint16(0x1234), u.Src)
	assert.Equal(t, uint16(0x2345), u.Dst)
	assert.Equal(t, uint16(8), u.Len)
	assert.Equal(t, uint16(0xff35), u.Csum)
}
