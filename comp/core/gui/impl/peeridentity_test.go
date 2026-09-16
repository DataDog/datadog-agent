// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package guiimpl

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubAddr is a net.Addr whose String() is an arbitrary literal, letting
// tests exercise serverAddr values that net.TCPAddr/net.UnixAddr can't
// produce (e.g. a host that isn't a valid IP).
type stubAddr string

func (s stubAddr) Network() string { return "tcp" }
func (s stubAddr) String() string  { return string(s) }

func TestResolvePeerIdentity(t *testing.T) {
	t.Run("nil serverAddr is rejected", func(t *testing.T) {
		_, err := resolvePeerIdentity(nil, "127.0.0.1:1234")
		assert.ErrorContains(t, err, "server address unavailable")
	})

	t.Run("a Unix domain socket serverAddr is rejected, not mishandled", func(t *testing.T) {
		// The CMD/IPC API server can be configured to listen on a Unix domain
		// socket instead of TCP (see comp/api/api/apiimpl/listener/common.go's
		// GetListener); *net.UnixAddr.String() returns just the socket path,
		// which has no ":port" for net.SplitHostPort to find.
		addr := &net.UnixAddr{Name: "/var/run/datadog/agent.sock", Net: "unix"}
		_, err := resolvePeerIdentity(addr, "127.0.0.1:1234")
		assert.ErrorContains(t, err, "malformed server address")
	})

	t.Run("a serverAddr with no parseable IP host is rejected", func(t *testing.T) {
		_, err := resolvePeerIdentity(stubAddr("not-an-ip:1234"), "127.0.0.1:1234")
		assert.ErrorContains(t, err, "no parseable IP")
	})

	t.Run("a serverAddr with a non-numeric port is rejected", func(t *testing.T) {
		_, err := resolvePeerIdentity(stubAddr("127.0.0.1:not-a-port"), "127.0.0.1:1234")
		assert.ErrorContains(t, err, "malformed server port")
	})

	t.Run("a malformed remoteAddr is rejected", func(t *testing.T) {
		_, err := resolvePeerIdentity(stubAddr("127.0.0.1:8080"), "not-a-valid-remote-addr")
		assert.ErrorContains(t, err, "malformed remote address")
	})

	t.Run("a non-loopback remoteAddr host is rejected", func(t *testing.T) {
		_, err := resolvePeerIdentity(stubAddr("127.0.0.1:8080"), "8.8.8.8:1234")
		assert.ErrorContains(t, err, "not a loopback address")
	})

	t.Run("a remoteAddr with no parseable IP host is rejected the same way", func(t *testing.T) {
		_, err := resolvePeerIdentity(stubAddr("127.0.0.1:8080"), "not-an-ip:1234")
		assert.ErrorContains(t, err, "not a loopback address")
	})

	t.Run("a remoteAddr with a non-numeric port is rejected", func(t *testing.T) {
		_, err := resolvePeerIdentity(stubAddr("127.0.0.1:8080"), "127.0.0.1:not-a-port")
		assert.ErrorContains(t, err, "malformed remote port")
	})
}

func TestMintTimeIdentity(t *testing.T) {
	t.Run("root is treated as unconstrained", func(t *testing.T) {
		assert.Empty(t, mintTimeIdentity(rootIdentity))
	})

	t.Run("an unresolved identity stays unconstrained", func(t *testing.T) {
		assert.Empty(t, mintTimeIdentity(peerIdentity("")))
	})

	t.Run("a non-root identity is preserved", func(t *testing.T) {
		assert.Equal(t, peerIdentity("1000"), mintTimeIdentity(peerIdentity("1000")))
	})
}
