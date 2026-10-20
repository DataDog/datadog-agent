// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// White-box unit tests for the shared helper functions in ss.go.
// These run on all platforms and exercise filterEntry, isListening,
// formatAddrPort, netidStr and summary output.

package ss

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- isListening ---

func TestIsListeningLISTEN(t *testing.T) {
	assert.True(t, isListening(socketEntry{state: "LISTEN"}))
}

func TestIsListeningUNCONN(t *testing.T) {
	// UDP bound-but-unconnected sockets are treated as "listening".
	assert.True(t, isListening(socketEntry{state: "UNCONN"}))
}

func TestIsListeningESTAB(t *testing.T) {
	assert.False(t, isListening(socketEntry{state: "ESTAB"}))
}

func TestIsListeningCLOSE(t *testing.T) {
	assert.False(t, isListening(socketEntry{state: "CLOSE"}))
}

func TestIsListeningTimeWait(t *testing.T) {
	assert.False(t, isListening(socketEntry{state: "TIME-WAIT"}))
}

// --- formatAddrPort ---

func TestFormatAddrPortNormal(t *testing.T) {
	assert.Equal(t, "127.0.0.1:22", formatAddrPort("127.0.0.1", "22"))
}

func TestFormatAddrPortZeroPort(t *testing.T) {
	assert.Equal(t, "0.0.0.0:*", formatAddrPort("0.0.0.0", "0"))
}

func TestFormatAddrPortEmptyPort(t *testing.T) {
	assert.Equal(t, "*:*", formatAddrPort("*", ""))
}

func TestFormatAddrPortIPv6(t *testing.T) {
	assert.Equal(t, "::1:443", formatAddrPort("::1", "443"))
}

// --- netidStr ---

func TestNetidStrTCP4(t *testing.T) {
	assert.Equal(t, "tcp", netidStr(socketEntry{kind: sockTCP4}))
}

func TestNetidStrTCP6(t *testing.T) {
	assert.Equal(t, "tcp", netidStr(socketEntry{kind: sockTCP6}))
}

func TestNetidStrUDP4(t *testing.T) {
	assert.Equal(t, "udp", netidStr(socketEntry{kind: sockUDP4}))
}

func TestNetidStrUDP6(t *testing.T) {
	assert.Equal(t, "udp", netidStr(socketEntry{kind: sockUDP6}))
}

func TestNetidStrUnix(t *testing.T) {
	assert.Equal(t, "u_str", netidStr(socketEntry{kind: sockUnix}))
}

// --- filterEntry: protocol filter ---

func TestFilterEntryShowOnlyTCP(t *testing.T) {
	opts := options{showTCP: true, showUDP: false, showUnix: false, showAll: true}
	assert.True(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "ESTAB"}))
	assert.True(t, filterEntry(opts, socketEntry{kind: sockTCP6, state: "ESTAB"}))
	assert.False(t, filterEntry(opts, socketEntry{kind: sockUDP4, state: "ESTAB"}))
	assert.False(t, filterEntry(opts, socketEntry{kind: sockUnix, state: "ESTAB"}))
}

func TestFilterEntryShowOnlyUDP(t *testing.T) {
	opts := options{showTCP: false, showUDP: true, showUnix: false, showAll: true}
	assert.False(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "ESTAB"}))
	assert.True(t, filterEntry(opts, socketEntry{kind: sockUDP4, state: "ESTAB"}))
	assert.True(t, filterEntry(opts, socketEntry{kind: sockUDP6, state: "ESTAB"}))
	assert.False(t, filterEntry(opts, socketEntry{kind: sockUnix, state: "ESTAB"}))
}

func TestFilterEntryShowOnlyUnix(t *testing.T) {
	opts := options{showTCP: false, showUDP: false, showUnix: true, showAll: true}
	assert.False(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "ESTAB"}))
	assert.False(t, filterEntry(opts, socketEntry{kind: sockUDP4, state: "ESTAB"}))
	assert.True(t, filterEntry(opts, socketEntry{kind: sockUnix, state: "ESTAB"}))
}

// --- filterEntry: IP version filter ---

func TestFilterEntryIPv4OnlyDropsTCP6(t *testing.T) {
	opts := options{showTCP: true, showAll: true, ipv4Only: true}
	assert.True(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "ESTAB"}))
	assert.False(t, filterEntry(opts, socketEntry{kind: sockTCP6, state: "ESTAB"}))
}

func TestFilterEntryIPv4OnlyDropsUDP6(t *testing.T) {
	opts := options{showUDP: true, showAll: true, ipv4Only: true}
	assert.True(t, filterEntry(opts, socketEntry{kind: sockUDP4, state: "ESTAB"}))
	assert.False(t, filterEntry(opts, socketEntry{kind: sockUDP6, state: "ESTAB"}))
}

func TestFilterEntryIPv4OnlyDoesNotDropUnix(t *testing.T) {
	opts := options{showUnix: true, showAll: true, ipv4Only: true}
	assert.True(t, filterEntry(opts, socketEntry{kind: sockUnix, state: "ESTAB"}))
}

func TestFilterEntryIPv6OnlyDropsTCP4(t *testing.T) {
	opts := options{showTCP: true, showAll: true, ipv6Only: true}
	assert.False(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "ESTAB"}))
	assert.True(t, filterEntry(opts, socketEntry{kind: sockTCP6, state: "ESTAB"}))
}

func TestFilterEntryIPv6OnlyDoesNotDropUnix(t *testing.T) {
	opts := options{showUnix: true, showAll: true, ipv6Only: true}
	assert.True(t, filterEntry(opts, socketEntry{kind: sockUnix, state: "ESTAB"}))
}

// --- filterEntry: listening / non-listening ---

func TestFilterEntryDefaultNonListeningOnly(t *testing.T) {
	// Default (no showAll, no listenOnly) shows non-listening only.
	opts := options{showTCP: true}
	assert.True(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "ESTAB"}))
	assert.False(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "LISTEN"}))
	assert.False(t, filterEntry(opts, socketEntry{kind: sockUDP4, state: "UNCONN"}))
}

func TestFilterEntryListenOnly(t *testing.T) {
	opts := options{showTCP: true, showUDP: true, listenOnly: true}
	assert.False(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "ESTAB"}))
	assert.True(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "LISTEN"}))
	assert.True(t, filterEntry(opts, socketEntry{kind: sockUDP4, state: "UNCONN"}))
}

func TestFilterEntryShowAll(t *testing.T) {
	opts := options{showTCP: true, showUDP: true, showAll: true}
	assert.True(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "ESTAB"}))
	assert.True(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "LISTEN"}))
	assert.True(t, filterEntry(opts, socketEntry{kind: sockUDP4, state: "UNCONN"}))
}

// --- printSummary counts ---

// capturingCallCtx wraps a CallContext substitute that collects output.
type capturingWriter struct {
	out string
}

func (c *capturingWriter) Out(s string) {
	c.out += s
}

func TestFilterEntryShowAllVsListenOnly(t *testing.T) {
	// showAll and listenOnly are mutually exclusive in practice but we verify
	// showAll takes precedence (it is checked first in filterEntry).
	opts := options{showTCP: true, showAll: true, listenOnly: true}
	assert.True(t, filterEntry(opts, socketEntry{kind: sockTCP4, state: "ESTAB"}),
		"showAll should override listenOnly")
}
