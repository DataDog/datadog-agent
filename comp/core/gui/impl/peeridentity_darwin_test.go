// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package guiimpl

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	loopbackV4 = net.ParseIP("127.0.0.1")
	loopbackV6 = net.ParseIP("::1")
)

// buildHeaderRecord fabricates the leading xinpgen header record; findUIDInPCBList only cares about its self-declared length, not its content.
func buildHeaderRecord(size int) []byte {
	rec := make([]byte, size)
	binary.LittleEndian.PutUint32(rec, uint32(size))
	return rec
}

// buildPCBRecord fabricates a well-formed xtcpcb64 record with the given ports, addresses, and owning UID at their real, SDK-derived offsets.
func buildPCBRecord(localPort, remotePort uint16, localAddr, remoteAddr net.IP, uid uint32) []byte {
	rec := make([]byte, xtcpcb64RecordSize)
	binary.LittleEndian.PutUint32(rec, uint32(xtcpcb64RecordSize))
	binary.BigEndian.PutUint16(rec[xtcpcb64FportOffset:], remotePort)
	binary.BigEndian.PutUint16(rec[xtcpcb64LportOffset:], localPort)
	binary.LittleEndian.PutUint32(rec[xtcpcb64SoUIDOffset:], uid)
	if v4 := localAddr.To4(); v4 != nil {
		rec[xtcpcb64VflagOffset] |= inpIPv4
		copy(rec[xtcpcb64Laddr4Offset:xtcpcb64Laddr4Offset+4], v4)
	} else {
		rec[xtcpcb64VflagOffset] |= inpIPv6
		copy(rec[xtcpcb64Laddr6Offset:xtcpcb64Laddr6Offset+16], localAddr.To16())
	}
	if v4 := remoteAddr.To4(); v4 != nil {
		rec[xtcpcb64VflagOffset] |= inpIPv4
		copy(rec[xtcpcb64Faddr4Offset:xtcpcb64Faddr4Offset+4], v4)
	} else {
		rec[xtcpcb64VflagOffset] |= inpIPv6
		copy(rec[xtcpcb64Faddr6Offset:xtcpcb64Faddr6Offset+16], remoteAddr.To16())
	}
	return rec
}

// buildDualStackPCBRecord fabricates a record with both INP_IPV4 and INP_IPV6 set in inp_vflag, as real xnu does for a dual-stack (::) listener accepting an IPv4 peer (address stored IPv4-mapped in the IPv6 fields).
func buildDualStackPCBRecord(localPort, remotePort uint16, localAddrV4, remoteAddrV4 net.IP, uid uint32) []byte {
	rec := buildPCBRecord(localPort, remotePort, localAddrV4, remoteAddrV4, uid)
	rec[xtcpcb64VflagOffset] = inpIPv4 | inpIPv6
	mappedLocal := localAddrV4.To4()
	mappedRemote := remoteAddrV4.To4()
	// Overwrite the (now-irrelevant) v4 slots with the IPv4-mapped form in the v6 slots, mirroring xnu's actual on-the-wire representation.
	copy(rec[xtcpcb64Laddr6Offset:xtcpcb64Laddr6Offset+16], net.IPv4(mappedLocal[0], mappedLocal[1], mappedLocal[2], mappedLocal[3]).To16())
	copy(rec[xtcpcb64Faddr6Offset:xtcpcb64Faddr6Offset+16], net.IPv4(mappedRemote[0], mappedRemote[1], mappedRemote[2], mappedRemote[3]).To16())
	return rec
}

func TestFindUIDInPCBList(t *testing.T) {
	t.Run("matches the single PCB record", func(t *testing.T) {
		buf := append(buildHeaderRecord(24), buildPCBRecord(8080, 80, loopbackV4, loopbackV4, 1000)...)

		id, err := findUIDInPCBList(buf, loopbackV4, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("1000"), id)
	})

	t.Run("matches the second of several PCB records", func(t *testing.T) {
		buf := buildHeaderRecord(24)
		buf = append(buf, buildPCBRecord(1234, 443, loopbackV4, loopbackV4, 5000)...)
		buf = append(buf, buildPCBRecord(8080, 80, loopbackV4, loopbackV4, 2000)...)
		buf = append(buf, buildPCBRecord(9999, 22, loopbackV4, loopbackV4, 6000)...)

		id, err := findUIDInPCBList(buf, loopbackV4, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("2000"), id)
	})

	t.Run("no matching connection is an error", func(t *testing.T) {
		buf := append(buildHeaderRecord(24), buildPCBRecord(1234, 443, loopbackV4, loopbackV4, 5000)...)

		_, err := findUIDInPCBList(buf, loopbackV4, 80, 8080, loopbackV4)
		assert.Error(t, err)
	})

	t.Run("a record with an unexpected size is skipped, not misread", func(t *testing.T) {
		// An IPv4-only xtcpcb record (shorter layout) can appear in real kernel output; it must be skipped wholesale, not reinterpreted as a v6 record.
		oddSizedRecord := make([]byte, 128)
		binary.LittleEndian.PutUint32(oddSizedRecord, uint32(len(oddSizedRecord)))

		buf := buildHeaderRecord(24)
		buf = append(buf, oddSizedRecord...)
		buf = append(buf, buildPCBRecord(8080, 80, loopbackV4, loopbackV4, 3000)...)

		id, err := findUIDInPCBList(buf, loopbackV4, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("3000"), id)
	})

	t.Run("a record claiming to extend past the buffer stops the scan safely", func(t *testing.T) {
		buf := buildHeaderRecord(24)
		truncated := buildPCBRecord(8080, 80, loopbackV4, loopbackV4, 4000)[:100]
		binary.LittleEndian.PutUint32(truncated, uint32(xtcpcb64RecordSize)) // claims full size, but buffer is short
		buf = append(buf, truncated...)

		_, err := findUIDInPCBList(buf, loopbackV4, 80, 8080, loopbackV4)
		assert.Error(t, err)
	})

	t.Run("a non-positive length record stops the scan safely", func(t *testing.T) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint32(buf, 0)

		_, err := findUIDInPCBList(buf, loopbackV4, 80, 8080, loopbackV4)
		assert.Error(t, err)
	})

	t.Run("an empty buffer is an error, not a panic", func(t *testing.T) {
		_, err := findUIDInPCBList(nil, loopbackV4, 80, 8080, loopbackV4)
		assert.Error(t, err)
	})

	t.Run("does not confuse connections that share a port pair across address families", func(t *testing.T) {
		// Regression test: a naive port-only match would return whichever record is found first, regardless of address family.
		buf := buildHeaderRecord(24)
		buf = append(buf, buildPCBRecord(8080, 80, loopbackV4, loopbackV4, 4000)...)
		buf = append(buf, buildPCBRecord(8080, 80, loopbackV6, loopbackV6, 6000)...)

		id, err := findUIDInPCBList(buf, loopbackV4, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("4000"), id)

		id, err = findUIDInPCBList(buf, loopbackV6, 80, 8080, loopbackV6)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("6000"), id)
	})

	t.Run("does not confuse connections that share a port pair across distinct server addresses", func(t *testing.T) {
		// Regression test: matching on ports and client address alone isn't enough; only the record actually made to serverAddr (127.0.0.1 vs .2) must match.
		loopbackV4Alt := net.ParseIP("127.0.0.2")
		buf := buildHeaderRecord(24)
		buf = append(buf, buildPCBRecord(8080, 80, loopbackV4, loopbackV4, 4000)...)
		buf = append(buf, buildPCBRecord(8080, 80, loopbackV4, loopbackV4Alt, 5000)...)

		id, err := findUIDInPCBList(buf, loopbackV4, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("4000"), id)

		id, err = findUIDInPCBList(buf, loopbackV4Alt, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("5000"), id)
	})

	t.Run("matches a dual-stack record (both INP_IPV4 and INP_IPV6 set)", func(t *testing.T) {
		// Regression test: a real xnu dual-stack listener sets both address-family flags and stores an IPv4-mapped address; local/foreign use distinct addresses so a field mix-up would fail, not pass vacuously.
		loopbackV4Alt := net.ParseIP("127.0.0.2")
		buf := append(buildHeaderRecord(24), buildDualStackPCBRecord(8080, 80, loopbackV4, loopbackV4Alt, 7000)...)

		id, err := findUIDInPCBList(buf, loopbackV4Alt, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("7000"), id)
	})
}

func TestConsoleUID(t *testing.T) {
	t.Run("resolves the owning UID of an existing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "console")
		require.NoError(t, os.WriteFile(path, nil, 0o644))
		orig := consoleDevicePath
		consoleDevicePath = path
		t.Cleanup(func() { consoleDevicePath = orig })

		uid, ok := consoleUID()
		require.True(t, ok)
		assert.Equal(t, uint32(os.Getuid()), uid)
	})

	t.Run("a missing device path is undeterminable", func(t *testing.T) {
		orig := consoleDevicePath
		consoleDevicePath = filepath.Join(t.TempDir(), "does-not-exist")
		t.Cleanup(func() { consoleDevicePath = orig })

		_, ok := consoleUID()
		assert.False(t, ok)
	})
}

func TestIdentityFromConsoleUID(t *testing.T) {
	t.Run("a resolved non-root UID is bound", func(t *testing.T) {
		assert.Equal(t, peerIdentity("1000"), identityFromConsoleUID(1000, true))
	})

	t.Run("an unresolved lookup stays unconstrained", func(t *testing.T) {
		assert.Empty(t, identityFromConsoleUID(0, false))
	})

	t.Run("UID 0 (nobody logged in, or genuinely root) stays unconstrained", func(t *testing.T) {
		assert.Empty(t, identityFromConsoleUID(0, true))
	})
}

func TestElevatedMintIdentity_Darwin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "console")
	require.NoError(t, os.WriteFile(path, nil, 0o644))
	orig := consoleDevicePath
	consoleDevicePath = path
	t.Cleanup(func() { consoleDevicePath = orig })

	want := identityFromConsoleUID(uint32(os.Getuid()), true)
	assert.Equal(t, want, elevatedMintIdentity())
}
