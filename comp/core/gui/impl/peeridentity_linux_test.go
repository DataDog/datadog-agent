// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package guiimpl

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// procNetTCPHeader is the header line every /proc/net/tcp{,6} file starts with; searchProcNetTCP discards it unconditionally.
const procNetTCPHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode"

var (
	loopbackV4 = net.ParseIP("127.0.0.1")
	loopbackV6 = net.ParseIP("::1")
)

func writeFixture(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tcp")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	return path
}

// encodeProcNetAddr encodes ip the way /proc/net/tcp{,6} does (byte-swapped 32-bit words); the inverse of hexAddrPort's decoding, used to build fixtures instead of hand-computing hex strings.
func encodeProcNetAddr(ip net.IP) string {
	raw := []byte(ip.To4())
	if raw == nil {
		raw = []byte(ip.To16())
	}
	buf := make([]byte, len(raw))
	for word := 0; word < len(raw); word += 4 {
		buf[word], buf[word+1], buf[word+2], buf[word+3] = raw[word+3], raw[word+2], raw[word+1], raw[word]
	}
	return strings.ToUpper(hex.EncodeToString(buf))
}

// procNetTCPLine fabricates a well-formed /proc/net/tcp{,6} row for a connection between localAddr:localPort and remoteAddr:remotePort, owned by uid.
func procNetTCPLine(localAddr net.IP, localPort int, remoteAddr net.IP, remotePort int, uid int) string {
	return fmt.Sprintf("   0: %s:%04X %s:%04X 01 00000000:00000000 00:00000000 00000000  %d        0 12345 1 0000000000000000 20 4 30 10 -1\n",
		encodeProcNetAddr(localAddr), localPort, encodeProcNetAddr(remoteAddr), remotePort, uid)
}

func TestHexAddrPort(t *testing.T) {
	t.Run("valid IPv4 address:port", func(t *testing.T) {
		addr, port, err := hexAddrPort("0100007F:1F90")
		require.NoError(t, err)
		assert.Equal(t, 8080, port)
		assert.True(t, loopbackV4.Equal(addr), "got %s", addr)
	})

	t.Run("valid IPv6 address:port", func(t *testing.T) {
		addr, port, err := hexAddrPort(encodeProcNetAddr(loopbackV6) + ":1F90")
		require.NoError(t, err)
		assert.Equal(t, 8080, port)
		assert.True(t, loopbackV6.Equal(addr), "got %s", addr)
	})

	t.Run("valid IPv6 address:port, against a literal computed independently of encodeProcNetAddr", func(t *testing.T) {
		// Unlike the subtest above, this hex string is an independent oracle, not produced by encodeProcNetAddr, so a symmetric bug shared by both can't hide a real decoding bug.
		addr, port, err := hexAddrPort("00000000000000000000000001000000:1F90")
		require.NoError(t, err)
		assert.Equal(t, 8080, port)
		assert.True(t, loopbackV6.Equal(addr), "got %s", addr)
	})

	t.Run("missing colon", func(t *testing.T) {
		_, _, err := hexAddrPort("0100007F")
		assert.Error(t, err)
	})

	t.Run("non-hex port", func(t *testing.T) {
		_, _, err := hexAddrPort("0100007F:ZZZZ")
		assert.Error(t, err)
	})

	t.Run("non-hex address", func(t *testing.T) {
		_, _, err := hexAddrPort("ZZZZZZZZ:1F90")
		assert.Error(t, err)
	})

	t.Run("address length not a multiple of 4 bytes", func(t *testing.T) {
		_, _, err := hexAddrPort("0102:1F90")
		assert.Error(t, err)
	})
}

func TestLookupLoopbackPeerIdentity(t *testing.T) {
	origFiles := procNetTCPFiles
	t.Cleanup(func() { procNetTCPFiles = origFiles })

	t.Run("matches a connection in the first file", func(t *testing.T) {
		path := writeFixture(t, procNetTCPHeader+"\n"+procNetTCPLine(loopbackV4, 8080, loopbackV4, 80, 1000))
		procNetTCPFiles = []string{path, "/definitely/does/not/exist"}

		id, err := lookupLoopbackPeerIdentity(loopbackV4, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("1000"), id)
	})

	t.Run("falls through to the second file when the first has no match", func(t *testing.T) {
		tcp4 := writeFixture(t, procNetTCPHeader+"\n")
		tcp6 := writeFixture(t, procNetTCPHeader+"\n"+procNetTCPLine(loopbackV6, 8080, loopbackV6, 80, 2000))
		procNetTCPFiles = []string{tcp4, tcp6}

		id, err := lookupLoopbackPeerIdentity(loopbackV6, 80, 8080, loopbackV6)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("2000"), id)
	})

	t.Run("a missing table file is skipped, not fatal", func(t *testing.T) {
		tcp6 := writeFixture(t, procNetTCPHeader+"\n"+procNetTCPLine(loopbackV6, 8080, loopbackV6, 80, 3000))
		procNetTCPFiles = []string{"/definitely/does/not/exist", tcp6}

		id, err := lookupLoopbackPeerIdentity(loopbackV6, 80, 8080, loopbackV6)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("3000"), id)
	})

	t.Run("no matching connection is an error", func(t *testing.T) {
		path := writeFixture(t, procNetTCPHeader+"\n"+procNetTCPLine(loopbackV4, 8080, loopbackV4, 80, 1000))
		procNetTCPFiles = []string{path}

		_, err := lookupLoopbackPeerIdentity(loopbackV4, 9999, 9999, loopbackV4)
		assert.Error(t, err)
	})

	t.Run("malformed lines are skipped without aborting the scan", func(t *testing.T) {
		path := writeFixture(t, procNetTCPHeader+"\n"+
			"   0: garbage\n"+
			"   1: 0100007F:1F90 0100007F:0050 01 00000000:00000000 00:00000000 00000000  notanumber        0 12345 1 0000000000000000 20 4 30 10 -1\n"+
			procNetTCPLine(loopbackV4, 8080, loopbackV4, 80, 4000))
		procNetTCPFiles = []string{path}

		id, err := lookupLoopbackPeerIdentity(loopbackV4, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("4000"), id)
	})

	t.Run("does not confuse connections that share a port pair across address families", func(t *testing.T) {
		// Regression test: a naive port-only match would return whichever row is found first, regardless of address family.
		tcp4 := writeFixture(t, procNetTCPHeader+"\n"+procNetTCPLine(loopbackV4, 8080, loopbackV4, 80, 4000))
		tcp6 := writeFixture(t, procNetTCPHeader+"\n"+procNetTCPLine(loopbackV6, 8080, loopbackV6, 80, 6000))
		procNetTCPFiles = []string{tcp4, tcp6}

		id, err := lookupLoopbackPeerIdentity(loopbackV4, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("4000"), id)

		id, err = lookupLoopbackPeerIdentity(loopbackV6, 80, 8080, loopbackV6)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("6000"), id)
	})

	t.Run("does not confuse connections that share a port pair across distinct server addresses", func(t *testing.T) {
		// Regression test: matching on ports and client address alone isn't enough; only the row actually made to serverAddr (127.0.0.1 vs .2) must match.
		loopbackV4Alt := net.ParseIP("127.0.0.2")
		path := writeFixture(t, procNetTCPHeader+"\n"+
			procNetTCPLine(loopbackV4, 8080, loopbackV4, 80, 4000)+
			procNetTCPLine(loopbackV4, 8080, loopbackV4Alt, 80, 5000))
		procNetTCPFiles = []string{path}

		id, err := lookupLoopbackPeerIdentity(loopbackV4, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("4000"), id)

		id, err = lookupLoopbackPeerIdentity(loopbackV4Alt, 80, 8080, loopbackV4)
		require.NoError(t, err)
		assert.Equal(t, peerIdentity("5000"), id)
	})
}
