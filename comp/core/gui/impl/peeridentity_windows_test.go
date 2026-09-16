// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package guiimpl

import (
	"encoding/binary"
	"math/bits"
	"net"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/cpu"
)

var (
	loopbackV4 = net.ParseIP("127.0.0.1")
	loopbackV6 = net.ParseIP("::1")
)

// portToField is the inverse of portFromField, used to build fixtures.
func portToField(port uint16) uint32 {
	if cpu.IsBigEndian {
		return uint32(port) << 16
	}
	return bits.ReverseBytes32(uint32(port) << 16)
}

// addrToV4Field is the inverse of addrFromV4Field, used to build fixtures.
func addrToV4Field(ip net.IP) uint32 {
	v4 := ip.To4()
	if cpu.IsBigEndian {
		return binary.BigEndian.Uint32(v4)
	}
	return binary.LittleEndian.Uint32(v4)
}

func v4Row(localPort, remotePort uint16, localAddr, remoteAddr net.IP, pid uint32) mibTCPRowOwnerPID {
	return mibTCPRowOwnerPID{
		localAddr:  addrToV4Field(localAddr),
		localPort:  portToField(localPort),
		remoteAddr: addrToV4Field(remoteAddr),
		remotePort: portToField(remotePort),
		pid:        pid,
	}
}

func v6Row(localPort, remotePort uint16, localAddr, remoteAddr net.IP, pid uint32) mibTCP6RowOwnerPID {
	var row mibTCP6RowOwnerPID
	copy(row.localAddr[:], localAddr.To16())
	row.localPort = portToField(localPort)
	copy(row.remoteAddr[:], remoteAddr.To16())
	row.remotePort = portToField(remotePort)
	row.pid = pid
	return row
}

// buildV4Table lays out rows exactly as GetExtendedTcpTable(AF_INET) would: a 4-byte entry count followed by that many mibTCPRowOwnerPID records.
func buildV4Table(rows []mibTCPRowOwnerPID) []byte {
	size := int(unsafe.Sizeof(mibTCPTableOwnerPID{}))
	if extra := len(rows) - 1; extra > 0 {
		size += extra * int(unsafe.Sizeof(mibTCPRowOwnerPID{}))
	}
	buf := make([]byte, size)
	info := (*mibTCPTableOwnerPID)(unsafe.Pointer(&buf[0]))
	info.numEntries = uint32(len(rows))
	if len(rows) > 0 {
		copy(unsafe.Slice(&info.table[0], len(rows)), rows)
	}
	return buf
}

// buildV6Table is buildV4Table's IPv6 counterpart.
func buildV6Table(rows []mibTCP6RowOwnerPID) []byte {
	size := int(unsafe.Sizeof(mibTCP6TableOwnerPID{}))
	if extra := len(rows) - 1; extra > 0 {
		size += extra * int(unsafe.Sizeof(mibTCP6RowOwnerPID{}))
	}
	buf := make([]byte, size)
	info := (*mibTCP6TableOwnerPID)(unsafe.Pointer(&buf[0]))
	info.numEntries = uint32(len(rows))
	if len(rows) > 0 {
		copy(unsafe.Slice(&info.table[0], len(rows)), rows)
	}
	return buf
}

func TestPortFromField(t *testing.T) {
	for _, port := range []uint16{1, 80, 8080, 65535} {
		assert.Equal(t, port, portFromField(portToField(port)))
	}
}

func TestAddrFromV4Field(t *testing.T) {
	assert.True(t, loopbackV4.Equal(addrFromV4Field(addrToV4Field(loopbackV4))))
}

func TestAddrFromV4Field_LiteralOracle(t *testing.T) {
	// Unlike the subtest above, this literal is an independent oracle, not produced by addrToV4Field, so a symmetric bug shared by both can't hide a real decoding bug; 127.0.0.1 is 0x0100007F little-endian, matching both of Windows's supported architectures (amd64, arm64).
	if cpu.IsBigEndian {
		t.Skip("this literal is little-endian-specific; Windows has no supported big-endian architecture")
	}
	assert.True(t, loopbackV4.Equal(addrFromV4Field(0x0100007F)))
}

func TestFindPIDInV4Table(t *testing.T) {
	t.Run("matches the row with the right ports and addresses", func(t *testing.T) {
		buf := buildV4Table([]mibTCPRowOwnerPID{
			v4Row(1234, 443, loopbackV4, loopbackV4, 111),
			v4Row(8080, 80, loopbackV4, loopbackV4, 222),
		})

		pid, ok := findPIDInV4Table(buf, 8080, 80, loopbackV4, loopbackV4)
		require.True(t, ok)
		assert.Equal(t, uint32(222), pid)
	})

	t.Run("no matching connection", func(t *testing.T) {
		buf := buildV4Table([]mibTCPRowOwnerPID{v4Row(1234, 443, loopbackV4, loopbackV4, 111)})

		_, ok := findPIDInV4Table(buf, 8080, 80, loopbackV4, loopbackV4)
		assert.False(t, ok)
	})

	t.Run("an empty buffer does not match, and does not panic", func(t *testing.T) {
		_, ok := findPIDInV4Table(nil, 8080, 80, loopbackV4, loopbackV4)
		assert.False(t, ok)
	})

	t.Run("does not confuse connections that share a port pair across distinct server addresses", func(t *testing.T) {
		// Regression test: matching on ports and client address alone isn't enough; only the row actually made to remoteAddr (127.0.0.1 vs .2) must match.
		loopbackV4Alt := net.ParseIP("127.0.0.2")
		buf := buildV4Table([]mibTCPRowOwnerPID{
			v4Row(8080, 80, loopbackV4, loopbackV4, 4000),
			v4Row(8080, 80, loopbackV4, loopbackV4Alt, 5000),
		})

		pid, ok := findPIDInV4Table(buf, 8080, 80, loopbackV4, loopbackV4)
		require.True(t, ok)
		assert.Equal(t, uint32(4000), pid)

		pid, ok = findPIDInV4Table(buf, 8080, 80, loopbackV4, loopbackV4Alt)
		require.True(t, ok)
		assert.Equal(t, uint32(5000), pid)
	})
}

func TestFindPIDInV6Table(t *testing.T) {
	t.Run("matches the row with the right ports and addresses", func(t *testing.T) {
		buf := buildV6Table([]mibTCP6RowOwnerPID{
			v6Row(1234, 443, loopbackV6, loopbackV6, 111),
			v6Row(8080, 80, loopbackV6, loopbackV6, 222),
		})

		pid, ok := findPIDInV6Table(buf, 8080, 80, loopbackV6, loopbackV6)
		require.True(t, ok)
		assert.Equal(t, uint32(222), pid)
	})

	t.Run("no matching connection", func(t *testing.T) {
		buf := buildV6Table([]mibTCP6RowOwnerPID{v6Row(1234, 443, loopbackV6, loopbackV6, 111)})

		_, ok := findPIDInV6Table(buf, 8080, 80, loopbackV6, loopbackV6)
		assert.False(t, ok)
	})

	t.Run("an empty buffer does not match, and does not panic", func(t *testing.T) {
		_, ok := findPIDInV6Table(nil, 8080, 80, loopbackV6, loopbackV6)
		assert.False(t, ok)
	})
}

func TestFindPID_DoesNotConfuseAddressFamilies(t *testing.T) {
	// Regression test: each finder must reject rows whose address family doesn't match, even though lookupLoopbackPeerIdentity already routes to the table matching peerAddr's own family in practice.
	v4Buf := buildV4Table([]mibTCPRowOwnerPID{v4Row(8080, 80, loopbackV4, loopbackV4, 4000)})
	v6Buf := buildV6Table([]mibTCP6RowOwnerPID{v6Row(8080, 80, loopbackV6, loopbackV6, 6000)})

	_, ok := findPIDInV4Table(v4Buf, 8080, 80, loopbackV6, loopbackV4)
	assert.False(t, ok, "must not match a v4 row against a v6 local address")

	_, ok = findPIDInV6Table(v6Buf, 8080, 80, loopbackV4, loopbackV6)
	assert.False(t, ok, "must not match a v6 row against a v4 local address")
}
