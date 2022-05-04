// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf || (windows && npm)
// +build linux_bpf windows,npm

package tracer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	nethttp "net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/network/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cihub/seelog"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

var (
	clientMessageSize = 2 << 8
	serverMessageSize = 2 << 14
	payloadSizesTCP   = []int{2 << 5, 2 << 8, 2 << 10, 2 << 12, 2 << 14, 2 << 15}
	payloadSizesUDP   = []int{2 << 5, 2 << 8, 2 << 12, 2 << 14}
)

// runtimeCompilationEnvVar forces use of the runtime compiler for ebpf functionality
const runtimeCompilationEnvVar = "DD_TESTS_RUNTIME_COMPILED"

func TestMain(m *testing.M) {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "warn"
	}
	log.SetupLogger(seelog.Default, logLevel)
	cfg := testConfig()
	if cfg.EnableRuntimeCompiler {
		fmt.Println("RUNTIME COMPILER ENABLED")
	}
	os.Exit(m.Run())
}

func TestGetStats(t *testing.T) {
	httpSupported := httpSupported(t)
	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	<-time.After(time.Second)

	tr.GetActiveConnections("-1")
	linuxExpected := map[string]interface{}{}
	err = json.Unmarshal([]byte(`{
      "conntrack": {
        "enobufs": 0,
        "evicts_total": 0,
        "gets_total": 9,
        "msg_errors": 0,
        "orphan_size": 0,
        "read_errors": 0,
        "registers_dropped": 2,
        "registers_total": 0,
        "sampling_pct": 100,
        "state_size": 0,
        "throttles": 0,
        "unregisters_total": 0
      },
      "dns": {
        "added": 0,
        "decoding_errors": 583,
        "dropped_stats": 0,
        "errors": 0,
        "expired": 0,
        "ips": 0,
        "lookups": 5,
        "num_stats": 0,
        "oversized": 0,
        "packets_captured": 586,
        "packets_dropped": 0,
        "packets_processed": 586,
        "queries": 0,
        "resolved": 0,
        "socket_polls": 6,
        "successes": 0,
        "timestamp_micro_secs": 1649790301434884,
        "truncated_packets": 0
      },
      "ebpf": {
        "closed_conn_polling_lost": 0,
        "closed_conn_polling_received": 0,
        "conn_stats_max_entries_hit": 0,
        "missed_tcp_close": 0,
        "missed_udp_close": 0,
        "pid_collisions": 0,
        "tcp_conns4": 1,
        "tcp_conns6": 0,
        "tcp_sent_miscounts": 0,
        "udp_conns4": 4,
        "udp_conns6": 0,
        "udp_sends_missed": 0,
        "udp_sends_processed": 162
      },
      "http": {
        "aggregations": 0,
        "dropped": 0,
        "hits1_xx": 0,
        "hits2_xx": 0,
        "hits3_xx": 0,
        "hits4_xx": 0,
        "hits5_xx": 0,
        "misses": 0,
        "rejected": 0
      },
      "kprobes": {},
      "state": {
        "closed_conn_dropped": 0,
		"conn_dropped": 0,
		"dns_pid_collisions": 0,
		"dns_stats_dropped": 0,
		"http_stats_dropped": 0,
		"stats_resets": 0,
		"time_sync_collisions": 0
      },
      "tracer": {
        "closed_conns": 1,
        "conn_stats_map_size": 5,
        "expired_tcp_conns": 0,
        "runtime": {
          "runtime_compilation_enabled": 0
        },
        "skipped_conns": 0
      }
    }`), &linuxExpected)
	require.NoError(t, err)

	rcExceptions := map[string]interface{}{}
	err = json.Unmarshal([]byte(`{
      "conntrack": {
        "evicts_total": 0,
        "orphan_size": 0
      }}`), &rcExceptions)
	require.NoError(t, err)

	expected := linuxExpected
	if runtime.GOOS == "windows" {
		expected = map[string]interface{}{
			"driver":                   map[string]interface{}{},
			"flows":                    map[string]interface{}{},
			"driver_total_flow_stats":  map[string]interface{}{},
			"driver_flow_handle_stats": map[string]interface{}{},
			"state":                    map[string]interface{}{},
			"dns":                      map[string]interface{}{},
		}
	}

	actual, _ := tr.GetStats()

	for section, entries := range expected {
		if section == "http" && !httpSupported {
			continue
		}
		require.Contains(t, actual, section, "missing section from telemetry map: %s", section)
		for name := range entries.(map[string]interface{}) {
			if cfg.EnableRuntimeCompiler {
				if sec, ok := rcExceptions[section]; ok {
					if _, ok := sec.(map[string]interface{})[name]; ok {
						continue
					}
				}
			}
			assert.Contains(t, actual[section], name, "%s actual is missing %s", section, name)
		}
	}
}

func TestTCPSendAndReceive(t *testing.T) {
	// Enable BPF-based system probe
	tr, err := NewTracer(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Create TCP Server which, for every line, sends back a message with size=serverMessageSize
	server := NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		for {
			_, err := r.ReadBytes(byte('\n'))
			c.Write(genPayload(serverMessageSize))
			if err != nil { // indicates that EOF has been reached,
				break
			}
		}
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Connect to server 10 times
	wg := new(errgroup.Group)
	for i := 0; i < 10; i++ {
		wg.Go(func() error {
			// Write clientMessageSize to server, and read response
			if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
				return err
			}

			r := bufio.NewReader(c)
			r.ReadBytes(byte('\n'))
			return nil
		})
	}

	err = wg.Wait()
	require.NoError(t, err)

	initTracerState(t, tr)

	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections := getConnections(t, tr)

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, 10*clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, 10*serverMessageSize, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	doneChan <- struct{}{}
}

func TestPreexistingConnectionDirection(t *testing.T) {
	// windows doesn't have a way to read existing port bindings on startup (yet)
	skipIfWindows(t)
	// Start the client and server before we enable the system probe to test that the tracer picks
	// up the pre-existing connection
	doneChan := make(chan struct{})

	server := NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		_, _ = r.ReadBytes(byte('\n'))
		_, _ = c.Write(genPayload(serverMessageSize))
		_ = c.Close()
	})
	err := server.Run(doneChan)
	require.NoError(t, err)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()

	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}

	// Enable BPF-based system probe
	tr, err := NewTracer(testConfig())
	require.NoError(t, err)
	defer tr.Stop()

	// Write more data so that the tracer will notice the connection
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)

	r := bufio.NewReader(c)
	_, _ = r.ReadBytes(byte('\n'))

	initTracerState(t, tr)
	connections := getConnections(t, tr)

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, serverMessageSize, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	doneChan <- struct{}{}
}

func TestTCPShortlived(t *testing.T) {
	// Enable BPF-based system probe
	cfg := testConfig()
	cfg.TCPClosedTimeout = 10 * time.Millisecond
	tr, err := NewTracer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	initTracerState(t, tr)

	// Create TCP Server which sends back serverMessageSize bytes
	server := NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		r.ReadBytes(byte('\n'))
		c.Write(genPayload(serverMessageSize))
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	// Connect to server
	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)

	// Write clientMessageSize to server, and read response
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	// Explicitly close this TCP connection
	c.Close()

	var conn *network.ConnectionStats
	require.Eventually(t, func() bool {
		var ok bool
		conn, ok = findConnection(c.LocalAddr(), c.RemoteAddr(), getConnections(t, tr))
		return ok
	}, 3*time.Second, time.Second, "connection not found")

	assert.Equal(t, clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, serverMessageSize, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, addrPort(server.address), int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	// Verify the short lived connection is accounting for both TCP_ESTABLISHED and TCP_CLOSED events
	assert.Equal(t, uint32(1), conn.MonotonicTCPEstablished)
	assert.Equal(t, uint32(1), conn.MonotonicTCPClosed)

	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), getConnections(t, tr))
	assert.False(t, ok)
}

func TestTCPOverIPv6(t *testing.T) {
	t.SkipNow()
	if !kernel.IsIPv6Enabled() {
		t.Skip("IPv6 not enabled on host")
	}

	config := testConfig()
	config.CollectIPv6Conns = true

	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	ln, err := net.Listen("tcp6", ":0")
	require.NoError(t, err)

	doneChan := make(chan struct{})
	go func(done chan struct{}) {
		<-done
		ln.Close()
	}(doneChan)

	// Create TCP Server which sends back serverMessageSize bytes
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			r := bufio.NewReader(c)
			r.ReadBytes(byte('\n'))
			c.Write(genPayload(serverMessageSize))
			c.Close()
		}
	}()

	// Connect to server
	c, err := net.DialTimeout("tcp6", ln.Addr().String(), 50*time.Millisecond)
	require.NoError(t, err)

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	initTracerState(t, tr)
	connections := getConnections(t, tr)

	conn, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.True(t, ok)
	assert.Equal(t, clientMessageSize, int(conn.MonotonicSentBytes))
	assert.Equal(t, serverMessageSize, int(conn.MonotonicRecvBytes))
	assert.Equal(t, 0, int(conn.MonotonicRetransmits))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, ln.Addr().(*net.TCPAddr).Port, int(conn.DPort))
	assert.Equal(t, network.OUTGOING, conn.Direction)
	assert.True(t, conn.IntraHost)

	doneChan <- struct{}{}

}

func TestTCPCollectionDisabled(t *testing.T) {
	// Enable BPF-based system probe with TCP disabled
	config := testConfig()
	config.CollectTCPConns = false

	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Create TCP Server which sends back serverMessageSize bytes
	server := NewTCPServer(func(c net.Conn) {
		r := bufio.NewReader(c)
		r.ReadBytes(byte('\n'))
		c.Write(genPayload(serverMessageSize))
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	// Connect to server
	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	r.ReadBytes(byte('\n'))

	initTracerState(t, tr)
	connections := getConnections(t, tr)

	// Confirm that we could not find connection created above
	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.False(t, ok)
}

func TestUDPSendAndReceive(t *testing.T) {
	t.Run("v4", func(t *testing.T) {
		testUDPSendAndReceive(t, "127.0.0.1:8001")
	})
	t.Run("v6", func(t *testing.T) {
		testUDPSendAndReceive(t, "[::1]:8001")
	})
}

func testUDPSendAndReceive(t *testing.T, addr string) {
	cfg := testConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	t.Cleanup(tr.Stop)

	server := &UDPServer{
		address: addr,
		onMessage: func(buf []byte, n int) []byte {
			return genPayload(serverMessageSize)
		},
	}

	doneChan := make(chan struct{})
	err = server.Run(doneChan, clientMessageSize)
	require.NoError(t, err)
	t.Cleanup(func() { close(doneChan) })

	// Connect to server
	c, err := net.DialTimeout("udp", server.address, 50*time.Millisecond)
	require.NoError(t, err)
	defer c.Close()

	// Write clientMessageSize to server, and read response
	_, err = c.Write(genPayload(clientMessageSize))
	require.NoError(t, err)

	_, err = c.Read(make([]byte, serverMessageSize))
	require.NoError(t, err)

	initTracerState(t, tr)
	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections := getConnections(t, tr)

	incoming, ok := findConnection(c.RemoteAddr(), c.LocalAddr(), connections)
	if assert.True(t, ok, "unable to find incoming connection") {
		assert.Equal(t, network.INCOMING, incoming.Direction)

		// make sure the inverse values are seen for the other message
		assert.Equal(t, serverMessageSize, int(incoming.MonotonicSentBytes), "incoming sent")
		assert.Equal(t, clientMessageSize, int(incoming.MonotonicRecvBytes), "incoming recv")
		assert.True(t, incoming.IntraHost, "incoming intrahost")
	}

	outgoing, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	if assert.True(t, ok, "unable to find outgoing connection") {
		assert.Equal(t, network.OUTGOING, outgoing.Direction)

		assert.Equal(t, clientMessageSize, int(outgoing.MonotonicSentBytes), "outgoing sent")
		assert.Equal(t, serverMessageSize, int(outgoing.MonotonicRecvBytes), "outgoing recv")
		assert.True(t, outgoing.IntraHost, "outgoing intrahost")
	}
}

func TestUDPDisabled(t *testing.T) {
	// Enable BPF-based system probe with UDP disabled
	config := testConfig()
	config.CollectUDPConns = false

	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Create UDP Server which sends back serverMessageSize bytes
	server := &UDPServer{
		onMessage: func(b []byte, n int) []byte {
			return genPayload(serverMessageSize)
		},
	}

	doneChan := make(chan struct{})
	err = server.Run(doneChan, clientMessageSize)
	require.NoError(t, err)
	defer close(doneChan)

	// Connect to server
	c, err := net.DialTimeout("udp", server.address, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Write clientMessageSize to server, and read response
	if _, err = c.Write(genPayload(clientMessageSize)); err != nil {
		t.Fatal(err)
	}

	c.Read(make([]byte, serverMessageSize))

	initTracerState(t, tr)
	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections := getConnections(t, tr)

	_, ok := findConnection(c.LocalAddr(), c.RemoteAddr(), connections)
	require.False(t, ok)
}

func TestLocalDNSCollectionDisabled(t *testing.T) {
	// Enable BPF-based system probe with DNS disabled (by default)
	config := testConfig()

	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Connect to local DNS
	addr, err := net.ResolveUDPAddr("udp", "localhost:53")
	assert.NoError(t, err)

	cn, err := net.DialUDP("udp", nil, addr)
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	initTracerState(t, tr)

	// Iterate through active connections making sure there are no local DNS calls
	for _, c := range getConnections(t, tr).Conns {
		assert.False(t, isLocalDNS(c))
	}
}

func TestLocalDNSCollectionEnabled(t *testing.T) {
	// Enable BPF-based system probe with DNS enabled
	config := testConfig()
	config.CollectLocalDNS = true
	config.CollectUDPConns = true

	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Connect to local DNS
	addr, err := net.ResolveUDPAddr("udp", "localhost:53")
	assert.NoError(t, err)

	cn, err := net.DialUDP("udp", nil, addr)
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	found := false

	initTracerState(t, tr)

	// Iterate through active connections making sure theres at least one connection
	for _, c := range getConnections(t, tr).Conns {
		found = found || isLocalDNS(c)
	}

	assert.True(t, found)
}

func isLocalDNS(c network.ConnectionStats) bool {
	return c.Source.String() == "127.0.0.1" && c.Dest.String() == "127.0.0.1" && c.DPort == 53
}

func TestShouldSkipExcludedConnection(t *testing.T) {
	// exclude connections from 127.0.0.1:80
	config := testConfig()
	// exclude source SSH connections to make this pass in VM
	config.ExcludedSourceConnections = map[string][]string{"127.0.0.1": {"80"}, "*": {"22"}}
	config.ExcludedDestinationConnections = map[string][]string{"127.0.0.1": {"tcp 80"}}
	tr, err := NewTracer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	// Connect to 127.0.0.1:80
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:80")
	assert.NoError(t, err)

	cn, err := net.DialUDP("udp", nil, addr)
	assert.NoError(t, err)
	defer cn.Close()

	// Write anything
	_, err = cn.Write([]byte("test"))
	assert.NoError(t, err)

	initTracerState(t, tr)

	// Make sure we're not picking up 127.0.0.1:80
	for _, c := range getConnections(t, tr).Conns {
		assert.False(t, c.Source.String() == "127.0.0.1" && c.SPort == 80, "connection %s should be excluded", c)
		assert.False(t, c.Dest.String() == "127.0.0.1" && c.DPort == 80 && c.Type == network.TCP, "connection %s should be excluded", c)
	}

	// ensure one of the connections is UDP to 127.0.0.1:80
	assert.Condition(t, func() bool {
		for _, c := range getConnections(t, tr).Conns {
			if c.Dest.String() == "127.0.0.1" && c.DPort == 80 && c.Type == network.UDP {
				return true
			}
		}
		return false
	}, "Unable to find UDP connection to 127.0.0.1:80")
}

func TestSkipConnectionDNS(t *testing.T) {
	t.Run("CollectLocalDNS disabled", func(t *testing.T) {
		tr := &Tracer{config: &config.Config{CollectLocalDNS: false}}
		assert.True(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 53,
		}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 8080,
		}))

		assert.True(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}))

		assert.True(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}))
	})

	t.Run("CollectLocalDNS disabled", func(t *testing.T) {
		tr := &Tracer{config: &config.Config{CollectLocalDNS: true}}

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 53,
		}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.1"),
			Dest:   util.AddressFromString("127.0.0.1"),
			SPort:  1000, DPort: 8080,
		}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}))

		assert.False(t, tr.shouldSkipConnection(&network.ConnectionStats{
			Source: util.AddressFromString("::3f::45"),
			Dest:   util.AddressFromString("::1"),
			SPort:  53, DPort: 1000,
		}))

	})
}

func byAddress(l, r net.Addr) func(c network.ConnectionStats) bool {
	return func(c network.ConnectionStats) bool {
		return addrMatches(l, c.Source.String(), c.SPort) && addrMatches(r, c.Dest.String(), c.DPort)
	}
}

func findConnection(l, r net.Addr, c *network.Connections) (*network.ConnectionStats, bool) {
	if result := searchConnections(c, byAddress(l, r)); len(result) > 0 {
		return &result[0], true
	}

	return nil, false
}

func searchConnections(c *network.Connections, predicate func(network.ConnectionStats) bool) []network.ConnectionStats {
	var results []network.ConnectionStats
	for _, conn := range c.Conns {
		if predicate(conn) {
			results = append(results, conn)
		}
	}
	return results
}

func addrMatches(addr net.Addr, host string, port uint16) bool {
	addrURL := url.URL{Scheme: addr.Network(), Host: addr.String()}

	return addrURL.Hostname() == host && addrURL.Port() == strconv.Itoa(int(port))
}

func runBenchtests(b *testing.B, payloads []int, prefix string, f func(p int) func(*testing.B)) {
	for _, p := range payloads {
		name := strings.TrimSpace(strings.Join([]string{prefix, strconv.Itoa(p), "bytes"}, " "))
		b.Run(name, f(p))
	}
}

func BenchmarkUDPEcho(b *testing.B) {
	runBenchtests(b, payloadSizesUDP, "", benchEchoUDP)

	// Enable BPF-based system probe
	t, err := NewTracer(testConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer t.Stop()

	runBenchtests(b, payloadSizesUDP, "eBPF", benchEchoUDP)
}

func benchEchoUDP(size int) func(b *testing.B) {
	payload := genPayload(size)
	echoOnMessage := func(b []byte, n int) []byte {
		resp := make([]byte, len(b))
		copy(resp, b)
		return resp
	}

	return func(b *testing.B) {
		end := make(chan struct{})
		server := &UDPServer{onMessage: echoOnMessage}
		err := server.Run(end, size)
		require.NoError(b, err)
		defer close(end)

		c, err := net.DialTimeout("udp", server.address, 50*time.Millisecond)
		if err != nil {
			b.Fatal(err)
		}
		defer c.Close()
		r := bufio.NewReader(c)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Write(payload)
			buf := make([]byte, size)
			n, err := r.Read(buf)

			if err != nil || n != len(payload) || !bytes.Equal(payload, buf) {
				b.Fatalf("Sizes: %d, %d. Equal: %v. Error: %s", len(buf), len(payload), bytes.Equal(payload, buf), err)
			}
		}
		b.StopTimer()
	}
}

func BenchmarkTCPEcho(b *testing.B) {
	runBenchtests(b, payloadSizesTCP, "", benchEchoTCP)

	// Enable BPF-based system probe
	t, err := NewTracer(testConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer t.Stop()

	runBenchtests(b, payloadSizesTCP, "eBPF", benchEchoTCP)
}

func BenchmarkTCPSend(b *testing.B) {
	runBenchtests(b, payloadSizesTCP, "", benchSendTCP)

	// Enable BPF-based system probe
	t, err := NewTracer(testConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer t.Stop()

	runBenchtests(b, payloadSizesTCP, "eBPF", benchSendTCP)
}

func benchEchoTCP(size int) func(b *testing.B) {
	payload := genPayload(size)
	echoOnMessage := func(c net.Conn) {
		r := bufio.NewReader(c)
		for {
			buf, err := r.ReadBytes(byte('\n'))
			if err == io.EOF {
				c.Close()
				return
			}
			c.Write(buf)
		}
	}

	return func(b *testing.B) {
		end := make(chan struct{})
		server := NewTCPServer(echoOnMessage)
		err := server.Run(end)
		require.NoError(b, err)
		defer close(end)

		c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
		if err != nil {
			b.Fatal(err)
		}
		defer c.Close()
		r := bufio.NewReader(c)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c.Write(payload)
			buf, err := r.ReadBytes(byte('\n'))

			if err != nil || len(buf) != len(payload) || !bytes.Equal(payload, buf) {
				b.Fatalf("Sizes: %d, %d. Equal: %v. Error: %s", len(buf), len(payload), bytes.Equal(payload, buf), err)
			}
		}
		b.StopTimer()
	}
}

func benchSendTCP(size int) func(b *testing.B) {
	payload := genPayload(size)
	dropOnMessage := func(c net.Conn) {
		r := bufio.NewReader(c)
		for { // Drop all payloads received
			_, err := r.Discard(r.Buffered() + 1)
			if err == io.EOF {
				c.Close()
				return
			}
		}
	}

	return func(b *testing.B) {
		end := make(chan struct{})
		server := NewTCPServer(dropOnMessage)
		err := server.Run(end)
		require.NoError(b, err)
		defer close(end)

		c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
		if err != nil {
			b.Fatal(err)
		}
		defer c.Close()

		b.ResetTimer()
		for i := 0; i < b.N; i++ { // Send-heavy workload
			_, err := c.Write(payload)
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
	}
}

type TCPServer struct {
	address   string
	onMessage func(c net.Conn)
}

func NewTCPServer(onMessage func(c net.Conn)) *TCPServer {
	return NewTCPServerOnAddress("127.0.0.1:0", onMessage)
}

func NewTCPServerOnAddress(addr string, onMessage func(c net.Conn)) *TCPServer {
	return &TCPServer{
		address:   addr,
		onMessage: onMessage,
	}
}

func (s *TCPServer) Run(done chan struct{}) error {
	ln, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.address = ln.Addr().String()

	go func() {
		<-done
		ln.Close()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.onMessage(conn)
		}
	}()

	return nil
}

type UDPServer struct {
	network   string
	address   string
	onMessage func(b []byte, n int) []byte
}

func (s *UDPServer) Run(done chan struct{}, payloadSize int) error {
	network := "udp"
	if s.network != "" {
		network = s.network
	}

	ln, err := net.ListenPacket(network, s.address)
	if err != nil {
		return err
	}

	s.address = ln.LocalAddr().String()

	go func() {
		buf := make([]byte, payloadSize)
		running := true
		for running {
			select {
			case <-done:
				running = false
			default:
				ln.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
				n, addr, err := ln.ReadFrom(buf)
				if err != nil {
					break
				}
				_, err = ln.WriteTo(s.onMessage(buf, n), addr)
				if err != nil {
					fmt.Println(err)
					break
				}
			}
		}

		ln.Close()
	}()

	return nil
}

var letterBytes = []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func genPayload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		if i == n-1 {
			b[i] = '\n'
		} else {
			b[i] = letterBytes[rand.Intn(len(letterBytes))]
		}
	}
	return b
}

func addrPort(addr string) int {
	p, _ := strconv.Atoi(strings.Split(addr, ":")[1])
	return p
}

func initTracerState(t *testing.T, tr *Tracer) {
	err := tr.RegisterClient("1")
	require.NoError(t, err)
}

func getConnections(t *testing.T, tr *Tracer) *network.Connections {
	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections, err := tr.GetActiveConnections("1")
	if err != nil {
		t.Fatal(err)
	}

	return connections
}

const (
	validDNSServer = "8.8.8.8"
)

func testDNSStats(t *testing.T, domain string, success int, failure int, timeout int, serverIP string) {
	config := testConfig()
	config.CollectDNSStats = true
	config.DNSTimeout = 1 * time.Second
	tr, err := NewTracer(config)
	require.NoError(t, err)
	defer tr.Stop()

	dnsServerAddr := &net.UDPAddr{IP: net.ParseIP(serverIP), Port: 53}

	queryMsg := new(dns.Msg)
	queryMsg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	queryMsg.RecursionDesired = true

	dnsClient := new(dns.Client)
	dnsConn, err := dnsClient.Dial(dnsServerAddr.String())
	require.NoError(t, err)
	defer dnsConn.Close()
	dnsClientAddr := dnsConn.LocalAddr().(*net.UDPAddr)
	_, _, err = dnsClient.ExchangeWithConn(queryMsg, dnsConn)

	if err != nil && timeout == 0 {
		t.Fatalf("Failed to get dns response %s\n", err.Error())
	}

	// Allow the DNS reply to be processed in the snooper
	time.Sleep(time.Millisecond * 500)

	initTracerState(t, tr)

	// Iterate through active connections until we find connection created above, and confirm send + recv counts
	connections := getConnections(t, tr)
	conn, ok := findConnection(dnsClientAddr, dnsServerAddr, connections)
	require.True(t, ok)

	assert.Equal(t, queryMsg.Len(), int(conn.MonotonicSentBytes))
	assert.Equal(t, os.Getpid(), int(conn.Pid))
	assert.Equal(t, dnsServerAddr.Port, int(conn.DPort))

	dnsKey, ok := network.DNSKey(conn)
	require.True(t, ok)

	dnsStats, ok := connections.DNSStats[dnsKey]
	require.True(t, ok)

	var total uint32
	var successfulResponses uint32
	var timeouts uint32
	for _, byDomain := range dnsStats {
		for _, byQueryType := range byDomain {
			successfulResponses += byQueryType.CountByRcode[uint32(0)]
			timeouts += byQueryType.Timeouts
			for _, count := range byQueryType.CountByRcode {
				total += count
			}
		}
	}

	failedResponses := total - successfulResponses

	// DNS Stats
	assert.Equal(t, uint32(success), successfulResponses)
	assert.Equal(t, uint32(failure), failedResponses)
	assert.Equal(t, uint32(timeout), timeouts)
}

func TestDNSStatsForValidDomain(t *testing.T) {
	testDNSStats(t, "golang.org", 1, 0, 0, validDNSServer)
}

func TestDNSStatsForInvalidDomain(t *testing.T) {
	testDNSStats(t, "abcdedfg", 0, 1, 0, validDNSServer)
}

func TestDNSStatsForTimeout(t *testing.T) {
	testDNSStats(t, "golang.org", 0, 0, 1, "1.2.3.4")
}

func TestTCPEstablished(t *testing.T) {
	// Ensure closed connections are flushed as soon as possible
	cfg := testConfig()
	cfg.TCPClosedTimeout = 500 * time.Millisecond

	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	initTracerState(t, tr)

	server := NewTCPServer(func(c net.Conn) {
		io.Copy(ioutil.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err = server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)

	laddr, raddr := c.LocalAddr(), c.RemoteAddr()
	c.Write([]byte("hello"))

	connections := getConnections(t, tr)
	conn, ok := findConnection(laddr, raddr, connections)

	require.True(t, ok)
	assert.Equal(t, uint32(1), conn.LastTCPEstablished)
	assert.Equal(t, uint32(0), conn.LastTCPClosed)

	c.Close()
	// Wait for the connection to be sent from the perf buffer
	time.Sleep(cfg.TCPClosedTimeout)

	connections = getConnections(t, tr)
	conn, ok = findConnection(laddr, raddr, connections)
	require.True(t, ok)
	assert.Equal(t, uint32(0), conn.LastTCPEstablished)
	assert.Equal(t, uint32(1), conn.LastTCPClosed)
}

func TestTCPEstablishedPreExistingConn(t *testing.T) {
	server := NewTCPServer(func(c net.Conn) {
		io.Copy(ioutil.Discard, c)
		c.Close()
	})
	doneChan := make(chan struct{})
	err := server.Run(doneChan)
	require.NoError(t, err)
	defer close(doneChan)

	c, err := net.DialTimeout("tcp", server.address, 50*time.Millisecond)
	require.NoError(t, err)
	laddr, raddr := c.LocalAddr(), c.RemoteAddr()

	// Ensure closed connections are flushed as soon as possible
	cfg := testConfig()
	cfg.TCPClosedTimeout = 500 * time.Millisecond

	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	initTracerState(t, tr)

	c.Write([]byte("hello"))
	c.Close()
	// Wait for the connection to be sent from the perf buffer
	time.Sleep(cfg.TCPClosedTimeout)
	connections := getConnections(t, tr)
	conn, ok := findConnection(laddr, raddr, connections)

	require.True(t, ok)
	assert.Equal(t, uint32(0), conn.MonotonicTCPEstablished)
	assert.Equal(t, uint32(1), conn.MonotonicTCPClosed)
}

func TestUnconnectedUDPSendIPv4(t *testing.T) {
	cfg := testConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	remotePort := rand.Int()%5000 + 15000
	remoteAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: remotePort}
	// Use ListenUDP instead of DialUDP to create a "connectionless" UDP connection
	conn, err := net.ListenUDP("udp", nil)
	require.NoError(t, err)
	defer conn.Close()
	message := []byte("payload")
	bytesSent, err := conn.WriteTo(message, remoteAddr)
	require.NoError(t, err)

	initTracerState(t, tr)
	connections := getConnections(t, tr)
	outgoing := searchConnections(connections, func(cs network.ConnectionStats) bool {
		return cs.DPort == uint16(remotePort)
	})

	require.Len(t, outgoing, 1)
	assert.Equal(t, bytesSent, int(outgoing[0].MonotonicSentBytes))
}

func TestConnectedUDPSendIPv6(t *testing.T) {
	if !kernel.IsIPv6Enabled() {
		t.Skip("IPv6 not enabled on host")
	}

	cfg := testConfig()
	cfg.CollectIPv6Conns = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	remotePort := rand.Int()%5000 + 15000
	remoteAddr := &net.UDPAddr{IP: net.IPv6loopback, Port: remotePort}
	conn, err := net.DialUDP("udp6", nil, remoteAddr)
	require.NoError(t, err)
	defer conn.Close()
	message := []byte("payload")
	bytesSent, err := conn.Write(message)
	require.NoError(t, err)

	initTracerState(t, tr)
	connections := getConnections(t, tr)
	outgoing := searchConnections(connections, func(cs network.ConnectionStats) bool {
		return cs.DPort == uint16(remotePort)
	})

	require.Len(t, outgoing, 1)
	assert.Equal(t, remoteAddr.IP.String(), outgoing[0].Dest.String())
	assert.Equal(t, bytesSent, int(outgoing[0].MonotonicSentBytes))
}

func TestConnectionClobber(t *testing.T) {
	cfg := testConfig()
	cfg.CollectUDPConns = false
	cfg.ExcludedDestinationConnections = map[string][]string{
		"0.0.0.0/2":   {"*"},
		"64.0.0.0/3":  {"*"},
		"96.0.0.0/4":  {"*"},
		"112.0.0.0/5": {"*"},
		"120.0.0.0/6": {"*"},
		"124.0.0.0/7": {"*"},
		"126.0.0.0/8": {"*"},
		"128.0.0.0/1": {"*"},
	}
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	// Create TCP Server which, for every line, sends back a message with size=serverMessageSize
	var serverConns []net.Conn
	srvRecvBuf := make([]byte, 4)
	server := NewTCPServer(func(c net.Conn) {
		serverConns = append(serverConns, c)
		_, _ = io.ReadFull(c, srvRecvBuf)
		_, _ = c.Write(srvRecvBuf)
	})
	doneChan := make(chan struct{})
	server.Run(doneChan)
	defer close(doneChan)

	// we only need 1/4 since both send and recv sides will be registered
	sendCount := tr.activeBuffer.Capacity()/4 + 1
	sendAndRecv := func() []net.Conn {
		connsCh := make(chan net.Conn, sendCount)
		var conns []net.Conn
		go func() {
			for c := range connsCh {
				conns = append(conns, c)
			}
		}()

		workers := sync.WaitGroup{}
		for i := 0; i < sendCount; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()

				c, err := net.DialTimeout("tcp", server.address, 5*time.Second)
				require.NoError(t, err)
				connsCh <- c

				buf := make([]byte, 4)
				_, err = c.Write(buf)
				require.NoError(t, err)

				_, err = io.ReadFull(c, buf[:0])
				require.NoError(t, err)
			}()
		}

		workers.Wait()
		close(connsCh)

		return conns
	}

	conns := sendAndRecv()

	// wait for tracer to pick up all connections
	//
	// there is not a good way do this other than a sleep since we
	// can't call getConnections in a `require.Eventually` call
	// to the get the number of connections as that could
	// affect the `activeBuffer` length
	time.Sleep(2 * time.Second)

	preCap := tr.activeBuffer.Capacity()
	initTracerState(t, tr)
	connections := getConnections(t, tr)
	require.NotEmpty(t, connections)
	src := connections.Conns[0].SPort
	dst := connections.Conns[0].DPort
	t.Logf("got %d connections", len(connections.Conns))
	// ensure we didn't grow or shrink the buffer
	assert.Equal(t, preCap, tr.activeBuffer.Capacity())

	for _, c := range append(conns, serverConns...) {
		c.Close()
	}

	// send second batch so that underlying array gets clobbered
	conns = sendAndRecv()
	serverConns = serverConns[:0]
	defer func() {
		for _, c := range append(conns, serverConns...) {
			c.Close()
		}
	}()

	time.Sleep(2 * time.Second)

	t.Logf("got %d connections", len(getConnections(t, tr).Conns))
	assert.Equal(t, src, connections.Conns[0].SPort, "source port should not change")
	assert.Equal(t, dst, connections.Conns[0].DPort, "dest port should not change")
	assert.Equal(t, preCap, tr.activeBuffer.Capacity())
}

func TestTCPDirection(t *testing.T) {

	cfg := testConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	initTracerState(t, tr)

	// Start an HTTP server on localhost:8080
	serverAddr := "127.0.0.1:8080"
	srv := &nethttp.Server{
		Addr: serverAddr,
		Handler: nethttp.HandlerFunc(func(w nethttp.ResponseWriter, req *nethttp.Request) {
			t.Logf("received http request from %s", req.RemoteAddr)
			io.Copy(ioutil.Discard, req.Body)
			w.WriteHeader(200)
		}),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	srv.SetKeepAlivesEnabled(false)
	go func() {
		_ = srv.ListenAndServe()
	}()
	defer srv.Shutdown(context.Background())

	// Allow the HTTP server time to get set up
	time.Sleep(time.Millisecond * 500)

	// Send a HTTP request to the test server
	client := new(nethttp.Client)
	resp, err := client.Get("http://" + serverAddr + "/test")
	require.NoError(t, err)
	resp.Body.Close()

	// Iterate through active connections until we find connection created above
	var outgoingConns []network.ConnectionStats
	var incomingConns []network.ConnectionStats
	require.Eventuallyf(t, func() bool {
		conns := getConnections(t, tr)
		if len(outgoingConns) == 0 {
			outgoingConns = searchConnections(conns, func(cs network.ConnectionStats) bool {
				return fmt.Sprintf("%s:%d", cs.Dest, cs.DPort) == serverAddr
			})
		}
		if len(incomingConns) == 0 {
			incomingConns = searchConnections(conns, func(cs network.ConnectionStats) bool {
				return fmt.Sprintf("%s:%d", cs.Source, cs.SPort) == serverAddr
			})
		}

		return len(outgoingConns) == 1 && len(incomingConns) == 1
	}, 3*time.Second, 10*time.Millisecond, "couldn't find incoming and outgoing http connections matching: %s", serverAddr)

	// Verify connection directions
	conn := outgoingConns[0]
	assert.Equal(t, conn.Direction, network.OUTGOING, "connection direction must be outgoing: %s", conn)
	conn = incomingConns[0]
	assert.Equal(t, conn.Direction, network.INCOMING, "connection direction must be incoming: %s", conn)
}

func TestTCPDirectionWithPreexistingConnection(t *testing.T) {
	// incoming flow is marked outgoing: windows doesn't have a way to handle directionality of preexisting connections
	skipIfWindows(t)

	wg := sync.WaitGroup{}

	// setup server to listen on a port
	server := NewTCPServer(func(c net.Conn) {
		t.Logf("received connection from %s", c.RemoteAddr())
		_, _ = bufio.NewReader(c).ReadBytes('\n')
		c.Close()
		wg.Done()
	})
	doneChan := make(chan struct{})
	server.Run(doneChan)
	defer close(doneChan)
	t.Logf("server address: %s", server.address)

	// create an initial client connection to the server
	c, err := net.DialTimeout("tcp", server.address, 5*time.Second)
	require.NoError(t, err)
	defer c.Close()

	// start tracer so it dumps port bindings
	cfg := testConfig()
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	initTracerState(t, tr)

	// open and close another client connection to force port binding delete
	wg.Add(1)
	c2, err := net.DialTimeout("tcp", server.address, 5*time.Second)
	require.NoError(t, err)
	_, err = c2.Write([]byte("conn2\n"))
	require.NoError(t, err)
	c2.Close()

	wg.Wait()

	wg.Add(1)
	// write some data so tracer determines direction of this connection
	_, err = c.Write([]byte("original\n"))
	require.NoError(t, err)

	wg.Wait()

	// the original connection should still be incoming for the server
	conns := getConnections(t, tr)
	origConn := searchConnections(conns, func(cs network.ConnectionStats) bool {
		return fmt.Sprintf("%s:%d", cs.Source, cs.SPort) == server.address &&
			fmt.Sprintf("%s:%d", cs.Dest, cs.DPort) == c.LocalAddr().String()
	})
	require.Len(t, origConn, 1)
	require.Equal(t, network.INCOMING, origConn[0].Direction, "original server<->client connection should have incoming direction")
}

func TestEnableHTTPMonitoring(t *testing.T) {
	if !httpSupported(t) {
		t.Skip("HTTP monitoring not supported")
	}

	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()
}

func TestHTTPStats(t *testing.T) {
	if !httpSupported(t) {
		t.Skip("HTTP monitoring feature not available")
		return
	}

	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	initTracerState(t, tr)

	// Start an HTTP server on localhost:8080
	serverAddr := "127.0.0.1:8080"
	srv := &nethttp.Server{
		Addr: serverAddr,
		Handler: nethttp.HandlerFunc(func(w nethttp.ResponseWriter, req *nethttp.Request) {
			io.Copy(ioutil.Discard, req.Body)
			w.WriteHeader(200)
		}),
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	srv.SetKeepAlivesEnabled(false)
	go func() {
		_ = srv.ListenAndServe()
	}()
	defer srv.Shutdown(context.Background())

	// Allow the HTTP server time to get set up
	time.Sleep(time.Millisecond * 500)

	// Send a series of HTTP requests to the test server
	client := new(nethttp.Client)
	resp, err := client.Get("http://" + serverAddr + "/test")
	require.NoError(t, err)
	resp.Body.Close()

	// Iterate through active connections until we find connection created above
	var httpReqStats *http.RequestStats
	require.Eventuallyf(t, func() bool {
		payload, err := tr.GetActiveConnections("1")
		if err != nil {
			t.Fatal(err)
		}

		for key, stats := range payload.HTTP {
			if key.Path == "/test" {
				httpReqStats = stats
				return true
			}
		}

		return false
	}, 3*time.Second, 10*time.Millisecond, "couldn't find http connection matching: %s", serverAddr)

	// Verify HTTP stats
	require.NotNil(t, httpReqStats)
	assert.Nil(t, httpReqStats.Stats(100), "100s")            // number of requests with response status 100
	assert.Equal(t, 1, httpReqStats.Stats(200).Count, "200s") // 200
	assert.Nil(t, httpReqStats.Stats(300), "300s")            // 300
	assert.Nil(t, httpReqStats.Stats(400), "400s")            // 400
	assert.Nil(t, httpReqStats.Stats(500), "500s")            // 500
}

var regexSSL = regexp.MustCompile(`/[^\ ]+libssl.so[^\ ]*`)

func TestHTTPSViaOpenSSLIntegration(t *testing.T) {
	if !httpSupported(t) {
		t.Skip("HTTPS feature not available on pre 4.1.0 kernels")
	}

	if !httpsSupported(t) {
		t.Skip("HTTPS feature not supported.")
	}

	if strings.HasPrefix(runtime.GOARCH, "arm") {
		t.Skip("this feature is not yet support on arm")
	}

	wget, err := exec.LookPath("wget")
	if err != nil {
		t.Skip("wget not found; skipping test.")
	}

	ldd, err := exec.LookPath("ldd")
	if err != nil {
		t.Skip("ldd not found; skipping test.")
	}

	linked, _ := exec.Command(ldd, wget).Output()
	libSSLPath := regexSSL.FindString(string(linked))
	if _, err := os.Stat(libSSLPath); len(libSSLPath) == 0 || os.IsNotExist(err) {
		t.Skip("libssl.so not found; skipping test.")
	}

	// Start tracer with HTTPS support
	cfg := testConfig()
	cfg.EnableHTTPMonitoring = true
	cfg.EnableHTTPSMonitoring = true
	tr, err := NewTracer(cfg)
	require.NoError(t, err)
	defer tr.Stop()

	testHTTPS := func(keepalives bool) {
		// Spin-up HTTPS server
		serverDoneFn := testutil.HTTPServer(t, "127.0.0.1:443", testutil.Options{
			EnableTLS:        true,
			EnableKeepAlives: keepalives,
		})
		defer serverDoneFn()

		// Run wget once to make sure the OpenSSL is detected and uprobes are attached
		exec.Command(wget).Run()
		time.Sleep(time.Second)

		// Issue request using `wget`
		// This is necessary (as opposed to using net/http) because we want to
		// test a HTTP client linked to OpenSSL
		const targetURL = "https://127.0.0.1:443/200/foobar"
		requestCmd := exec.Command(wget, "--no-check-certificate", "-O/dev/null", targetURL)
		err = requestCmd.Run()
		assert.NoErrorf(t, err, "failed to issue request via wget: %s", err)

		assert.Eventuallyf(t, func() bool {
			payload, _ := tr.GetActiveConnections("1")
			for key := range payload.HTTP {
				if key.Path == "/200/foobar" {
					return true
				}
			}

			return false
		}, 3*time.Second, 10*time.Millisecond, "couldn't find HTTPS stats")
	}

	t.Run("with keep-alives", func(t *testing.T) {
		testHTTPS(true)
	})
	t.Run("without keep-alives", func(t *testing.T) {
		testHTTPS(false)
	})
}

func TestRuntimeCompilerEnvironmentVar(t *testing.T) {
	cfg := testConfig()
	enabled := os.Getenv(runtimeCompilationEnvVar) != ""
	assert.Equal(t, enabled, cfg.EnableRuntimeCompiler)
	assert.NotEqual(t, enabled, cfg.AllowPrecompiledFallback)
}

func testConfig() *config.Config {
	cfg := config.New()
	if os.Getenv("BPF_DEBUG") != "" {
		cfg.BPFDebug = true
	}
	if os.Getenv(runtimeCompilationEnvVar) != "" {
		cfg.EnableRuntimeCompiler = true
		cfg.AllowPrecompiledFallback = false
	}
	return cfg
}

func doDNSQuery(t *testing.T, domain string, serverIP string) (*net.UDPAddr, *net.UDPAddr) {
	dnsServerAddr := &net.UDPAddr{IP: net.ParseIP(serverIP), Port: 53}
	queryMsg := new(dns.Msg)
	queryMsg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	queryMsg.RecursionDesired = true
	dnsClient := new(dns.Client)
	dnsConn, err := dnsClient.Dial(dnsServerAddr.String())
	require.NoError(t, err)
	defer dnsConn.Close()
	dnsClientAddr := dnsConn.LocalAddr().(*net.UDPAddr)
	_, _, err = dnsClient.ExchangeWithConn(queryMsg, dnsConn)
	require.NoError(t, err)

	return dnsClientAddr, dnsServerAddr
}

func skipIfWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test unavailable on windows")
	}
}
