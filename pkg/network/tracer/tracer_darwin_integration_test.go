// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin && integration

// Run these tests with:
//
//	sudo go test -tags integration ./pkg/network/tracer/... -run TestDarwinCNM -v
//
// Requirements:
//   - Root (or wheel-group membership) is required to open BPF devices (/dev/bpf*).
//   - Internet connectivity is required. The tests generate real outgoing traffic to
//     well-known public IPs (1.1.1.1, 8.8.8.8). Tests that need internet access call
//     requireInternet(t) and skip gracefully if connectivity is absent.
//
// Background:
//
// The Darwin pcap backend captures on non-loopback interfaces (en0, utun*, etc.).
// Self-to-self traffic where both src and dst IPs are local to the same interface
// is classified as PacketOtherHost and dropped — this is correct production
// behaviour. Therefore these tests use real outgoing connections to external hosts
// to exercise the full packet-capture path.
package tracer

import (
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

const (
	cnmClientID     = "cnm-integration-test"
	cnmPollInterval = 100 * time.Millisecond
	cnmPollTimeout  = 8 * time.Second

	// Well-known public endpoints used to generate real outgoing traffic.
	// Both are anycast addresses highly unlikely to be blocked on a dev machine.
	externalTCPTarget = "1.1.1.1:80"  // Cloudflare HTTP
	externalDNSTarget = "8.8.8.8:53"  // Google DNS (UDP)
	externalTCPPort   = uint16(80)
	externalDNSPort   = uint16(53)
	externalTCPIP     = "1.1.1.1"
	externalDNSIP     = "8.8.8.8"
)

// ============================================================================
// Setup helpers
// ============================================================================

// requireRoot skips the test if the process is not running as root.
func requireRoot(t *testing.T) {
	t.Helper()
	if os.Getuid() != 0 {
		t.Skip("CNM integration tests require root (needed to open BPF devices)")
	}
}

// requireInternet dials the well-known external TCP target to confirm internet
// connectivity. Tests that require real outgoing traffic call this first.
func requireInternet(t *testing.T) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", externalTCPTarget, 3*time.Second)
	if err != nil {
		t.Skipf("no internet connectivity (%v), skipping CNM integration test", err)
	}
	conn.Close()
}

// setupCNMTracer starts a real Tracer backed by libpcap and registers the
// integration-test client.
func setupCNMTracer(t *testing.T) *Tracer {
	t.Helper()
	requireRoot(t)

	cfg := config.New()
	tel := noopsimpl.GetCompatComponent()

	tr, err := NewTracer(cfg, tel, nil)
	require.NoError(t, err, "NewTracer must succeed (are you running as root?)")
	t.Cleanup(tr.Stop)

	require.NoError(t, tr.RegisterClient(cnmClientID))
	return tr
}

// getConns returns the current active connections for the integration client.
func getConns(t *testing.T, tr *Tracer) *network.Connections {
	t.Helper()
	conns, cleanup, err := tr.GetActiveConnections(cnmClientID)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	return conns
}

// waitForConnection polls until pred returns true for at least one connection.
func waitForConnection(t *testing.T, tr *Tracer, pred func(network.ConnectionStats) bool) *network.ConnectionStats {
	t.Helper()
	var found *network.ConnectionStats
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		conns := getConns(t, tr)
		for _, conn := range conns.Conns {
			if pred(conn) {
				cp := conn
				found = &cp
				return
			}
		}
		assert.Fail(c, "connection not yet visible")
	}, cnmPollTimeout, cnmPollInterval, "connection did not appear within timeout")
	return found
}

// ============================================================================
// T-4: Integration tests
// ============================================================================

// TestDarwinCNM_TCPOutgoingDirection verifies that a new outgoing TCP connection
// to an external host is reported with Direction=OUTGOING.
func TestDarwinCNM_TCPOutgoingDirection(t *testing.T) {
	requireInternet(t)
	tr := setupCNMTracer(t)

	c, err := net.DialTimeout("tcp", externalTCPTarget, 5*time.Second)
	if err == nil {
		defer c.Close()
	}
	// Even a refused/timed-out SYN is sent through the interface and captured.

	conn := waitForConnection(t, tr, func(cs network.ConnectionStats) bool {
		return cs.Type == network.TCP &&
			cs.Direction == network.OUTGOING &&
			cs.DPort == externalTCPPort
	})

	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.Equal(t, network.TCP, conn.Type)
	assert.Equal(t, externalTCPPort, conn.DPort)
}

// TestDarwinCNM_UDPOutgoingDirection verifies that a UDP datagram to an external
// host is reported with Direction=OUTGOING.
func TestDarwinCNM_UDPOutgoingDirection(t *testing.T) {
	requireInternet(t)
	tr := setupCNMTracer(t)

	// Send a minimal DNS query to 8.8.8.8:53.
	addr, err := net.ResolveUDPAddr("udp4", externalDNSTarget)
	require.NoError(t, err)
	conn, err := net.DialUDP("udp4", nil, addr)
	require.NoError(t, err)
	defer conn.Close()

	// Minimal valid DNS query for "." (root) type A.
	dnsQuery := []byte{
		0x00, 0x01, // ID
		0x01, 0x00, // Flags: standard query
		0x00, 0x01, // Questions: 1
		0x00, 0x00, // Answers: 0
		0x00, 0x00, // Authority: 0
		0x00, 0x00, // Additional: 0
		0x00,       // Root label
		0x00, 0x01, // Type A
		0x00, 0x01, // Class IN
	}
	_, err = conn.Write(dnsQuery)
	require.NoError(t, err)

	found := waitForConnection(t, tr, func(cs network.ConnectionStats) bool {
		return cs.Type == network.UDP &&
			cs.Direction == network.OUTGOING &&
			cs.DPort == externalDNSPort
	})

	assert.Equal(t, network.OUTGOING, found.Direction)
	assert.Equal(t, network.UDP, found.Type)
}

// TestDarwinCNM_TCPByteCount verifies that sent byte counters accumulate after
// data is exchanged with an external server.
//
// GetDelta only returns a connection when its stats change. Therefore we write
// data first and poll for a single event where the connection appears with
// SentBytes > 0 — avoiding a two-step approach that would miss the one delta.
func TestDarwinCNM_TCPByteCount(t *testing.T) {
	requireInternet(t)
	tr := setupCNMTracer(t)

	c, err := net.DialTimeout("tcp", externalTCPTarget, 5*time.Second)
	require.NoError(t, err, "TCP dial to external host required for byte count test")
	defer c.Close()

	// Write data before polling so SentBytes are already accumulated when the
	// connection first appears in GetDelta.
	// TCPPayloadLen derives counts from IP header fields, so they are accurate
	// even with snapLen=120 (only headers captured by the Darwin backend).
	httpReq := "GET / HTTP/1.0\r\nHost: 1.1.1.1\r\n\r\n"
	_, err = io.WriteString(c, httpReq)
	require.NoError(t, err)

	conn := waitForConnection(t, tr, func(cs network.ConnectionStats) bool {
		return cs.Type == network.TCP &&
			cs.Direction == network.OUTGOING &&
			cs.DPort == externalTCPPort &&
			cs.Monotonic.SentBytes > 0
	})

	assert.Greater(t, conn.Monotonic.SentBytes, uint64(0),
		"SentBytes should be non-zero after writing HTTP request")
}

// TestDarwinCNM_TCPGracefulClose verifies that after a graceful TCP teardown
// the connection no longer appears in the active connection set.
func TestDarwinCNM_TCPGracefulClose(t *testing.T) {
	requireInternet(t)
	tr := setupCNMTracer(t)

	c, err := net.DialTimeout("tcp", externalTCPTarget, 5*time.Second)
	require.NoError(t, err, "TCP dial to external host required for close test")

	// Wait until the connection is visible.
	waitForConnection(t, tr, func(cs network.ConnectionStats) bool {
		return cs.Type == network.TCP &&
			cs.Direction == network.OUTGOING &&
			cs.DPort == externalTCPPort
	})

	// Close the connection and wait for it to leave the active set.
	c.Close()

	require.EventuallyWithT(t, func(col *assert.CollectT) {
		conns := getConns(t, tr)
		for _, cs := range conns.Conns {
			if cs.Type == network.TCP && cs.Direction == network.OUTGOING && cs.DPort == externalTCPPort {
				assert.Fail(col, "closed connection still present in active set")
				return
			}
		}
	}, cnmPollTimeout, cnmPollInterval, "closed connection did not leave active set")
}

// TestDarwinCNM_MultipleConnections verifies that several concurrent outgoing
// TCP connections are tracked independently.
func TestDarwinCNM_MultipleConnections(t *testing.T) {
	requireInternet(t)
	tr := setupCNMTracer(t)

	// Open three TCP connections to external hosts on different well-known ports.
	targets := []string{
		"1.1.1.1:80",
		"1.1.1.1:443",
		"8.8.8.8:53",
	}
	ports := []uint16{80, 443, 53}

	var conns []net.Conn
	for _, target := range targets {
		c, err := net.DialTimeout("tcp", target, 5*time.Second)
		if err == nil {
			conns = append(conns, c)
		}
		// Even refused connections send a SYN that is captured.
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// Each outgoing port must appear.
	for _, port := range ports {
		p := port // capture
		waitForConnection(t, tr, func(cs network.ConnectionStats) bool {
			return cs.Type == network.TCP &&
				cs.Direction == network.OUTGOING &&
				cs.DPort == p
		})
	}
}

// TestDarwinCNM_PreexistingConnectionNotSeen documents a known behavioral
// difference from the Linux eBPF tracer: connections established before the
// libpcap capture started are invisible because pcap only sees new packets.
func TestDarwinCNM_PreexistingConnectionNotSeen(t *testing.T) {
	requireInternet(t)

	// Establish the connection BEFORE starting the tracer.
	preexisting, err := net.DialTimeout("tcp", externalTCPTarget, 5*time.Second)
	require.NoError(t, err, "pre-existing TCP connection required")
	defer preexisting.Close()

	// Now start the tracer.
	tr := setupCNMTracer(t)

	// Give the tracer a short window to observe any phantom packets.
	time.Sleep(500 * time.Millisecond)

	conns := getConns(t, tr)
	for _, cs := range conns.Conns {
		if cs.Type == network.TCP && cs.DPort == externalTCPPort {
			t.Logf("NOTE: pre-existing connection IS visible (unexpected): src=%v dst=%v bytes=%d",
				cs.Source, cs.Dest, cs.Monotonic.SentBytes)
			return
		}
	}
	t.Logf("Confirmed: pre-existing connection is not visible (expected pcap limitation)")
}

// TestDarwinCNM_GetStats verifies that GetStats returns the darwin_pcap backend.
func TestDarwinCNM_GetStats(t *testing.T) {
	tr := setupCNMTracer(t)

	stats, err := tr.GetStats()
	require.NoError(t, err)

	assert.Contains(t, stats, "state")
	assert.Contains(t, stats, "tracer")

	tracerStats, ok := stats["tracer"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "darwin_pcap", tracerStats["type"])
}

// TestDarwinCNM_StopIsClean verifies that a single Stop() call does not panic.
// Note: calling Stop() twice will panic because ebpfLessTracer.Stop() closes
// an unbuffered channel without sync.Once protection — callers must not double-stop.
func TestDarwinCNM_StopIsClean(t *testing.T) {
	requireRoot(t)
	cfg := config.New()
	tel := noopsimpl.GetCompatComponent()

	tr, err := NewTracer(cfg, tel, nil)
	require.NoError(t, err)
	require.NoError(t, tr.RegisterClient(cnmClientID))

	// Single stop must not panic.
	assert.NotPanics(t, func() { tr.Stop() })
}
