// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"net/netip"
	"testing"

	"github.com/mdlayher/netlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func newIPTuple(srcIP, dstIP string, srcPort, dstPort uint16, proto uint8) ConTuple {
	_s := netip.MustParseAddr(srcIP)
	_d := netip.MustParseAddr(dstIP)
	return ConTuple{
		Src:   netip.AddrPortFrom(_s, srcPort),
		Dst:   netip.AddrPortFrom(_d, dstPort),
		Proto: proto,
	}
}

func TestEncodeConn(t *testing.T) {
	// orig_src=10.0.2.15:58472 orig_dst=2.2.2.2:5432 reply_src=1.1.1.1:5432 reply_dst=10.0.2.15:58472 proto=tcp(6)
	origin := newIPTuple("10.0.2.15", "2.2.2.2", 58472, 5432, uint8(unix.IPPROTO_TCP))
	reply := newIPTuple("1.1.1.1", "10.0.2.15", 5432, 58472, uint8(unix.IPPROTO_TCP))
	conn := Con{
		Origin: origin,
		Reply:  reply,
	}

	data, err := EncodeConn(&conn)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	decoder := NewDecoder()
	connections := decoder.DecodeAndReleaseEvent(Event{
		msgs: []netlink.Message{
			{
				Data: data,
			},
		},
	})
	require.Len(t, connections, 1)
	c := connections[0]

	assert.Equal(t, conn.Origin.Src, c.Origin.Src)
	assert.Equal(t, conn.Origin.Dst, c.Origin.Dst)
	assert.Equal(t, conn.Origin.Proto, c.Origin.Proto)

	assert.Equal(t, conn.Reply.Src, c.Reply.Src)
	assert.Equal(t, conn.Reply.Dst, c.Reply.Dst)
	assert.Equal(t, conn.Reply.Proto, c.Reply.Proto)

}
