/* SPDX-License-Identifier: BSD-2-Clause */

package net

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIPv4MarshalBinary(t *testing.T) {
	want := []byte{
		0x47,  // Ver, IHL
		0x00,  // differentiated services
		0, 28, // total len
		0x12, 0x34, // ID
		0x40, 0x00, // don't fragment, frag offset = 0
		0xa,        // TTL
		0x11,       // Proto, UDP
		0x00, 0x00, // Checksum
		192, 168, 10, 1, // src
		8, 8, 8, 8, // dst
		0xa, 0xb, 0xc, 0xd, // opt 1
		0x1, 0x2, 0x3, 0x4, // opt 2
	}
	iph := IPv4{
		Version:   4,
		HeaderLen: 7,
		TotalLen:  28,
		ID:        0x1234,
		Flags:     DontFragment,
		TTL:       10,
		Proto:     ProtoUDP,
		Src:       net.ParseIP("192.168.10.1"),
		Dst:       net.ParseIP("8.8.8.8"),
		Options: []Option{
			{0xa, 0xb, 0xc, 0xd},
			{0x1, 0x2, 0x3, 0x4},
		},
	}
	b, err := iph.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, want, b)
}

func TestIPv4MarshalBinaryPayload(t *testing.T) {
	want := []byte{
		0x47,  // Ver, IHL
		0x00,  // differentiated services
		0, 32, // total len
		0x12, 0x34, // ID
		0x40, 0x00, // don't fragment, frag offset = 0
		0xa,        // TTL
		0x11,       // Proto, UDP
		0x00, 0x00, // Checksum
		192, 168, 10, 1, // src
		8, 8, 8, 8, // dst
		0xa, 0xb, 0xc, 0xd, // opt 1
		0x1, 0x2, 0x3, 0x4, // opt 2
		0xde, 0xad, 0xc0, 0xde, // payload
	}
	iph := IPv4{
		Version:   4,
		HeaderLen: 7,
		TotalLen:  28,
		ID:        0x1234,
		Flags:     DontFragment,
		TTL:       10,
		Proto:     ProtoUDP,
		Src:       net.ParseIP("192.168.10.1"),
		Dst:       net.ParseIP("8.8.8.8"),
		Options: []Option{
			{0xa, 0xb, 0xc, 0xd},
			{0x1, 0x2, 0x3, 0x4},
		},
	}
	iph.SetNext(&Raw{Data: []byte{0xde, 0xad, 0xc0, 0xde}})
	b, err := iph.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, want, b)
}

func TestIPv4Unmarshal(t *testing.T) {
	data := []byte{
		0x47,  // Ver, IHL
		0x00,  // DSCP, ECN
		0, 36, // total len
		0x12, 0x34, // ID
		0x40, 0x00, // don't fragment, frag offset = 0
		0xa,        // TTL
		0x11,       // Proto, UDP
		0x43, 0x21, // Checksum
		192, 168, 10, 1, // src
		8, 8, 8, 8, // dst
		0xa, 0xb, 0xc, 0xd, // opt 1
		0x1, 0x2, 0x3, 0x4, // opt 2
		// UDP
		0x30, 0x39, // src port 1235
		0x01, 0xbb, // dst port 443
		0x00, 0x00, // len
		0x00, 0x00, // checksum

	}
	var ip IPv4
	err := ip.Unmarshal(data)
	require.NoError(t, err)
	require.Equal(t, Version4, ip.Version)
	require.Equal(t, 0, ip.DiffServ)
	require.Equal(t, 36, ip.TotalLen)
	require.Equal(t, 0x1234, ip.ID)
	require.Equal(t, DontFragment, ip.Flags)
	require.Equal(t, 0, ip.FragOff)
	require.Equal(t, 10, ip.TTL)
	require.Equal(t, ProtoUDP, ip.Proto)
	require.Equal(t, 0x4321, ip.Checksum)
	require.Equal(t, net.IP{192, 168, 10, 1}, ip.Src)
	require.Equal(t, net.IP{8, 8, 8, 8}, ip.Dst)
	require.Equal(t, 2, len(ip.Options))
	require.Equal(t, Option{0xa, 0xb, 0xc, 0xd}, ip.Options[0])
	require.Equal(t, Option{0x1, 0x2, 0x3, 0x4}, ip.Options[1])
}
