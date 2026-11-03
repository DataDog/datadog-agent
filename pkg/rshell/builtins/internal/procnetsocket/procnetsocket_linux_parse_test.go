// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// White-box unit tests for the Linux-specific parsing helpers in
// procnetsocket_linux.go. These exercise parseIPv4Proc, parseIPv6Proc,
// parsePortProc, and formatIPv6.

package procnetsocket

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- parseIPv4Proc ---

func TestParseIPv4ProcLoopback(t *testing.T) {
	// 0100007F = 127.0.0.1 little-endian
	ip, err := parseIPv4Proc("0100007F")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", ip)
}

func TestParseIPv4ProcAllZeros(t *testing.T) {
	ip, err := parseIPv4Proc("00000000")
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", ip)
}

func TestParseIPv4ProcBroadcast(t *testing.T) {
	// FFFFFFFF = 255.255.255.255
	ip, err := parseIPv4Proc("FFFFFFFF")
	require.NoError(t, err)
	assert.Equal(t, "255.255.255.255", ip)
}

func TestParseIPv4ProcLowerCase(t *testing.T) {
	// ParseUint handles lowercase hex.
	ip, err := parseIPv4Proc("0100007f")
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", ip)
}

func TestParseIPv4ProcTooShort(t *testing.T) {
	_, err := parseIPv4Proc("010000")
	assert.Error(t, err)
}

func TestParseIPv4ProcTooLong(t *testing.T) {
	_, err := parseIPv4Proc("0100007F00")
	assert.Error(t, err)
}

func TestParseIPv4ProcNonHex(t *testing.T) {
	_, err := parseIPv4Proc("ZZZZZZZZ")
	assert.Error(t, err)
}

func TestParseIPv4ProcEmpty(t *testing.T) {
	_, err := parseIPv4Proc("")
	assert.Error(t, err)
}

// --- parsePortProc ---

func TestParsePortProcCommon(t *testing.T) {
	// 0016 = port 22
	port, err := parsePortProc("0016")
	require.NoError(t, err)
	assert.Equal(t, "22", port)
}

func TestParsePortProcHTTPS(t *testing.T) {
	// 01BB = port 443
	port, err := parsePortProc("01BB")
	require.NoError(t, err)
	assert.Equal(t, "443", port)
}

func TestParsePortProcZero(t *testing.T) {
	port, err := parsePortProc("0000")
	require.NoError(t, err)
	assert.Equal(t, "0", port)
}

func TestParsePortProcMax(t *testing.T) {
	// FFFF = 65535
	port, err := parsePortProc("FFFF")
	require.NoError(t, err)
	assert.Equal(t, "65535", port)
}

func TestParsePortProcNonHex(t *testing.T) {
	_, err := parsePortProc("ZZZZ")
	assert.Error(t, err)
}

// --- parseIPv6Proc ---

func TestParseIPv6ProcLoopback(t *testing.T) {
	// ::1 in /proc/net/tcp6 format: 00000000000000000000000001000000
	// That's 4 little-endian uint32 groups:
	//   00000000, 00000000, 00000000, 01000000 (0x00000001 LE = bytes 01 00 00 00)
	ip, err := parseIPv6Proc("00000000000000000000000001000000")
	require.NoError(t, err)
	assert.Equal(t, "::1", ip)
}

func TestParseIPv6ProcAllZeros(t *testing.T) {
	// :: — all zero address
	ip, err := parseIPv6Proc("00000000000000000000000000000000")
	require.NoError(t, err)
	assert.Equal(t, "::", ip)
}

func TestParseIPv6ProcTooShort(t *testing.T) {
	_, err := parseIPv6Proc("0000000000000000000000000000000")
	assert.Error(t, err)
}

func TestParseIPv6ProcTooLong(t *testing.T) {
	_, err := parseIPv6Proc("000000000000000000000000000000001")
	assert.Error(t, err)
}

func TestParseIPv6ProcEmpty(t *testing.T) {
	_, err := parseIPv6Proc("")
	assert.Error(t, err)
}

func TestParseIPv6ProcNonHex(t *testing.T) {
	_, err := parseIPv6Proc("ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ")
	assert.Error(t, err)
}

// --- formatIPv6 ---

func TestFormatIPv6Loopback(t *testing.T) {
	var b [16]byte
	b[15] = 1
	assert.Equal(t, "::1", formatIPv6(b))
}

func TestFormatIPv6AllZeros(t *testing.T) {
	var b [16]byte
	assert.Equal(t, "::", formatIPv6(b))
}

func TestFormatIPv6NoCompression(t *testing.T) {
	// 2001:db8:1:2:3:4:5:6 — no run of zeros long enough to compress (all single).
	var b [16]byte
	// 2001:0db8:0001:0002:0003:0004:0005:0006
	b[0], b[1] = 0x20, 0x01
	b[2], b[3] = 0x0d, 0xb8
	b[4], b[5] = 0x00, 0x01
	b[6], b[7] = 0x00, 0x02
	b[8], b[9] = 0x00, 0x03
	b[10], b[11] = 0x00, 0x04
	b[12], b[13] = 0x00, 0x05
	b[14], b[15] = 0x00, 0x06
	assert.Equal(t, "2001:db8:1:2:3:4:5:6", formatIPv6(b))
}

func TestFormatIPv6LongestZeroRunPicked(t *testing.T) {
	// 2001:0:0:1:0:0:0:1 — two runs of zeros; the longer run (3 groups) wins.
	var b [16]byte
	b[0], b[1] = 0x20, 0x01 // 2001
	// groups 1-2: zero
	b[6], b[7] = 0x00, 0x01 // 0001
	// groups 4-6: zero (3 groups)
	b[14], b[15] = 0x00, 0x01 // 0001
	assert.Equal(t, "2001:0:0:1::1", formatIPv6(b))
}

func TestFormatIPv6IPv4Mapped(t *testing.T) {
	// ::ffff:127.0.0.1
	var b [16]byte
	b[10], b[11] = 0xff, 0xff
	b[12] = 127
	b[15] = 1
	assert.Equal(t, "::ffff:7f00:1", formatIPv6(b))
}

func TestFormatIPv6SingleZeroGroupNotCompressed(t *testing.T) {
	// 2001:db8:0:1:2:3:4:5 — single zero at position 2, no :: compression.
	var b [16]byte
	b[0], b[1] = 0x20, 0x01
	b[2], b[3] = 0x0d, 0xb8
	// group 2: 0x0000 (zero but only one group)
	b[6], b[7] = 0x00, 0x01
	b[8], b[9] = 0x00, 0x02
	b[10], b[11] = 0x00, 0x03
	b[12], b[13] = 0x00, 0x04
	b[14], b[15] = 0x00, 0x05
	result := formatIPv6(b)
	// Single zero group must not be compressed with ::
	assert.NotContains(t, result, "::")
	assert.Contains(t, result, ":0:")
}
